package assets

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// memoryStore is a Store backed by an in-memory map, safe for concurrent
// use. Its contents are lost on process restart — used by cmd/receipt's
// offline render path (which has no configured asset backend at all) and
// by tests, the same role queue.NewMemoryStore plays for queue.Store. See
// NewFilesystemStore for persistent storage.
type memoryStore struct {
	mu     sync.RWMutex
	assets map[string][]byte
}

// NewMemoryStore returns a Store backed by an in-memory map.
func NewMemoryStore() Store {
	return &memoryStore{assets: make(map[string][]byte)}
}

func (s *memoryStore) Get(_ context.Context, name string) ([]byte, error) {
	if err := validateName("assets.Store.Get", name); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.assets[name]
	if !ok {
		return nil, apperr.Wrap(apperr.KindNotFound, "assets.Store.Get", fmt.Errorf("asset %q not found", name))
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	return cp, nil
}

func (s *memoryStore) Put(_ context.Context, name string, data []byte) error {
	if err := validateName("assets.Store.Put", name); err != nil {
		return err
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assets[name] = cp
	return nil
}

func (s *memoryStore) Delete(_ context.Context, name string) error {
	if err := validateName("assets.Store.Delete", name); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.assets[name]; !ok {
		return apperr.Wrap(apperr.KindNotFound, "assets.Store.Delete", fmt.Errorf("asset %q not found", name))
	}
	delete(s.assets, name)
	return nil
}

func (s *memoryStore) List(_ context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.assets))
	for name := range s.assets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
