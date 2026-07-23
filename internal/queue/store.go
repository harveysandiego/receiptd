package queue

import (
	"context"
	"time"
)

// Filter narrows the results of Store.List. It has no fields yet — no
// filter criteria are frozen in docs/ARCHITECTURE.md §2 for this slice —
// so List currently returns every Job in the Store regardless of the
// Filter passed.
type Filter struct{}

// Store persists Jobs. Save creates the Job at j.ID if it doesn't exist yet
// and overwrites it if it does. Get, List, NextPending, ClaimNextPending, and
// EnqueueIdempotent never return a *Job that aliases the Store's internal
// state, so a caller mutating a returned Job can't corrupt what the Store
// holds.
type Store interface {
	Save(ctx context.Context, j *Job) error
	Get(ctx context.Context, id string) (*Job, error)
	List(ctx context.Context, f Filter) ([]*Job, error)
	// NextPending returns the first Job (ordered by ID) with State ==
	// JobPending, or nil if none exists. Not atomic and not printer-scoped;
	// a worker uses ClaimNextPending instead
	// (docs/adr/0016-queue-concurrency-per-printer-workers.md), but this
	// stays available for a caller that genuinely wants the global view.
	NextPending(ctx context.Context) (*Job, error)
	// ClaimNextPending atomically finds the lowest-ID Job with State ==
	// JobPending and PrinterName == printerName, transitions it to
	// JobRunning, persists that, and returns the transitioned copy (or
	// nil, nil if none exists). No two concurrent callers, for the same or
	// a different printerName, can ever be handed the same Job — what
	// makes one worker per configured printer safe
	// (docs/adr/0016-queue-concurrency-per-printer-workers.md).
	ClaimNextPending(ctx context.Context, printerName string) (*Job, error)

	// EnqueueIdempotent atomically resolves newJob.IdempotencyKey against
	// any existing non-expired Job under that key (empty key always
	// creates fresh, like Enqueue): a Receipt/PrinterName mismatch is
	// apperr.KindValidation; a match in JobPending/JobRunning/JobDone is
	// returned unchanged; a match in JobFailed is reactivated in place
	// (State, Attempts, LastError reset) — the one failed -> pending
	// transition this state machine otherwise disallows. now lets a
	// caller test the IdempotencyKeyTTL boundary deterministically. The
	// whole lookup-decide-persist sequence is atomic per key, reusing
	// ClaimNextPending's invariant (docs/adr/0020-idempotent-print-requests.md).
	EnqueueIdempotent(ctx context.Context, newJob *Job, now time.Time) (job *Job, created bool, err error)
}
