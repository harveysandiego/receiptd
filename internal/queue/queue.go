package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// Queue accepts Jobs for asynchronous printing, persists them via a Store,
// and processes them with a Processor, retrying apperr.KindTransient
// failures with bounded exponential backoff (see ProcessNext) and
// supporting cancellation of still-Pending Jobs (see Cancel).
type Queue struct {
	store     Store
	processor Processor
	// sleep is called between retry attempts; it's a field rather than a
	// direct time.Sleep call so tests can inject a non-blocking stub
	// instead of waiting out real backoff delays.
	sleep func(time.Duration)
}

// New returns a Queue that persists Jobs via store and processes them with
// processor.
func New(store Store, processor Processor) *Queue {
	return &Queue{store: store, processor: processor, sleep: time.Sleep}
}

// Enqueue assigns j a new ID, sets its State to JobPending, stamps
// CreatedAt and UpdatedAt with the current time, and persists it via the
// Queue's Store. Any ID or State the caller already set on j is
// overwritten.
func (q *Queue) Enqueue(ctx context.Context, j *Job) error {
	id, err := newJobID()
	if err != nil {
		return apperr.Wrap(apperr.KindPermanent, "queue.Enqueue", err)
	}
	j.ID = id
	j.State = JobPending
	now := time.Now()
	j.CreatedAt = now
	j.UpdatedAt = now

	return q.store.Save(ctx, j)
}

// newJobID returns a random 32-character hex string, unique with
// overwhelming probability. crypto/rand.Read against the default reader
// only fails on catastrophic OS entropy failure; that failure is reported
// to the caller rather than panicking, since Enqueue already has a
// structured way to report it.
func newJobID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
