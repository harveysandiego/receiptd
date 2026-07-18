// This file is package queue (not queue_test, unlike most of this
// package's tests) so the persistence-across-reopen test can reach the
// concrete *boltStore's Close method directly: NewBoltStore returns the
// Store interface, which deliberately has no Close (see
// docs/ARCHITECTURE.md §2), so closing the underlying bbolt.DB to
// simulate a restart requires the unexported type.
package queue

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

func TestBoltStore_SaveThenGet_RoundTrips(t *testing.T) {
	s := newTestBoltStore(t)
	ctx := context.Background()
	want := &Job{ID: "job-1", PrinterName: "front-desk", State: JobPending}

	if err := s.Save(ctx, want); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}
	got, err := s.Get(ctx, "job-1")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.ID != want.ID || got.PrinterName != want.PrinterName || got.State != want.State {
		t.Errorf("Get() = %+v, want %+v", *got, *want)
	}
}

func TestBoltStore_Get_UnknownID_ReturnsNotFoundError(t *testing.T) {
	s := newTestBoltStore(t)
	_, err := s.Get(context.Background(), "does-not-exist")
	if !apperr.Is(err, apperr.KindNotFound) {
		t.Fatalf("Get() error = %v, want apperr.KindNotFound", err)
	}
}

func TestBoltStore_Save_OverwritesExistingID(t *testing.T) {
	s := newTestBoltStore(t)
	ctx := context.Background()

	if err := s.Save(ctx, &Job{ID: "job-1", State: JobPending}); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}
	if err := s.Save(ctx, &Job{ID: "job-1", State: JobDone}); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	got, err := s.Get(ctx, "job-1")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.State != JobDone {
		t.Errorf("Get().State = %v, want %v", got.State, JobDone)
	}
}

func TestBoltStore_List_ReturnsAllSavedJobs(t *testing.T) {
	s := newTestBoltStore(t)
	ctx := context.Background()
	for _, id := range []string{"job-2", "job-1", "job-3"} {
		if err := s.Save(ctx, &Job{ID: id}); err != nil {
			t.Fatalf("Save(%q) error = %v, want nil", id, err)
		}
	}

	got, err := s.List(ctx, Filter{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(List()) = %d, want 3", len(got))
	}
	var ids []string
	for _, j := range got {
		ids = append(ids, j.ID)
	}
	want := []string{"job-1", "job-2", "job-3"}
	for i, id := range want {
		if ids[i] != id {
			t.Errorf("List()[%d].ID = %q, want %q (got order %v)", i, ids[i], id, ids)
		}
	}
}

func TestBoltStore_List_EmptyStore(t *testing.T) {
	s := newTestBoltStore(t)
	got, err := s.List(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("len(List()) = %d, want 0", len(got))
	}
}

func TestBoltStore_Get_DoesNotAliasStoredJob(t *testing.T) {
	s := newTestBoltStore(t)
	ctx := context.Background()
	if err := s.Save(ctx, &Job{ID: "job-1", State: JobPending}); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	got, err := s.Get(ctx, "job-1")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	got.State = JobFailed

	again, err := s.Get(ctx, "job-1")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if again.State != JobPending {
		t.Errorf("Get().State = %v after mutating a previous Get() result, want unchanged %v", again.State, JobPending)
	}
}

func TestBoltStore_Save_DoesNotAliasCallersJob(t *testing.T) {
	s := newTestBoltStore(t)
	ctx := context.Background()
	j := &Job{ID: "job-1", State: JobPending}
	if err := s.Save(ctx, j); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	j.State = JobFailed

	got, err := s.Get(ctx, "job-1")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.State != JobPending {
		t.Errorf("Get().State = %v after mutating the Job passed to Save(), want unchanged %v", got.State, JobPending)
	}
}

func TestBoltStore_SurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.db")
	ctx := context.Background()

	s, err := NewBoltStore(path)
	if err != nil {
		t.Fatalf("NewBoltStore() error = %v, want nil", err)
	}
	if err := s.Save(ctx, &Job{ID: "job-1", PrinterName: "front-desk", State: JobRunning, Attempts: 2}); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}
	if err := s.(*boltStore).Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	reopened, err := NewBoltStore(path)
	if err != nil {
		t.Fatalf("NewBoltStore() (reopen) error = %v, want nil", err)
	}
	defer func() { _ = reopened.(*boltStore).Close() }()

	got, err := reopened.Get(ctx, "job-1")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.PrinterName != "front-desk" || got.State != JobRunning || got.Attempts != 2 {
		t.Errorf("Get() = %+v, want PrinterName=front-desk State=running Attempts=2", *got)
	}
}

func TestNewBoltStore_InvalidPath_ReturnsPermanentError(t *testing.T) {
	// A path inside a nonexistent directory can't be opened by bbolt.
	_, err := NewBoltStore(filepath.Join(t.TempDir(), "no-such-dir", "queue.db"))
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("NewBoltStore() error = %v, want apperr.KindPermanent", err)
	}
}

func TestBoltStore_ConcurrentSaveAndGet(t *testing.T) {
	s := newTestBoltStore(t)
	ctx := context.Background()
	const n = 50

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("job-%d", i)
			_ = s.Save(ctx, &Job{ID: id, State: JobPending})
			_, _ = s.Get(ctx, id)
			_, _ = s.List(ctx, Filter{})
		}(i)
	}
	wg.Wait()
}

// newTestBoltStore returns a boltStore backed by a temp file, closed
// automatically via t.Cleanup.
func newTestBoltStore(t *testing.T) Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "queue.db")
	s, err := NewBoltStore(path)
	if err != nil {
		t.Fatalf("NewBoltStore() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		_ = s.(*boltStore).Close()
	})
	return s
}
