package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// noopProcessor is a queue.Processor test double that records how many
// times Process was called. Service.Print must never trigger it — Print
// only enqueues work, it never processes it.
type noopProcessor struct {
	calls int
}

func (p *noopProcessor) Process(_ context.Context, _ *queue.Job) error {
	p.calls++
	return nil
}

// fakeStore is a queue.Store test double that returns saveErr from Save and
// either getJob or getErr from Get, letting tests observe how Service
// propagates a Store failure without needing a real Store implementation.
type fakeStore struct {
	saveErr error
	getJob  *queue.Job
	getErr  error
}

func (f *fakeStore) Save(_ context.Context, _ *queue.Job) error { return f.saveErr }

func (f *fakeStore) Get(_ context.Context, _ string) (*queue.Job, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.getJob, nil
}

func (f *fakeStore) List(_ context.Context, _ queue.Filter) ([]*queue.Job, error) {
	return nil, nil
}

func (f *fakeStore) NextPending(_ context.Context) (*queue.Job, error) {
	return nil, nil
}

func validReceipt() receipt.Receipt {
	return receipt.Receipt{
		Elements: []receipt.Element{receipt.Text{Content: "hello"}},
	}
}

func TestService_Print_EnqueuesJob(t *testing.T) {
	store := queue.NewMemoryStore()
	proc := &noopProcessor{}
	s := app.New(queue.New(store, proc))
	ctx := context.Background()

	jobID, err := s.Print(ctx, validReceipt(), "front-desk")
	if err != nil {
		t.Fatalf("Print() error = %v, want nil", err)
	}

	got, err := store.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("store.Get(%q) error = %v, want nil", jobID, err)
	}
	if got.State != queue.JobPending {
		t.Errorf("store.Get().State = %v, want %v", got.State, queue.JobPending)
	}
}

func TestService_Print_ReturnedJobIDMatchesQueuedJob(t *testing.T) {
	store := queue.NewMemoryStore()
	s := app.New(queue.New(store, &noopProcessor{}))
	ctx := context.Background()

	jobID, err := s.Print(ctx, validReceipt(), "front-desk")
	if err != nil {
		t.Fatalf("Print() error = %v, want nil", err)
	}
	if jobID == "" {
		t.Fatal("Print() jobID = \"\", want a generated ID")
	}

	got, err := store.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("store.Get(%q) error = %v, want nil", jobID, err)
	}
	if got.ID != jobID {
		t.Errorf("store.Get().ID = %q, want %q", got.ID, jobID)
	}
}

func TestService_Print_PropagatesPrinterName(t *testing.T) {
	store := queue.NewMemoryStore()
	s := app.New(queue.New(store, &noopProcessor{}))
	ctx := context.Background()

	jobID, err := s.Print(ctx, validReceipt(), "back-office")
	if err != nil {
		t.Fatalf("Print() error = %v, want nil", err)
	}

	got, err := store.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("store.Get(%q) error = %v, want nil", jobID, err)
	}
	if got.PrinterName != "back-office" {
		t.Errorf("store.Get().PrinterName = %q, want %q", got.PrinterName, "back-office")
	}
}

func TestService_Print_PropagatesReceipt(t *testing.T) {
	store := queue.NewMemoryStore()
	s := app.New(queue.New(store, &noopProcessor{}))
	ctx := context.Background()
	r := validReceipt()

	jobID, err := s.Print(ctx, r, "front-desk")
	if err != nil {
		t.Fatalf("Print() error = %v, want nil", err)
	}

	got, err := store.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("store.Get(%q) error = %v, want nil", jobID, err)
	}
	if len(got.Receipt.Elements) != len(r.Elements) {
		t.Fatalf("store.Get().Receipt.Elements = %d elements, want %d", len(got.Receipt.Elements), len(r.Elements))
	}
	gotText, ok := got.Receipt.Elements[0].(receipt.Text)
	if !ok {
		t.Fatalf("store.Get().Receipt.Elements[0] = %T, want receipt.Text", got.Receipt.Elements[0])
	}
	if gotText.Content != "hello" {
		t.Errorf("store.Get().Receipt.Elements[0].Content = %q, want %q", gotText.Content, "hello")
	}
}

func TestService_Print_QueueErrorPropagates(t *testing.T) {
	wantErr := apperr.Wrap(apperr.KindPermanent, "fakeStore.Save", errors.New("disk full"))
	store := &fakeStore{saveErr: wantErr}
	s := app.New(queue.New(store, &noopProcessor{}))

	jobID, err := s.Print(context.Background(), validReceipt(), "front-desk")
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Print() error = %v, want apperr.KindPermanent", err)
	}
	if jobID != "" {
		t.Errorf("Print() jobID = %q, want \"\" on error", jobID)
	}
}

func TestService_Print_InvalidReceipt_ReturnsValidationErrorWithoutEnqueuing(t *testing.T) {
	store := queue.NewMemoryStore()
	s := app.New(queue.New(store, &noopProcessor{}))
	ctx := context.Background()
	invalid := receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: ""}}}

	jobID, err := s.Print(ctx, invalid, "front-desk")
	if !apperr.Is(err, apperr.KindValidation) {
		t.Fatalf("Print() error = %v, want apperr.KindValidation", err)
	}
	if jobID != "" {
		t.Errorf("Print() jobID = %q, want \"\" on error", jobID)
	}

	jobs, err := store.List(ctx, queue.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v, want nil", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(store.List()) = %d, want 0 (invalid Receipt must not be enqueued)", len(jobs))
	}
}

func TestService_Print_DoesNotInvokeProcessor(t *testing.T) {
	store := queue.NewMemoryStore()
	proc := &noopProcessor{}
	s := app.New(queue.New(store, proc))

	if _, err := s.Print(context.Background(), validReceipt(), "front-desk"); err != nil {
		t.Fatalf("Print() error = %v, want nil", err)
	}
	if proc.calls != 0 {
		t.Errorf("proc.calls = %d, want 0 (Print must only enqueue, never process)", proc.calls)
	}
}
