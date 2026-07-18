package queue

import (
	"context"
	"time"
)

// Processor executes the work a Job represents.
type Processor interface {
	Process(ctx context.Context, j *Job) error
}

// ProcessNext locates one pending Job, transitions it to JobRunning and
// persists that transition, invokes the Queue's Processor, then transitions
// the Job to JobDone or JobFailed based on the result and persists that too.
// If no Job is pending, ProcessNext returns nil without invoking the
// Processor. A Processor error is recorded on the Job (State, LastError)
// rather than returned — it is a business-level outcome, not a queue
// failure. Only Store errors are returned. ProcessNext processes at most one
// Job per call; there is no background looping, retry or backoff in this
// slice.
func (q *Queue) ProcessNext(ctx context.Context) error {
	// TODO: listing every Job and scanning for the first JobPending is a
	// temporary implementation detail of the current in-memory Store, not a
	// property of this design. Once a persistent Store (BoltDB) exists,
	// Store should grow an operation for efficiently selecting the next
	// pending Job, and this scan should be replaced with that. Deferred to a
	// future slice — not an issue with ProcessNext as it stands today.
	jobs, err := q.store.List(ctx, Filter{})
	if err != nil {
		return err
	}

	var next *Job
	for _, j := range jobs {
		if j.State == JobPending {
			next = j
			break
		}
	}
	if next == nil {
		return nil
	}

	next.State = JobRunning
	next.Attempts++
	next.UpdatedAt = time.Now()
	if err := q.store.Save(ctx, next); err != nil {
		return err
	}

	if procErr := q.processor.Process(ctx, next); procErr != nil {
		next.State = JobFailed
		next.LastError = procErr.Error()
	} else {
		next.State = JobDone
	}
	next.UpdatedAt = time.Now()

	return q.store.Save(ctx, next)
}
