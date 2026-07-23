package queue

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// Processor executes the work a Job represents.
type Processor interface {
	Process(ctx context.Context, j *Job) error
}

// defaultMaxAttempts and defaultBaseBackoff are the retry settings New
// gives a Queue built without explicit config, matching the queue.*
// defaults in docs/ARCHITECTURE.md §7. cmd/receiptd uses NewWithRetry
// with the configured values instead.
const (
	defaultMaxAttempts = 3
	defaultBaseBackoff = 5 * time.Second
)

// ProcessNext locates one pending Job, marks it JobRunning (persisting
// that), then invokes the Queue's Processor via runClaimedJob. A
// KindTransient failure retries with exponential backoff starting at
// q.baseBackoff, up to q.maxAttempts total; any other kind fails the Job
// immediately. If a backoff wait is cut short by ctx cancellation, the Job
// is left JobRunning rather than marked Failed — see runClaimedJob and
// docs/adr/0018-graceful-shutdown.md. A Processor error is recorded on the
// Job (State, LastError), not returned — it's a business-level outcome,
// not a queue failure; only Store errors are returned. At most one Job
// (including its retries) is processed per call.
//
// ProcessNext selects globally via NextPending, non-atomically — safe
// today only because it's the sole caller of that path; cmd/receiptd's
// per-printer workers instead use ProcessNextForPrinter
// (docs/adr/0016-queue-concurrency-per-printer-workers.md).
func (q *Queue) ProcessNext(ctx context.Context) error {
	next, err := q.store.NextPending(ctx)
	if err != nil {
		return err
	}
	if next == nil {
		return nil
	}

	next.State = JobRunning
	next.UpdatedAt = time.Now()
	if err := q.store.Save(ctx, next); err != nil {
		return err
	}

	return q.runClaimedJob(ctx, next)
}

// ProcessNextForPrinter behaves exactly like ProcessNext, except it
// atomically selects and claims its Job via
// q.store.ClaimNextPending(ctx, printerName) instead of NextPending plus a
// manual transition-and-save — what makes it safe to run one of these per
// configured printer concurrently (docs/adr/
// 0016-queue-concurrency-per-printer-workers.md). This is what
// cmd/receiptd's per-printer worker goroutines call.
func (q *Queue) ProcessNextForPrinter(ctx context.Context, printerName string) error {
	next, err := q.store.ClaimNextPending(ctx, printerName)
	if err != nil {
		return err
	}
	if next == nil {
		return nil
	}

	return q.runClaimedJob(ctx, next)
}

// runClaimedJob runs the retry/backoff loop for a Job its caller already
// transitioned to JobRunning and persisted, then persists the resulting
// terminal state — the shared body of ProcessNext and
// ProcessNextForPrinter, which differ only in how they claim next.
func (q *Queue) runClaimedJob(ctx context.Context, next *Job) error {
	var procErr error
	backoff := q.baseBackoff
	for attempt := 1; attempt <= q.maxAttempts; attempt++ {
		next.Attempts++
		procErr = q.callProcessor(ctx, next)
		if procErr == nil || !apperr.Is(procErr, apperr.KindTransient) || attempt == q.maxAttempts {
			break
		}
		q.sleep(ctx, backoff)
		if ctx.Err() != nil {
			// Cut short by cancellation, not exhausted attempts: leave the
			// Job JobRunning, not Failed (docs/adr/0018-graceful-shutdown.md).
			return nil
		}
		backoff *= 2
	}

	if procErr != nil {
		next.State = JobFailed
		next.LastError = procErr.Error()
		// Logged in full here: internal/api.JobStatusHandler only returns
		// LastError sanitized, since it may embed a path or printer address.
		log.Printf("queue: job %s failed: %v", next.ID, procErr)
	} else {
		next.State = JobDone
	}
	next.UpdatedAt = time.Now()

	return q.store.Save(ctx, next)
}

// sleepCtx blocks for d, or until ctx is cancelled, whichever comes
// first — the real backoff wait New wires up as Queue.sleep. Without it,
// ProcessNext's retry loop would ignore cancellation once sleeping,
// holding the caller's goroutine for the full backoff regardless of ctx.
func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

// callProcessor invokes q.processor.Process, recovering any panic instead
// of letting it unwind through ProcessNext: a bug in the
// render/encode/print pipeline must fail one Job, not take down the
// background worker goroutine (and with it the whole daemon, since an
// unrecovered panic terminates the process). A recovered panic is logged
// with the Job ID, value, and stack, and reported as apperr.KindPermanent
// — the same Kind a renderer bug maps to — so the Job fails immediately
// rather than retrying the same panic.
func (q *Queue) callProcessor(ctx context.Context, j *Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("queue: recovered panic processing job %s: %v\n%s", j.ID, r, debug.Stack())
			err = apperr.Wrap(apperr.KindPermanent, "queue.ProcessNext", fmt.Errorf("panic: %v", r))
		}
	}()
	return q.processor.Process(ctx, j)
}
