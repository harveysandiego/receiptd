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
// gives a Queue built without explicit config — matching the
// queue.max_attempts/queue.retry_backoff defaults documented in
// docs/ARCHITECTURE.md §7's config schema. cmd/receiptd's composition root
// uses NewWithRetry with cfg.Queue.MaxAttempts/RetryBackoff instead of
// these, since config.Validate already guarantees both are positive.
const (
	defaultMaxAttempts = 3
	defaultBaseBackoff = 5 * time.Second
)

// ProcessNext locates one pending Job, transitions it to JobRunning and
// persists that transition, then invokes the Queue's Processor. A
// Processor failure with apperr.KindTransient is retried, with exponential
// backoff starting at q.baseBackoff, up to q.maxAttempts total attempts;
// any other error kind fails the Job immediately with no retry. A backoff
// wait is interrupted early if ctx is cancelled, in which case retrying
// stops immediately and the Job is failed with the last Processor error,
// the same outcome as exhausting every attempt. Once the Job succeeds or
// its retries are exhausted (or interrupted), ProcessNext transitions it
// to JobDone or JobFailed and persists that. If no Job is pending,
// ProcessNext returns nil without invoking the Processor. A Processor
// error is recorded on the Job (State, LastError) rather than returned —
// it is a business-level outcome, not a queue failure. Only Store errors
// are returned. ProcessNext processes at most one Job, including all of
// that Job's retries, per call — there is no background looping or
// scheduling across calls.
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
			break
		}
		backoff *= 2
	}

	if procErr != nil {
		next.State = JobFailed
		next.LastError = procErr.Error()
	} else {
		next.State = JobDone
	}
	next.UpdatedAt = time.Now()

	return q.store.Save(ctx, next)
}

// sleepCtx blocks for d, or until ctx is cancelled, whichever comes
// first — the real backoff wait New wires up as Queue.sleep. Without this,
// ProcessNext's retry loop would ignore shutdown/cancellation entirely
// once it started sleeping, holding the caller's goroutine for the full
// backoff no matter what ctx says.
func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

// callProcessor invokes q.processor.Process, recovering any panic instead
// of letting it unwind through ProcessNext: rendering, encoding, and
// printing all run inside Process, and a bug anywhere in that pipeline
// must fail one Job, not take down the background worker goroutine (and,
// with it, the whole daemon — an unrecovered panic terminates the
// process). A recovered panic is logged with the Job ID, the recovered
// value, and a stack trace — enough to diagnose the bug — and reported as
// apperr.KindPermanent, the same Kind a renderer bug already maps to
// (see apperr.KindPermanent's own doc comment), so it fails the Job
// immediately rather than retrying the same panic.
func (q *Queue) callProcessor(ctx context.Context, j *Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("queue: recovered panic processing job %s: %v\n%s", j.ID, r, debug.Stack())
			err = apperr.Wrap(apperr.KindPermanent, "queue.ProcessNext", fmt.Errorf("panic: %v", r))
		}
	}()
	return q.processor.Process(ctx, j)
}
