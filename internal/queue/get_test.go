package queue_test

import (
	"context"
	"errors"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/queue"
)

func TestQueue_Get_ReturnsStoredJob(t *testing.T) {
	store := queue.NewMemoryStore()
	q := queue.New(store, &stubProcessor{})
	ctx := context.Background()
	j := &queue.Job{PrinterName: "front-desk"}
	if err := q.Enqueue(ctx, j); err != nil {
		t.Fatalf("Enqueue() error = %v, want nil", err)
	}

	got, err := q.Get(ctx, j.ID)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.ID != j.ID || got.PrinterName != j.PrinterName {
		t.Errorf("Get() = %+v, want ID=%q PrinterName=%q", *got, j.ID, j.PrinterName)
	}
}

func TestQueue_Get_UnknownID_ReturnsNotFoundError(t *testing.T) {
	q := queue.New(queue.NewMemoryStore(), &stubProcessor{})

	_, err := q.Get(context.Background(), "does-not-exist")
	if !apperr.Is(err, apperr.KindNotFound) {
		t.Fatalf("Get() error = %v, want apperr.KindNotFound", err)
	}
}

func TestQueue_Get_StoreErrorPropagates(t *testing.T) {
	wantErr := apperr.Wrap(apperr.KindPermanent, "fakeCancelStore.Get", errors.New("disk error"))
	store := &fakeCancelStore{getErr: wantErr}
	q := queue.New(store, &stubProcessor{})

	_, err := q.Get(context.Background(), "job-1")
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Get() error = %v, want apperr.KindPermanent", err)
	}
}
