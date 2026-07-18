package queue

import "context"

// Filter narrows the results of Store.List. It has no fields yet — no
// filter criteria are frozen in docs/ARCHITECTURE.md §2 for this slice —
// so List currently returns every Job in the Store regardless of the
// Filter passed.
type Filter struct{}

// Store persists Jobs. Save creates the Job at j.ID if it doesn't exist yet
// and overwrites it if it does. Get and List never return a *Job that
// aliases the Store's internal state, so a caller mutating a returned Job
// can't corrupt what the Store holds.
type Store interface {
	Save(ctx context.Context, j *Job) error
	Get(ctx context.Context, id string) (*Job, error)
	List(ctx context.Context, f Filter) ([]*Job, error)
}
