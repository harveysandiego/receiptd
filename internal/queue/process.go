package queue

import (
	"context"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// Processor executes the work a Job represents.
type Processor interface {
	Process(ctx context.Context, j *Job) error
}

// maxAttempts bounds how many times ProcessNext will call the Processor for
// a Job that keeps failing with apperr.KindTransient. baseBackoff is the
// delay before the first retry; each subsequent retry doubles it. These
// match the max_attempts/retry_backoff defaults documented in
// docs/ARCHITECTURE.md §7's config schema — config itself isn't wired up
// yet, so they're hardcoded here rather than configurable in this slice.
const (
	maxAttempts = 3
	baseBackoff = 5 * time.Second
)

// ProcessNext locates one pending Job, transitions it to JobRunning and
// persists that transition, then invokes the Queue's Processor. A
// Processor failure with apperr.KindTransient is retried, with exponential
// backoff starting at baseBackoff, up to maxAttempts total attempts; any
// other error kind fails the Job immediately with no retry. Once the Job
// succeeds or its retries are exhausted, ProcessNext transitions it to
// JobDone or JobFailed and persists that. If no Job is pending, ProcessNext
// returns nil without invoking the Processor. A Processor error is recorded
// on the Job (State, LastError) rather than returned — it is a
// business-level outcome, not a queue failure. Only Store errors are
// returned. ProcessNext processes at most one Job, including all of that
// Job's retries, per call — there is no background looping or scheduling
// across calls.
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
	backoff := baseBackoff
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		next.Attempts++
		procErr = q.processor.Process(ctx, next)
		if procErr == nil || !apperr.Is(procErr, apperr.KindTransient) || attempt == maxAttempts {
			break
		}
		q.sleep(backoff)
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
