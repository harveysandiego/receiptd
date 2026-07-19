package app_test

import (
	"bytes"
	"context"
	"image/png"
	"testing"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// previewServiceWithProfile returns a Service configured with one
// printer, "front-desk", so Preview's printerName resolution has
// something to succeed against.
func previewServiceWithProfile() *app.Service {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	return s
}

func TestService_Preview_ValidReceipt_ReturnsPNGBytes(t *testing.T) {
	s := previewServiceWithProfile()

	b, err := s.Preview(context.Background(), validReceipt(), "front-desk")
	if err != nil {
		t.Fatalf("Preview() error = %v, want nil", err)
	}
	if len(b) == 0 {
		t.Fatal("Preview() returned no bytes")
	}
}

func TestService_Preview_ReturnsDecodablePNG(t *testing.T) {
	s := previewServiceWithProfile()

	b, err := s.Preview(context.Background(), validReceipt(), "front-desk")
	if err != nil {
		t.Fatalf("Preview() error = %v, want nil", err)
	}
	if _, err := png.Decode(bytes.NewReader(b)); err != nil {
		t.Fatalf("png.Decode() error = %v, want a valid PNG", err)
	}
}

func TestService_Preview_InvalidReceipt_ReturnsValidationError(t *testing.T) {
	// No Profiles configured at all: an invalid Receipt must fail
	// validation before printerName is ever resolved.
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	invalid := receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: ""}}}

	b, err := s.Preview(context.Background(), invalid, "front-desk")
	if !apperr.Is(err, apperr.KindValidation) {
		t.Fatalf("Preview() error = %v, want apperr.KindValidation", err)
	}
	if b != nil {
		t.Errorf("Preview() bytes = %v, want nil on error", b)
	}
}

func TestService_Preview_UnconfiguredPrinter_ReturnsNotFoundError(t *testing.T) {
	// A valid Receipt against a printer name with no configured Profile
	// must fail with apperr.KindNotFound, not a panic on a nil/missing
	// map entry, per docs/adr/0006-preview-requires-printer-profile.md.
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))

	b, err := s.Preview(context.Background(), validReceipt(), "does-not-exist")
	if !apperr.Is(err, apperr.KindNotFound) {
		t.Fatalf("Preview() error = %v, want apperr.KindNotFound", err)
	}
	if b != nil {
		t.Errorf("Preview() bytes = %v, want nil on error", b)
	}
}

func TestService_Preview_UnsupportedElement_ReturnsPermanentError(t *testing.T) {
	// unsupportedElement (defined in process_test.go) exercises the same
	// real error path as TestService_Process_RenderingError_Propagates.
	s := previewServiceWithProfile()
	r := receipt.Receipt{Elements: []receipt.Element{unsupportedElement{}}}

	b, err := s.Preview(context.Background(), r, "front-desk")
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Preview() error = %v, want apperr.KindPermanent", err)
	}
	if b != nil {
		t.Errorf("Preview() bytes = %v, want nil on error", b)
	}
}

func TestService_Preview_EmptyReceipt_ReturnsPermanentError(t *testing.T) {
	// An empty Receipt validates and renders to a zero-size Canvas;
	// canvas.EncodePNG defines that as apperr.KindPermanent rather than
	// producing placeholder bytes, so Preview must surface that error
	// rather than returning an empty PNG.
	s := previewServiceWithProfile()

	b, err := s.Preview(context.Background(), receipt.Receipt{}, "front-desk")
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Preview() error = %v, want apperr.KindPermanent", err)
	}
	if b != nil {
		t.Errorf("Preview() bytes = %v, want nil on error", b)
	}
}

func TestService_Preview_DoesNotEnqueueOrProcess(t *testing.T) {
	store := queue.NewMemoryStore()
	proc := &noopProcessor{}
	s := app.New(queue.New(store, proc))
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	ctx := context.Background()

	if _, err := s.Preview(ctx, validReceipt(), "front-desk"); err != nil {
		t.Fatalf("Preview() error = %v, want nil", err)
	}
	if proc.calls != 0 {
		t.Errorf("proc.calls = %d, want 0 (Preview must not invoke the Processor)", proc.calls)
	}

	jobs, err := store.List(ctx, queue.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v, want nil", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(store.List()) = %d, want 0 (Preview must not enqueue a Job)", len(jobs))
	}
}
