package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/queue"
)

func TestService_QueueSummary_EmptyQueue_ReturnsZeroCounts(t *testing.T) {
	s := newTestService()

	summary, err := s.QueueSummary(context.Background())
	if err != nil {
		t.Fatalf("QueueSummary() error = %v, want nil", err)
	}
	want := app.QueueSummary{}
	if summary != want {
		t.Errorf("QueueSummary() = %+v, want %+v", summary, want)
	}
	if summary.Total() != 0 {
		t.Errorf("QueueSummary().Total() = %d, want 0", summary.Total())
	}
}

func TestService_QueueSummary_CountsJobsByState(t *testing.T) {
	ctx := context.Background()
	store := queue.NewMemoryStore()
	s := app.New(queue.New(store, &noopProcessor{}))

	// One Pending Job via Print (the only supported way to create one
	// through Service, matching how a real Job enters the Queue).
	if _, err := s.Print(ctx, validReceipt(), "front-desk", ""); err != nil {
		t.Fatalf("Print() error = %v, want nil", err)
	}

	// Directly persist Jobs in every other state: QueueSummary only
	// reads via Queue.List, so it must reflect whatever the Store holds
	// regardless of how each Job got there.
	states := []queue.JobState{queue.JobRunning, queue.JobDone, queue.JobFailed, queue.JobCancelled}
	for i, state := range states {
		j := &queue.Job{ID: string(rune('a' + i)), PrinterName: "front-desk", State: state}
		if err := store.Save(ctx, j); err != nil {
			t.Fatalf("store.Save() error = %v, want nil", err)
		}
	}

	summary, err := s.QueueSummary(ctx)
	if err != nil {
		t.Fatalf("QueueSummary() error = %v, want nil", err)
	}
	want := app.QueueSummary{Pending: 1, Running: 1, Done: 1, Failed: 1, Cancelled: 1}
	if summary != want {
		t.Errorf("QueueSummary() = %+v, want %+v", summary, want)
	}
	if summary.Total() != 5 {
		t.Errorf("QueueSummary().Total() = %d, want 5", summary.Total())
	}
}

func TestService_QueueSummary_StoreErrorPropagates(t *testing.T) {
	wantErr := apperr.Wrap(apperr.KindPermanent, "fakeQueueSummaryStore.List", errors.New("disk error"))
	s := app.New(queue.New(&errListStore{listErr: wantErr}, &noopProcessor{}))

	_, err := s.QueueSummary(context.Background())
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("QueueSummary() error = %v, want apperr.KindPermanent", err)
	}
}

// errListStore is a queue.Store test double whose List always returns
// listErr, letting TestService_QueueSummary_StoreErrorPropagates observe
// error propagation without a real Store. Every other method is
// unreachable through QueueSummary and panics if called, so a bug that
// starts calling one is caught immediately rather than silently
// succeeding.
type errListStore struct {
	listErr error
}

func (s *errListStore) Save(context.Context, *queue.Job) error { panic("unexpected Save call") }
func (s *errListStore) Get(context.Context, string) (*queue.Job, error) {
	panic("unexpected Get call")
}
func (s *errListStore) List(context.Context, queue.Filter) ([]*queue.Job, error) {
	return nil, s.listErr
}
func (s *errListStore) NextPending(context.Context) (*queue.Job, error) {
	panic("unexpected NextPending call")
}
func (s *errListStore) ClaimNextPending(context.Context, string) (*queue.Job, error) {
	panic("unexpected ClaimNextPending call")
}
func (s *errListStore) EnqueueIdempotent(context.Context, *queue.Job, time.Time) (*queue.Job, bool, error) {
	panic("unexpected EnqueueIdempotent call")
}
