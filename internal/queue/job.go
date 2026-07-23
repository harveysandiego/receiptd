package queue

import (
	"reflect"
	"time"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

// JobState is the lifecycle state of a Job.
type JobState string

// The closed set of states a Job moves through, in order, with Failed and
// Cancelled as the two terminal exits besides Done. Store.EnqueueIdempotent
// introduces one additional legal transition beyond this, failed ->
// pending, gated specifically on a same-key client retry — see
// docs/adr/0020-idempotent-print-requests.md.
const (
	JobPending   JobState = "pending"
	JobRunning   JobState = "running"
	JobDone      JobState = "done"
	JobFailed    JobState = "failed"
	JobCancelled JobState = "cancelled"
)

// IdempotencyKeyTTL is how long a Job's IdempotencyKey stays eligible to
// match a later request with the same key. Enforced lazily, inside
// Store.EnqueueIdempotent's lookup, not by an active sweep or deletion
// (docs/adr/0020-idempotent-print-requests.md).
const IdempotencyKeyTTL = 24 * time.Hour

// Job is one queued print request.
type Job struct {
	ID          string
	PrinterName string
	Receipt     receipt.Receipt
	State       JobState
	Attempts    int
	LastError   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// IdempotencyKey is the optional client-supplied key
	// (POST /api/v1/print's Idempotency-Key header) that lets a retried
	// request be recognized as the same logical print. Empty means "no
	// key supplied" (docs/adr/0020-idempotent-print-requests.md).
	IdempotencyKey string
}

// sameLogicalRequest reports whether a and b are the same logical print
// request (equal PrinterName and Receipt), the check
// Store.EnqueueIdempotent uses to reject a reused key as a client bug.
// reflect.DeepEqual is needed since Receipt.Elements is a slice of
// interface values, which == can't compare.
func sameLogicalRequest(a, b *Job) bool {
	return a.PrinterName == b.PrinterName && reflect.DeepEqual(a.Receipt, b.Receipt)
}

// idempotencyKeyExpired reports whether j's IdempotencyKey is older than
// IdempotencyKeyTTL as of now, and should therefore be treated by
// Store.EnqueueIdempotent as if it had never been used.
func idempotencyKeyExpired(j *Job, now time.Time) bool {
	return now.Sub(j.CreatedAt) > IdempotencyKeyTTL
}
