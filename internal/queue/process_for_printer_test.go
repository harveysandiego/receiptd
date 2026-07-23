package queue_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/queue"
)

func TestQueue_ProcessNextForPrinter_ProcessesPendingJobForThatPrinter_ResultsInJobDone(t *testing.T) {
	store := queue.NewMemoryStore()
	proc := &stubProcessor{}
	q := queue.New(store, proc)
	ctx := context.Background()
	j := &queue.Job{PrinterName: "front-desk"}
	mustEnqueue(t, q, j)

	if err := q.ProcessNextForPrinter(ctx, "front-desk"); err != nil {
		t.Fatalf("ProcessNextForPrinter() error = %v, want nil", err)
	}
	if proc.calls != 1 {
		t.Errorf("proc.calls = %d, want 1", proc.calls)
	}

	got, err := store.Get(ctx, j.ID)
	if err != nil {
		t.Fatalf("store.Get(%q) error = %v, want nil", j.ID, err)
	}
	if got.State != queue.JobDone {
		t.Errorf("store.Get().State = %v, want %v", got.State, queue.JobDone)
	}
}

func TestQueue_ProcessNextForPrinter_ProcessorError_ResultsInJobFailed(t *testing.T) {
	store := queue.NewMemoryStore()
	wantErr := errors.New("printer offline")
	proc := &stubProcessor{err: wantErr}
	q := queue.New(store, proc)
	ctx := context.Background()
	j := &queue.Job{PrinterName: "front-desk"}
	mustEnqueue(t, q, j)

	if err := q.ProcessNextForPrinter(ctx, "front-desk"); err != nil {
		t.Fatalf("ProcessNextForPrinter() error = %v, want nil (processor errors are recorded on the Job, not returned)", err)
	}

	got, err := store.Get(ctx, j.ID)
	if err != nil {
		t.Fatalf("store.Get(%q) error = %v, want nil", j.ID, err)
	}
	if got.State != queue.JobFailed {
		t.Errorf("store.Get().State = %v, want %v", got.State, queue.JobFailed)
	}
	if got.LastError != wantErr.Error() {
		t.Errorf("store.Get().LastError = %q, want %q", got.LastError, wantErr.Error())
	}
}

func TestQueue_ProcessNextForPrinter_NoPendingJobForThatPrinter_ReturnsNilWithoutInvokingProcessor(t *testing.T) {
	store := queue.NewMemoryStore()
	proc := &stubProcessor{}
	q := queue.New(store, proc)
	ctx := context.Background()
	// A Job is pending, but for a different printer — front-desk's worker
	// must not touch it.
	mustEnqueue(t, q, &queue.Job{PrinterName: "kitchen"})

	if err := q.ProcessNextForPrinter(ctx, "front-desk"); err != nil {
		t.Fatalf("ProcessNextForPrinter() error = %v, want nil", err)
	}
	if proc.calls != 0 {
		t.Errorf("proc.calls = %d, want 0 (no Job pending for front-desk)", proc.calls)
	}
}

func TestQueue_ProcessNextForPrinter_EmptyQueue_ReturnsNilWithoutInvokingProcessor(t *testing.T) {
	store := queue.NewMemoryStore()
	proc := &stubProcessor{}
	q := queue.New(store, proc)

	if err := q.ProcessNextForPrinter(context.Background(), "front-desk"); err != nil {
		t.Fatalf("ProcessNextForPrinter() error = %v, want nil", err)
	}
	if proc.calls != 0 {
		t.Errorf("proc.calls = %d, want 0", proc.calls)
	}
}

func TestQueue_ProcessNextForPrinter_PersistsRunningStateBeforeInvokingProcessor(t *testing.T) {
	store := queue.NewMemoryStore()
	var sawState queue.JobState
	proc := &stubProcessor{}
	q := queue.New(store, proc)
	ctx := context.Background()
	proc.onProcess = func(j *queue.Job) {
		persisted, err := store.Get(ctx, j.ID)
		if err != nil {
			t.Fatalf("store.Get(%q) error = %v, want nil", j.ID, err)
		}
		sawState = persisted.State
	}
	j := &queue.Job{PrinterName: "front-desk"}
	mustEnqueue(t, q, j)

	if err := q.ProcessNextForPrinter(ctx, "front-desk"); err != nil {
		t.Fatalf("ProcessNextForPrinter() error = %v, want nil", err)
	}
	if sawState != queue.JobRunning {
		t.Errorf("state persisted before Process() was invoked = %v, want %v", sawState, queue.JobRunning)
	}
}

// blockingPrinterProcessor is a queue.Processor test double that blocks on
// a per-printer channel until the test closes it, letting a test hold one
// printer's Job "processing" indefinitely while asserting on a different
// printer's Job in the meantime. release[printerName] must be closed (or
// absent, for an immediate return) for Process to return for that printer.
type blockingPrinterProcessor struct {
	release map[string]chan struct{}
}

func (p *blockingPrinterProcessor) Process(_ context.Context, j *queue.Job) error {
	if ch, ok := p.release[j.PrinterName]; ok {
		<-ch
	}
	return nil
}

// TestQueue_ProcessNextForPrinter_OnePrinterBlocked_DoesNotBlockAnotherPrinter
// is this ADR's central fairness guarantee (docs/adr/
// 0016-queue-concurrency-per-printer-workers.md): a call stuck mid-Process
// for one printer (standing in for a slow/offline one retrying with
// backoff) must never block a concurrent call for a different, healthy
// printer from claiming and completing its own Job.
func TestQueue_ProcessNextForPrinter_OnePrinterBlocked_DoesNotBlockAnotherPrinter(t *testing.T) {
	store := queue.NewMemoryStore()
	stuck := make(chan struct{})
	proc := &blockingPrinterProcessor{release: map[string]chan struct{}{"front-desk": stuck}}
	q := queue.New(store, proc)
	ctx := context.Background()

	frontDesk := &queue.Job{PrinterName: "front-desk"}
	kitchen := &queue.Job{PrinterName: "kitchen"}
	mustEnqueue(t, q, frontDesk)
	mustEnqueue(t, q, kitchen)

	frontDeskDone := make(chan error, 1)
	go func() {
		frontDeskDone <- q.ProcessNextForPrinter(ctx, "front-desk")
	}()

	// Give the front-desk call a moment to actually enter Process and
	// start blocking, so this test would fail (timeout below) if kitchen's
	// call were somehow serialized behind it.
	time.Sleep(20 * time.Millisecond)

	kitchenDone := make(chan error, 1)
	go func() {
		kitchenDone <- q.ProcessNextForPrinter(ctx, "kitchen")
	}()

	select {
	case err := <-kitchenDone:
		if err != nil {
			t.Fatalf("ProcessNextForPrinter(kitchen) error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ProcessNextForPrinter(kitchen) did not return — front-desk's stuck Job blocked a different printer's Job")
	}

	got, err := store.Get(ctx, kitchen.ID)
	if err != nil {
		t.Fatalf("store.Get(%q) error = %v, want nil", kitchen.ID, err)
	}
	if got.State != queue.JobDone {
		t.Errorf("kitchen Job State = %v, want %v", got.State, queue.JobDone)
	}

	// Unblock front-desk so its goroutine (and the test) can finish
	// cleanly.
	close(stuck)
	if err := <-frontDeskDone; err != nil {
		t.Fatalf("ProcessNextForPrinter(front-desk) error = %v, want nil", err)
	}
}
