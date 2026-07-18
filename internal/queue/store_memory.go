package queue

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// memoryStore is a Store backed by an in-memory map, safe for concurrent
// use. Its contents are lost on process restart — see NewBoltStore for
// persistent storage.
type memoryStore struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

// NewMemoryStore returns a Store backed by an in-memory map.
func NewMemoryStore() Store {
	return &memoryStore{jobs: make(map[string]*Job)}
}

func (s *memoryStore) Save(_ context.Context, j *Job) error {
	cp := *j
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[j.ID] = &cp
	return nil
}

func (s *memoryStore) Get(_ context.Context, id string) (*Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, apperr.Wrap(apperr.KindNotFound, "queue.Store.Get", fmt.Errorf("job %q not found", id))
	}
	cp := *j
	return &cp, nil
}

func (s *memoryStore) List(_ context.Context, _ Filter) ([]*Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		cp := *j
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out, nil
}

func (s *memoryStore) NextPending(_ context.Context) (*Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var next *Job
	for _, j := range s.jobs {
		if j.State != JobPending {
			continue
		}
		if next == nil || j.ID < next.ID {
			next = j
		}
	}
	if next == nil {
		return nil, nil
	}
	cp := *next
	return &cp, nil
}
