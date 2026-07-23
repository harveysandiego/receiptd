package queue

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

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

// ClaimNextPending runs entirely under one s.mu.Lock() critical section,
// not a read-lock then a separate write-lock, closing the race
// docs/adr/0016-queue-concurrency-per-printer-workers.md requires. next
// aliases s.jobs, so mutating it persists the transition; the returned
// copy does not, matching every other Store method's no-alias contract.
func (s *memoryStore) ClaimNextPending(_ context.Context, printerName string) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var next *Job
	for _, j := range s.jobs {
		if j.State != JobPending || j.PrinterName != printerName {
			continue
		}
		if next == nil || j.ID < next.ID {
			next = j
		}
	}
	if next == nil {
		return nil, nil
	}
	next.State = JobRunning
	next.UpdatedAt = time.Now()
	cp := *next
	return &cp, nil
}

// EnqueueIdempotent runs entirely under s.mu, the same single-critical-
// section discipline Save/Get/NextPending use. See Store.EnqueueIdempotent
// for the behavior implemented here.
func (s *memoryStore) EnqueueIdempotent(_ context.Context, newJob *Job, now time.Time) (*Job, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if newJob.IdempotencyKey != "" {
		if existing := s.findByIdempotencyKeyLocked(newJob.IdempotencyKey, now); existing != nil {
			if !sameLogicalRequest(existing, newJob) {
				return nil, false, apperr.Wrap(apperr.KindValidation, "queue.Store.EnqueueIdempotent",
					fmt.Errorf("idempotency key %q already used for a different printer or receipt", newJob.IdempotencyKey))
			}

			result := *existing
			if existing.State == JobFailed {
				result.State = JobPending
				result.Attempts = 0
				result.LastError = ""
				result.UpdatedAt = now
				stored := result
				s.jobs[result.ID] = &stored
			}
			out := result
			return &out, false, nil
		}
	}

	id, err := newJobID()
	if err != nil {
		return nil, false, apperr.Wrap(apperr.KindPermanent, "queue.Store.EnqueueIdempotent", err)
	}
	newJob.ID = id
	newJob.State = JobPending
	newJob.CreatedAt = now
	newJob.UpdatedAt = now

	cp := *newJob
	s.jobs[newJob.ID] = &cp
	out := cp
	return &out, true, nil
}

// findByIdempotencyKeyLocked returns the Job stored under key, ignoring an
// expired one, or nil. Callers must hold s.mu; the returned *Job aliases
// Store state, safe only because every caller is in this file and
// EnqueueIdempotent copies before returning it further.
func (s *memoryStore) findByIdempotencyKeyLocked(key string, now time.Time) *Job {
	for _, j := range s.jobs {
		if j.IdempotencyKey == key && !idempotencyKeyExpired(j, now) {
			return j
		}
	}
	return nil
}
