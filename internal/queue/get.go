package queue

import "context"

// Get returns the Job identified by id, as currently persisted in the
// Queue's Store. It performs no state transition and never invokes the
// Processor — it is a read-only lookup, unlike Cancel and ProcessNext
// which both mutate and persist the Job they operate on.
func (q *Queue) Get(ctx context.Context, id string) (*Job, error) {
	return q.store.Get(ctx, id)
}
