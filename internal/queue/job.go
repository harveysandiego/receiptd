package queue

import (
	"time"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

// JobState is the lifecycle state of a Job.
type JobState string

// The closed set of states a Job moves through, in order, with Failed and
// Cancelled as the two terminal exits besides Done.
const (
	JobPending   JobState = "pending"
	JobRunning   JobState = "running"
	JobDone      JobState = "done"
	JobFailed    JobState = "failed"
	JobCancelled JobState = "cancelled"
)

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
}
