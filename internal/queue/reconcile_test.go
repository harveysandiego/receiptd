package queue_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/queue"
)

// reconcileStores runs each Reconcile test against both storage backends —
// business logic layered on List/Save, which both already implement
// correctly, so this is testing Reconcile itself, not re-testing the
// stores (docs/adr/0017-queue-lifecycle-crash-recovery.md).
func reconcileStores(t *testing.T) map[string]queue.Store {
	t.Helper()
	boltStore, err := queue.NewBoltStore(t.TempDir() + "/queue.db")
	if err != nil {
		t.Fatalf("queue.NewBoltStore() error = %v", err)
	}
	t.Cleanup(func() {
		if closer, ok := boltStore.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})
	return map[string]queue.Store{
		"memory": queue.NewMemoryStore(),
		"bbolt":  boltStore,
	}
}

func seedJob(t *testing.T, ctx context.Context, store queue.Store, j *queue.Job) {
	t.Helper()
	if err := store.Save(ctx, j); err != nil {
		t.Fatalf("store.Save() error = %v", err)
	}
}

func TestQueue_Reconcile_RunningUnderBudget_ReturnsToPending(t *testing.T) {
	for name, store := range reconcileStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			seedJob(t, ctx, store, &queue.Job{ID: "job-1", PrinterName: "front-desk", State: queue.JobRunning, Attempts: 0})
			q := queue.NewWithRetry(store, noopProcessor{}, 3, time.Second)

			retried, failed, err := q.Reconcile(ctx)
			if err != nil {
				t.Fatalf("Reconcile() error = %v, want nil", err)
			}
			if retried != 1 || failed != 0 {
				t.Errorf("Reconcile() = (%d, %d), want (1, 0)", retried, failed)
			}

			got, err := store.Get(ctx, "job-1")
			if err != nil {
				t.Fatalf("store.Get() error = %v", err)
			}
			if got.State != queue.JobPending {
				t.Errorf("State = %v, want %v", got.State, queue.JobPending)
			}
			if got.Attempts != 1 {
				t.Errorf("Attempts = %d, want 1", got.Attempts)
			}
			if !strings.Contains(got.LastError, "interrupted") || !strings.Contains(got.LastError, "attempt 1 of 3") {
				t.Errorf("LastError = %q, want it to mention the interruption and attempt count", got.LastError)
			}
		})
	}
}

func TestQueue_Reconcile_RunningAtBudget_Fails(t *testing.T) {
	for name, store := range reconcileStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			// Attempts is already 2 of 3; this interruption is the third and
			// final one, so it must land in Failed, not Pending.
			seedJob(t, ctx, store, &queue.Job{ID: "job-1", PrinterName: "front-desk", State: queue.JobRunning, Attempts: 2})
			q := queue.NewWithRetry(store, noopProcessor{}, 3, time.Second)

			retried, failed, err := q.Reconcile(ctx)
			if err != nil {
				t.Fatalf("Reconcile() error = %v, want nil", err)
			}
			if retried != 0 || failed != 1 {
				t.Errorf("Reconcile() = (%d, %d), want (0, 1)", retried, failed)
			}

			got, err := store.Get(ctx, "job-1")
			if err != nil {
				t.Fatalf("store.Get() error = %v", err)
			}
			if got.State != queue.JobFailed {
				t.Errorf("State = %v, want %v", got.State, queue.JobFailed)
			}
			if got.Attempts != 3 {
				t.Errorf("Attempts = %d, want 3", got.Attempts)
			}
		})
	}
}

// TestQueue_Reconcile_RunningOneAttemptShortOfBudget_ReturnsToPending pins
// the boundary case explicitly: an incremented Attempts one below
// maxAttempts must still be requeued, not failed.
func TestQueue_Reconcile_RunningOneAttemptShortOfBudget_ReturnsToPending(t *testing.T) {
	for name, store := range reconcileStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			// Attempts is 1 of 3; incremented to 2, still < 3.
			seedJob(t, ctx, store, &queue.Job{ID: "job-1", PrinterName: "front-desk", State: queue.JobRunning, Attempts: 1})
			q := queue.NewWithRetry(store, noopProcessor{}, 3, time.Second)

			retried, failed, err := q.Reconcile(ctx)
			if err != nil {
				t.Fatalf("Reconcile() error = %v, want nil", err)
			}
			if retried != 1 || failed != 0 {
				t.Errorf("Reconcile() = (%d, %d), want (1, 0)", retried, failed)
			}

			got, err := store.Get(ctx, "job-1")
			if err != nil {
				t.Fatalf("store.Get() error = %v", err)
			}
			if got.State != queue.JobPending {
				t.Errorf("State = %v, want %v", got.State, queue.JobPending)
			}
			if got.Attempts != 2 {
				t.Errorf("Attempts = %d, want 2", got.Attempts)
			}
		})
	}
}

