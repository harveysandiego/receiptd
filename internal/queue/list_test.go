package queue_test

import (
	"context"
	"errors"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/queue"
)

func TestQueue_List_ReturnsEveryJob(t *testing.T) {
	store := queue.NewMemoryStore()
	q := queue.New(store, &stubProcessor{})
	ctx := context.Background()

	first := &queue.Job{PrinterName: "front-desk"}
	second := &queue.Job{PrinterName: "kitchen"}
	if err := q.Enqueue(ctx, first); err != nil {
		t.Fatalf("Enqueue() error = %v, want nil", err)
	}
	if err := q.Enqueue(ctx, second); err != nil {
		t.Fatalf("Enqueue() error = %v, want nil", err)
	}

	jobs, err := q.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(List()) = %d, want 2", len(jobs))
	}
}

func TestQueue_List_EmptyStore_ReturnsNoJobs(t *testing.T) {
	q := queue.New(queue.NewMemoryStore(), &stubProcessor{})

	jobs, err := q.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(List()) = %d, want 0", len(jobs))
	}
}

func TestQueue_List_StoreErrorPropagates(t *testing.T) {
	wantErr := apperr.Wrap(apperr.KindPermanent, "fakeCancelStore.List", errors.New("disk error"))
	store := &fakeCancelStore{listErr: wantErr}
	q := queue.New(store, &stubProcessor{})

	_, err := q.List(context.Background())
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("List() error = %v, want apperr.KindPermanent", err)
	}
}
