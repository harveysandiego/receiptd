package queue

import "context"

// Filter narrows the results of Store.List. It has no fields yet — no
// filter criteria are frozen in docs/ARCHITECTURE.md §2 for this slice —
// so List currently returns every Job in the Store regardless of the
// Filter passed.
type Filter struct{}

// Store persists Jobs. Save creates the Job at j.ID if it doesn't exist yet
// and overwrites it if it does. Get, List, and NextPending never return a
// *Job that aliases the Store's internal state, so a caller mutating a
// returned Job can't corrupt what the Store holds.
type Store interface {
	Save(ctx context.Context, j *Job) error
	Get(ctx context.Context, id string) (*Job, error)
	List(ctx context.Context, f Filter) ([]*Job, error)
	// NextPending returns the first Job (ordered by ID) with State ==
	// JobPending, or nil if none exists. It exists so ProcessNext can find
	// its next Job without materializing every stored Job just to scan
	// past the ones that aren't pending.
	NextPending(ctx context.Context) (*Job, error)
}