func TestQueue_Reconcile_NonRunningJobs_Untouched(t *testing.T) {
	for name, store := range reconcileStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			pending := &queue.Job{ID: "job-pending", PrinterName: "front-desk", State: queue.JobPending, Attempts: 0}
			done := &queue.Job{ID: "job-done", PrinterName: "front-desk", State: queue.JobDone, Attempts: 1}
			failedJob := &queue.Job{ID: "job-failed", PrinterName: "front-desk", State: queue.JobFailed, Attempts: 3, LastError: "printer offline"}
			cancelled := &queue.Job{ID: "job-cancelled", PrinterName: "front-desk", State: queue.JobCancelled, Attempts: 0}
			for _, j := range []*queue.Job{pending, done, failedJob, cancelled} {
				seedJob(t, ctx, store, j)
			}
			q := queue.NewWithRetry(store, noopProcessor{}, 3, time.Second)

			retried, failed, err := q.Reconcile(ctx)
			if err != nil {
				t.Fatalf("Reconcile() error = %v, want nil", err)
			}
			if retried != 0 || failed != 0 {
				t.Errorf("Reconcile() = (%d, %d), want (0, 0)", retried, failed)
			}

			for _, want := range []*queue.Job{pending, done, failedJob, cancelled} {
				got, err := store.Get(ctx, want.ID)
				if err != nil {
					t.Fatalf("store.Get(%q) error = %v", want.ID, err)
				}
				if got.State != want.State || got.Attempts != want.Attempts || got.LastError != want.LastError {
					t.Errorf("job %q = %+v, want unchanged from %+v", want.ID, got, want)
				}
			}
		})
	}
}

func TestQueue_Reconcile_MultipleRunningJobsAcrossPrinters_AllReconciled(t *testing.T) {
	for name, store := range reconcileStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			frontDesk := &queue.Job{ID: "job-front", PrinterName: "front-desk", State: queue.JobRunning, Attempts: 0}
			kitchen := &queue.Job{ID: "job-kitchen", PrinterName: "kitchen", State: queue.JobRunning, Attempts: 2}
			for _, j := range []*queue.Job{frontDesk, kitchen} {
				seedJob(t, ctx, store, j)
			}
			q := queue.NewWithRetry(store, noopProcessor{}, 3, time.Second)

			retried, failed, err := q.Reconcile(ctx)
			if err != nil {
				t.Fatalf("Reconcile() error = %v, want nil", err)
			}
			if retried != 1 || failed != 1 {
				t.Errorf("Reconcile() = (%d, %d), want (1, 1)", retried, failed)
			}

			got, err := store.Get(ctx, "job-front")
			if err != nil {
				t.Fatalf("store.Get() error = %v", err)
			}
			if got.State != queue.JobPending {
				t.Errorf("job-front State = %v, want %v", got.State, queue.JobPending)
			}

			got, err = store.Get(ctx, "job-kitchen")
			if err != nil {
				t.Fatalf("store.Get() error = %v", err)
			}
			if got.State != queue.JobFailed {
				t.Errorf("job-kitchen State = %v, want %v", got.State, queue.JobFailed)
			}
		})
	}
}

func TestQueue_Reconcile_NoRunningJobs_IsNoOp(t *testing.T) {
	for name, store := range reconcileStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			seedJob(t, ctx, store, &queue.Job{ID: "job-1", PrinterName: "front-desk", State: queue.JobPending})
			q := queue.NewWithRetry(store, noopProcessor{}, 3, time.Second)

			retried, failed, err := q.Reconcile(ctx)
			if err != nil {
				t.Fatalf("Reconcile() error = %v, want nil", err)
			}
			if retried != 0 || failed != 0 {
				t.Errorf("Reconcile() = (%d, %d), want (0, 0)", retried, failed)
			}
		})
	}
}

// listErrStore is a minimal Store test double whose List always fails, so
// Reconcile's error-propagation path can be tested independent of either
// real backend's own List behavior.
type listErrStore struct {
	queue.Store
	err error
}

func (s listErrStore) List(context.Context, queue.Filter) ([]*queue.Job, error) {
	return nil, s.err
}

func TestQueue_Reconcile_ListErrorPropagates(t *testing.T) {
	wantErr := apperr.Wrap(apperr.KindPermanent, "test", errors.New("disk error"))
	store := listErrStore{Store: queue.NewMemoryStore(), err: wantErr}
	q := queue.NewWithRetry(store, noopProcessor{}, 3, time.Second)

	_, _, err := q.Reconcile(context.Background())
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Reconcile() error = %v, want apperr.KindPermanent", err)
	}
}

// saveErrStore is a minimal Store test double whose Save always fails,
// wrapping a real backend for List, so Reconcile's error-propagation from
// Save can be tested independent of either real backend's own Save
// behavior.
type saveErrStore struct {
	queue.Store
	err error
}

func (s saveErrStore) Save(context.Context, *queue.Job) error {
	return s.err
}

func TestQueue_Reconcile_SaveErrorPropagates(t *testing.T) {
	ctx := context.Background()
	inner := queue.NewMemoryStore()
	if err := inner.Save(ctx, &queue.Job{ID: "job-1", PrinterName: "front-desk", State: queue.JobRunning}); err != nil {
		t.Fatalf("inner.Save() error = %v", err)
	}
	wantErr := apperr.Wrap(apperr.KindPermanent, "test", errors.New("disk full"))
	store := saveErrStore{Store: inner, err: wantErr}
	q := queue.NewWithRetry(store, noopProcessor{}, 3, time.Second)

	_, _, err := q.Reconcile(ctx)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Reconcile() error = %v, want apperr.KindPermanent", err)
	}
}

// noopProcessor is a queue.Processor test double for tests that only
// exercise Reconcile/Store paths and never actually invoke Process.
type noopProcessor struct{}

func (noopProcessor) Process(context.Context, *queue.Job) error { return nil }
