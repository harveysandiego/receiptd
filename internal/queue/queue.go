package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// Queue accepts Jobs for asynchronous printing and persists them via a
// Store. Processing, retries, backoff and cancellation are not
// implemented by this slice — see docs/ARCHITECTURE.md §2.
type Queue struct {
	store Store
}

// New returns a Queue that persists Jobs via store.
func New(store Store) *Queue {
	return &Queue{store: store}
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
