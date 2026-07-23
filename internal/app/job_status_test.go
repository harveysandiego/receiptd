package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/queue"
)

func TestService_JobStatus_ExistingJob_ReturnsJob(t *testing.T) {
	store := queue.NewMemoryStore()
	s := app.New(queue.New(store, &noopProcessor{}))
	ctx := context.Background()

	jobID, err := s.Print(ctx, validReceipt(), "front-desk", "")
	if err != nil {
		t.Fatalf("Print() error = %v, want nil", err)
	}

	got, err := s.JobStatus(ctx, jobID)
	if err != nil {
		t.Fatalf("JobStatus() error = %v, want nil", err)
	}
	if got.ID != jobID {
		t.Errorf("JobStatus().ID = %q, want %q", got.ID, jobID)
	}
	if got.State != queue.JobPending {
		t.Errorf("JobStatus().State = %v, want %v", got.State, queue.JobPending)
	}
}

func TestService_JobStatus_UnknownJob_ReturnsNotFoundError(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))

	got, err := s.JobStatus(context.Background(), "does-not-exist")
	if !apperr.Is(err, apperr.KindNotFound) {
		t.Fatalf("JobStatus() error = %v, want apperr.KindNotFound", err)
	}
	if got != nil {
		t.Errorf("JobStatus() = %v, want nil on error", got)
	}
}

func TestService_JobStatus_StoreErrorPropagates(t *testing.T) {
	wantErr := apperr.Wrap(apperr.KindPermanent, "fakeStore.Get", errors.New("disk error"))
	store := &fakeStore{getErr: wantErr}
	s := app.New(queue.New(store, &noopProcessor{}))

	_, err := s.JobStatus(context.Background(), "job-1")
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("JobStatus() error = %v, want apperr.KindPermanent", err)
	}
}

func TestService_JobStatus_DoesNotModifyQueueState(t *testing.T) {
	store := queue.NewMemoryStore()
	s := app.New(queue.New(store, &noopProcessor{}))
	ctx := context.Background()

	jobID, err := s.Print(ctx, validReceipt(), "front-desk", "")
	if err != nil {
		t.Fatalf("Print() error = %v, want nil", err)
	}
	before, err := store.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("store.Get() error = %v, want nil", err)
	}

	if _, err := s.JobStatus(ctx, jobID); err != nil {
		t.Fatalf("JobStatus() error = %v, want nil", err)
	}

	after, err := store.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("store.Get() error = %v, want nil", err)
	}
	if after.State != before.State || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("JobStatus() changed stored Job: before = %+v, after = %+v", *before, *after)
	}
}

func TestService_JobStatus_DoesNotInvokeProcessor(t *testing.T) {
	store := queue.NewMemoryStore()
	proc := &noopProcessor{}
	s := app.New(queue.New(store, proc))
	ctx := context.Background()

	jobID, err := s.Print(ctx, validReceipt(), "front-desk", "")
	if err != nil {
		t.Fatalf("Print() error = %v, want nil", err)
	}

	if _, err := s.JobStatus(ctx, jobID); err != nil {
		t.Fatalf("JobStatus() error = %v, want nil", err)
	}
	if proc.calls != 0 {
		t.Errorf("proc.calls = %d, want 0 (JobStatus must never invoke the Processor)", proc.calls)
	}
}
