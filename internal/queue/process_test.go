package queue_test

import (
	"context"
	"errors"
	"testing"

	"github.com/harveysandiego/receiptd/internal/queue"
)

// stubProcessor is a trivial Processor test double: it records how many
// times Process was called and returns err (nil by default, meaning
// success). onProcess, if set, is invoked with the Job before Process
// returns, letting a test observe state as of the moment Process ran.
type stubProcessor struct {
	err       error
	calls     int
	onProcess func(j *queue.Job)
}

func (p *stubProcessor) Process(_ context.Context, j *queue.Job) error {
	p.calls++
	if p.onProcess != nil {
		p.onProcess(j)
	}
	return p.err
}

func mustEnqueue(t *testing.T, q *queue.Queue, j *queue.Job) {
	t.Helper()
	if err := q.Enqueue(context.Background(), j); err != nil {
		t.Fatalf("Enqueue() error = %v, want nil", err)
	}
}

func TestQueue_ProcessNext_ProcessesPendingJob_ResultsInJobDone(t *testing.T) {
	store := queue.NewMemoryStore()
	proc := &stubProcessor{}
	q := queue.New(store, proc)
	ctx := context.Background()
	j := &queue.Job{PrinterName: "front-desk"}
	mustEnqueue(t, q, j)

	if err := q.ProcessNext(ctx); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
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

func TestQueue_ProcessNext_ProcessorError_ResultsInJobFailed(t *testing.T) {
	store := queue.NewMemoryStore()
	wantErr := errors.New("printer offline")
	proc := &stubProcessor{err: wantErr}
	q := queue.New(store, proc)
	ctx := context.Background()
	j := &queue.Job{PrinterName: "front-desk"}
	mustEnqueue(t, q, j)

	if err := q.ProcessNext(ctx); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil (processor errors are recorded on the Job, not returned)", err)
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

func TestQueue_ProcessNext_PersistsRunningStateBeforeInvokingProcessor(t *testing.T) {
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

	if err := q.ProcessNext(ctx); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}
	if sawState != queue.JobRunning {
		t.Errorf("state persisted before Process() was invoked = %v, want %v", sawState, queue.JobRunning)
	}
}

func TestQueue_ProcessNext_NoPendingJobs_ReturnsNilWithoutInvokingProcessor(t *testing.T) {
	store := queue.NewMemoryStore()
	proc := &stubProcessor{}
	q := queue.New(store, proc)

	if err := q.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}
	if proc.calls != 0 {
		t.Errorf("proc.calls = %d, want 0", proc.calls)
	}
}

func TestQueue_ProcessNext_ProcessesAtMostOneJobPerCall(t *testing.T) {
	store := queue.NewMemoryStore()
	proc := &stubProcessor{}
	q := queue.New(store, proc)
	ctx := context.Background()
	j1 := &queue.Job{PrinterName: "front-desk"}
	j2 := &queue.Job{PrinterName: "back-office"}
	mustEnqueue(t, q, j1)
	mustEnqueue(t, q, j2)

	if err := q.ProcessNext(ctx); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}
	if proc.calls != 1 {
		t.Errorf("proc.calls = %d, want 1", proc.calls)
	}

	got1, err := store.Get(ctx, j1.ID)
	if err != nil {
		t.Fatalf("store.Get(%q) error = %v, want nil", j1.ID, err)
	}
	got2, err := store.Get(ctx, j2.ID)
	if err != nil {
		t.Fatalf("store.Get(%q) error = %v, want nil", j2.ID, err)
	}

	pendingCount, doneCount := 0, 0
	for _, s := range []queue.JobState{got1.State, got2.State} {
		switch s {
		case queue.JobPending:
			pendingCount++
		case queue.JobDone:
			doneCount++
		}
	}
	if pendingCount != 1 || doneCount != 1 {
		t.Errorf("got1.State=%v got2.State=%v, want exactly one JobPending and one JobDone", got1.State, got2.State)
	}
}
