package queue_test

import (
	"context"
	"errors"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/queue"
)

func TestQueue_Cancel_PendingJob_TransitionsToCancelled(t *testing.T) {
	store := queue.NewMemoryStore()
	q := queue.New(store, &stubProcessor{})
	ctx := context.Background()
	j := &queue.Job{PrinterName: "front-desk"}
	mustEnqueue(t, q, j)

	if err := q.Cancel(ctx, j.ID); err != nil {
		t.Fatalf("Cancel() error = %v, want nil", err)
	}

	got, err := store.Get(ctx, j.ID)
	if err != nil {
		t.Fatalf("store.Get(%q) error = %v, want nil", j.ID, err)
	}
	if got.State != queue.JobCancelled {
		t.Errorf("store.Get().State = %v, want %v", got.State, queue.JobCancelled)
	}
}

func TestQueue_Cancel_PendingJob_UpdatesTimestamp(t *testing.T) {
	store := queue.NewMemoryStore()
	q := queue.New(store, &stubProcessor{})
	ctx := context.Background()
	j := &queue.Job{PrinterName: "front-desk"}
	mustEnqueue(t, q, j)
	before := j.UpdatedAt

	if err := q.Cancel(ctx, j.ID); err != nil {
		t.Fatalf("Cancel() error = %v, want nil", err)
	}

	got, err := store.Get(ctx, j.ID)
	if err != nil {
		t.Fatalf("store.Get(%q) error = %v, want nil", j.ID, err)
	}
	if got.UpdatedAt.Before(before) {
		t.Errorf("got.UpdatedAt = %v, want at or after %v", got.UpdatedAt, before)
	}
}

func TestQueue_Cancel_NonPendingStates_ReturnsValidationErrorAndLeavesStateUnchanged(t *testing.T) {
	tests := []struct {
		name  string
		state queue.JobState
	}{
		{"running", queue.JobRunning},
		{"done", queue.JobDone},
		{"failed", queue.JobFailed},
		{"cancelled", queue.JobCancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := queue.NewMemoryStore()
			q := queue.New(store, &stubProcessor{})
			ctx := context.Background()
			j := &queue.Job{ID: "job-1", State: tt.state}
			if err := store.Save(ctx, j); err != nil {
				t.Fatalf("store.Save() error = %v, want nil", err)
			}

			err := q.Cancel(ctx, j.ID)
			if !apperr.Is(err, apperr.KindValidation) {
				t.Fatalf("Cancel() error = %v, want apperr.KindValidation", err)
			}

			got, getErr := store.Get(ctx, j.ID)
			if getErr != nil {
				t.Fatalf("store.Get(%q) error = %v, want nil", j.ID, getErr)
			}
			if got.State != tt.state {
				t.Errorf("store.Get().State = %v, want unchanged %v", got.State, tt.state)
			}
		})
	}
}

func TestQueue_Cancel_UnknownJob_ReturnsNotFoundError(t *testing.T) {
	store := queue.NewMemoryStore()
	q := queue.New(store, &stubProcessor{})

	err := q.Cancel(context.Background(), "does-not-exist")
	if !apperr.Is(err, apperr.KindNotFound) {
		t.Fatalf("Cancel() error = %v, want apperr.KindNotFound", err)
	}
}

func TestQueue_Cancel_StoreGetErrorPropagates(t *testing.T) {
	wantErr := apperr.Wrap(apperr.KindPermanent, "fakeCancelStore.Get", errors.New("disk error"))
	store := &fakeCancelStore{getErr: wantErr}
	q := queue.New(store, &stubProcessor{})

	err := q.Cancel(context.Background(), "job-1")
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Cancel() error = %v, want apperr.KindPermanent", err)
	}
}

func TestQueue_Cancel_StoreSaveErrorPropagates(t *testing.T) {
	wantErr := apperr.Wrap(apperr.KindPermanent, "fakeCancelStore.Save", errors.New("disk full"))
	store := &fakeCancelStore{
		getJob:  &queue.Job{ID: "job-1", State: queue.JobPending},
		saveErr: wantErr,
	}
	q := queue.New(store, &stubProcessor{})

	err := q.Cancel(context.Background(), "job-1")
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Cancel() error = %v, want apperr.KindPermanent", err)
	}
}

// fakeCancelStore is a Store test double giving Cancel's tests control
// over what Get returns (a specific Job or a specific error) independent
// of Save's outcome. queue_test.go's fakeStore only lets Save fail, which
// isn't enough to test Cancel's Get-then-Save sequence in isolation.
type fakeCancelStore struct {
	getJob  *queue.Job
	getErr  error
	saveErr error
}

func (f *fakeCancelStore) Save(_ context.Context, _ *queue.Job) error { return f.saveErr }

func (f *fakeCancelStore) Get(_ context.Context, _ string) (*queue.Job, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.getJob, nil
}

func (f *fakeCancelStore) List(_ context.Context, _ queue.Filter) ([]*queue.Job, error) {
	return nil, nil
}
