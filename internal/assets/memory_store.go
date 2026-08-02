package assets

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// entry is one stored asset. modTime is tracked only so List reports the
// same fields as filesystemStore, which gets it from the filesystem.
type entry struct {
	data    []byte
	modTime time.Time
}

// memoryStore is a Store backed by an in-memory map, safe for concurrent
// use. Its contents are lost on process restart — used by cmd/receipt's
// offline render path (which has no configured asset backend at all) and
// by tests, the same role queue.NewMemoryStore plays for queue.Store. See
// NewFilesystemStore for persistent storage.
type memoryStore struct {
	mu     sync.RWMutex
	assets map[string]entry
}

// NewMemoryStore returns a Store backed by an in-memory map.
func NewMemoryStore() Store {
	return &memoryStore{assets: make(map[string]entry)}
}

func (s *memoryStore) Get(_ context.Context, name string) ([]byte, error) {
	if err := validateName("assets.Store.Get", name); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.assets[name]
	if !ok {
		return nil, apperr.Wrap(apperr.KindNotFound, "assets.Store.Get", fmt.Errorf("asset %q not found", name))
	}
	cp := make([]byte, len(e.data))
	copy(cp, e.data)
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
	s.assets[name] = entry{data: cp, modTime: time.Now()}
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

func (s *memoryStore) List(_ context.Context) ([]Info, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	infos := make([]Info, 0, len(s.assets))
	for name, e := range s.assets {
		infos = append(infos, Info{Name: name, Size: int64(len(e.data)), ModTime: e.modTime})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	return infos, nil
}
