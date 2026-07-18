package queue_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/queue"
)

func TestMemoryStore_SaveThenGet_RoundTrips(t *testing.T) {
	s := queue.NewMemoryStore()
	ctx := context.Background()
	want := &queue.Job{ID: "job-1", PrinterName: "front-desk", State: queue.JobPending}

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

func TestMemoryStore_Get_UnknownID_ReturnsNotFoundError(t *testing.T) {
	s := queue.NewMemoryStore()
	_, err := s.Get(context.Background(), "does-not-exist")
	if !apperr.Is(err, apperr.KindNotFound) {
		t.Fatalf("Get() error = %v, want apperr.KindNotFound", err)
	}
}

func TestMemoryStore_Save_OverwritesExistingID(t *testing.T) {
	s := queue.NewMemoryStore()
	ctx := context.Background()

	if err := s.Save(ctx, &queue.Job{ID: "job-1", State: queue.JobPending}); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}
	if err := s.Save(ctx, &queue.Job{ID: "job-1", State: queue.JobDone}); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	got, err := s.Get(ctx, "job-1")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.State != queue.JobDone {
		t.Errorf("Get().State = %v, want %v", got.State, queue.JobDone)
	}
}

func TestMemoryStore_List_ReturnsAllSavedJobs(t *testing.T) {
	s := queue.NewMemoryStore()
	ctx := context.Background()
	for _, id := range []string{"job-2", "job-1", "job-3"} {
		if err := s.Save(ctx, &queue.Job{ID: id}); err != nil {
			t.Fatalf("Save(%q) error = %v, want nil", id, err)
		}
	}

	got, err := s.List(ctx, queue.Filter{})
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

func TestMemoryStore_List_EmptyStore(t *testing.T) {
	s := queue.NewMemoryStore()
	got, err := s.List(context.Background(), queue.Filter{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("len(List()) = %d, want 0", len(got))
	}
}

func TestMemoryStore_Get_DoesNotAliasStoredJob(t *testing.T) {
	s := queue.NewMemoryStore()
	ctx := context.Background()
	if err := s.Save(ctx, &queue.Job{ID: "job-1", State: queue.JobPending}); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	got, err := s.Get(ctx, "job-1")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	got.State = queue.JobFailed

	again, err := s.Get(ctx, "job-1")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if again.State != queue.JobPending {
		t.Errorf("Get().State = %v after mutating a previous Get() result, want unchanged %v", again.State, queue.JobPending)
	}
}

func TestMemoryStore_Save_DoesNotAliasCallersJob(t *testing.T) {
	s := queue.NewMemoryStore()
	ctx := context.Background()
	j := &queue.Job{ID: "job-1", State: queue.JobPending}
	if err := s.Save(ctx, j); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	j.State = queue.JobFailed

	got, err := s.Get(ctx, "job-1")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.State != queue.JobPending {
		t.Errorf("Get().State = %v after mutating the Job passed to Save(), want unchanged %v", got.State, queue.JobPending)
	}
}

func TestMemoryStore_ConcurrentSaveAndGet(t *testing.T) {
	s := queue.NewMemoryStore()
	ctx := context.Background()
	const n = 50

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("job-%d", i)
			_ = s.Save(ctx, &queue.Job{ID: id, State: queue.JobPending})
			_, _ = s.Get(ctx, id)
			_, _ = s.List(ctx, queue.Filter{})
		}(i)
	}
	wg.Wait()
}
