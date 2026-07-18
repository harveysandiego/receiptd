package queue_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/queue"
)

func TestQueue_Enqueue_PersistsJobInStore(t *testing.T) {
	store := queue.NewMemoryStore()
	q := queue.New(store)
	ctx := context.Background()
	j := &queue.Job{PrinterName: "front-desk"}

	if err := q.Enqueue(ctx, j); err != nil {
		t.Fatalf("Enqueue() error = %v, want nil", err)
	}

	got, err := store.Get(ctx, j.ID)
	if err != nil {
		t.Fatalf("store.Get(%q) error = %v, want nil", j.ID, err)
	}
	if got.PrinterName != j.PrinterName {
		t.Errorf("store.Get().PrinterName = %q, want %q", got.PrinterName, j.PrinterName)
	}
}

func TestQueue_Enqueue_SetsInitialState(t *testing.T) {
	store := queue.NewMemoryStore()
	q := queue.New(store)
	j := &queue.Job{State: queue.JobDone} // pre-set to a non-pending state

	if err := q.Enqueue(context.Background(), j); err != nil {
		t.Fatalf("Enqueue() error = %v, want nil", err)
	}
	if j.State != queue.JobPending {
		t.Errorf("j.State = %v, want %v", j.State, queue.JobPending)
	}
}

func TestQueue_Enqueue_SetsTimestamps(t *testing.T) {
	store := queue.NewMemoryStore()
	q := queue.New(store)
	j := &queue.Job{}

	before := time.Now()
	if err := q.Enqueue(context.Background(), j); err != nil {
		t.Fatalf("Enqueue() error = %v, want nil", err)
	}
	after := time.Now()

	if j.CreatedAt.Before(before) || j.CreatedAt.After(after) {
		t.Errorf("j.CreatedAt = %v, want between %v and %v", j.CreatedAt, before, after)
	}
	if j.UpdatedAt.Before(before) || j.UpdatedAt.After(after) {
		t.Errorf("j.UpdatedAt = %v, want between %v and %v", j.UpdatedAt, before, after)
	}
}

func TestQueue_Enqueue_GeneratesID(t *testing.T) {
	store := queue.NewMemoryStore()
	q := queue.New(store)
	j := &queue.Job{}

	if err := q.Enqueue(context.Background(), j); err != nil {
		t.Fatalf("Enqueue() error = %v, want nil", err)
	}
	if j.ID == "" {
		t.Error("j.ID = \"\", want a generated ID")
	}
}

func TestQueue_Enqueue_MultipleJobsGetDistinctIDs(t *testing.T) {
	store := queue.NewMemoryStore()
	q := queue.New(store)
	ctx := context.Background()

	j1 := &queue.Job{}
	j2 := &queue.Job{}
	if err := q.Enqueue(ctx, j1); err != nil {
		t.Fatalf("Enqueue(j1) error = %v, want nil", err)
	}
	if err := q.Enqueue(ctx, j2); err != nil {
		t.Fatalf("Enqueue(j2) error = %v, want nil", err)
	}

	if j1.ID == j2.ID {
		t.Errorf("j1.ID == j2.ID == %q, want distinct IDs", j1.ID)
	}
}

func TestQueue_Enqueue_StoreErrorPropagates(t *testing.T) {
	wantErr := apperr.Wrap(apperr.KindPermanent, "fakeStore.Save", errors.New("disk full"))
	store := &fakeStore{saveErr: wantErr}
	q := queue.New(store)

	err := q.Enqueue(context.Background(), &queue.Job{})
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Enqueue() error = %v, want apperr.KindPermanent", err)
	}
}

type fakeStore struct {
	saveErr error
}

func (f *fakeStore) Save(_ context.Context, _ *queue.Job) error { return f.saveErr }

func (f *fakeStore) Get(_ context.Context, _ string) (*queue.Job, error) {
	return nil, nil
}

func (f *fakeStore) List(_ context.Context, _ queue.Filter) ([]*queue.Job, error) {
	return nil, nil
}
