package queue

import "context"

// List returns every Job currently in the Store — the plural counterpart
// to Get, with the same read-only contract: no state transition, no
// Processor invocation. Reconcile already relies on this via the Store
// directly; List exposes the same capability through Queue for any other
// caller that needs the whole set without reaching into the Store.
func (q *Queue) List(ctx context.Context) ([]*Job, error) {
	return q.store.List(ctx, Filter{})
}
