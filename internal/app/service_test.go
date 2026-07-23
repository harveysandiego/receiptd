package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

func (f *fakeStore) ClaimNextPending(_ context.Context, _ string) (*queue.Job, error) {
	return nil, nil
}

func (f *fakeStore) EnqueueIdempotent(_ context.Context, _ *queue.Job, _ time.Time) (*queue.Job, bool, error) {
	return nil, false, nil
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

	jobID, err := s.Print(ctx, validReceipt(), "front-desk", "")
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

	jobID, err := s.Print(ctx, validReceipt(), "front-desk", "")
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

	jobID, err := s.Print(ctx, validReceipt(), "back-office", "")
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

	jobID, err := s.Print(ctx, r, "front-desk", "")
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

	jobID, err := s.Print(context.Background(), validReceipt(), "front-desk", "")
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

	jobID, err := s.Print(ctx, invalid, "front-desk", "")
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

	if _, err := s.Print(context.Background(), validReceipt(), "front-desk", ""); err != nil {
		t.Fatalf("Print() error = %v, want nil", err)
	}
	if proc.calls != 0 {
		t.Errorf("proc.calls = %d, want 0 (Print must only enqueue, never process)", proc.calls)
	}
}

// --- docs/adr/0020-idempotent-print-requests.md: Print with a non-empty
// idempotencyKey. ---

func TestService_Print_WithIdempotencyKey_FirstCallEnqueuesJob(t *testing.T) {
	store := queue.NewMemoryStore()
	s := app.New(queue.New(store, &noopProcessor{}))
	ctx := context.Background()

	jobID, err := s.Print(ctx, validReceipt(), "front-desk", "key-1")
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
	if got.State != queue.JobPending {
		t.Errorf("store.Get().State = %v, want %v", got.State, queue.JobPending)
	}
}

func TestService_Print_WithIdempotencyKey_RetryOfPendingJobReturnsSameID(t *testing.T) {
	store := queue.NewMemoryStore()
	s := app.New(queue.New(store, &noopProcessor{}))
	ctx := context.Background()
	r := validReceipt()

	first, err := s.Print(ctx, r, "front-desk", "key-1")
	if err != nil {
		t.Fatalf("Print() (first) error = %v, want nil", err)
	}
	second, err := s.Print(ctx, r, "front-desk", "key-1")
	if err != nil {
		t.Fatalf("Print() (retry) error = %v, want nil", err)
	}

	if second != first {
		t.Errorf("Print() (retry) jobID = %q, want the same ID as the first call, %q", second, first)
	}

	jobs, err := store.List(ctx, queue.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v, want nil", err)
	}
	if len(jobs) != 1 {
		t.Errorf("len(store.List()) = %d, want 1 (a same-key retry must not enqueue a second Job)", len(jobs))
	}
}

func TestService_Print_WithIdempotencyKey_MismatchedReceiptRejected(t *testing.T) {
	store := queue.NewMemoryStore()
	s := app.New(queue.New(store, &noopProcessor{}))
	ctx := context.Background()

	if _, err := s.Print(ctx, validReceipt(), "front-desk", "key-1"); err != nil {
		t.Fatalf("Print() (first) error = %v, want nil", err)
	}

	other := receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: "goodbye"}}}
	jobID, err := s.Print(ctx, other, "front-desk", "key-1")
	if !apperr.Is(err, apperr.KindValidation) {
		t.Fatalf("Print() (mismatched Receipt) error = %v, want apperr.KindValidation", err)
	}
	if jobID != "" {
		t.Errorf("Print() jobID = %q, want \"\" on a rejected reused key", jobID)
	}

	jobs, err := store.List(ctx, queue.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v, want nil", err)
	}
	if len(jobs) != 1 {
		t.Errorf("len(store.List()) = %d, want 1 (a rejected reused key must not create or mutate a Job)", len(jobs))
	}
}

func TestService_Print_WithIdempotencyKey_MismatchedPrinterNameRejected(t *testing.T) {
	store := queue.NewMemoryStore()
	s := app.New(queue.New(store, &noopProcessor{}))
	ctx := context.Background()
	r := validReceipt()

	if _, err := s.Print(ctx, r, "front-desk", "key-1"); err != nil {
		t.Fatalf("Print() (first) error = %v, want nil", err)
	}

	jobID, err := s.Print(ctx, r, "back-office", "key-1")
	if !apperr.Is(err, apperr.KindValidation) {
		t.Fatalf("Print() (mismatched printerName) error = %v, want apperr.KindValidation", err)
	}
	if jobID != "" {
		t.Errorf("Print() jobID = %q, want \"\" on a rejected reused key", jobID)
	}
}

func TestService_Print_WithIdempotencyKey_RetryOfFailedJobReactivatesInPlace(t *testing.T) {
	store := queue.NewMemoryStore()
	s := app.New(queue.New(store, &noopProcessor{}))
	ctx := context.Background()
	r := validReceipt()

	first, err := s.Print(ctx, r, "front-desk", "key-1")
	if err != nil {
		t.Fatalf("Print() (first) error = %v, want nil", err)
	}

	failed, err := store.Get(ctx, first)
	if err != nil {
		t.Fatalf("store.Get() error = %v, want nil", err)
	}
	failed.State = queue.JobFailed
	failed.Attempts = 3
	failed.LastError = "printer offline"
	if err := store.Save(ctx, failed); err != nil {
		t.Fatalf("store.Save() error = %v, want nil", err)
	}

	second, err := s.Print(ctx, r, "front-desk", "key-1")
	if err != nil {
		t.Fatalf("Print() (retry of Failed) error = %v, want nil", err)
	}
	if second != first {
		t.Errorf("Print() (retry of Failed) jobID = %q, want the same reactivated ID %q", second, first)
	}

	reactivated, err := store.Get(ctx, second)
	if err != nil {
		t.Fatalf("store.Get() error = %v, want nil", err)
	}
	if reactivated.State != queue.JobPending {
		t.Errorf("reactivated.State = %v, want %v", reactivated.State, queue.JobPending)
	}
	if reactivated.Attempts != 0 {
		t.Errorf("reactivated.Attempts = %d, want 0", reactivated.Attempts)
	}
	if reactivated.LastError != "" {
		t.Errorf("reactivated.LastError = %q, want \"\"", reactivated.LastError)
	}

	jobs, err := store.List(ctx, queue.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v, want nil", err)
	}
	if len(jobs) != 1 {
		t.Errorf("len(store.List()) = %d, want 1 (reactivation must reuse the existing Job, not create a second one)", len(jobs))
	}
}
