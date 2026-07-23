package app_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"reflect"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/assets"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// unsupportedElement is a receipt.Element with no render/layout.Build or
// render/canvas.Paint support, used across this package's tests to
// exercise real error propagation through app.Service. receipt.Divider
// filled this role until it became a supported element in its own
// right — every other element type documented in
// docs/ARCHITECTURE.md §3 (image, asset, qrcode, etc.) doesn't exist as
// a receipt.Go type yet, so a small local fake is now the only way to
// construct a "valid but unrenderable" Receipt.
type unsupportedElement struct{}

func (unsupportedElement) Validate() error { return nil }

// fakePrinter is a printer.Printer test double that records every Send call
// and either succeeds (copying data into sent) or returns sendErr, letting
// tests observe how Process orchestrates the printer stage without needing
// a real transport. Status/Close are never exercised through Process, so
// they return fixed, uninteresting values.
type fakePrinter struct {
	sendErr error
	// failOnCall, if non-zero, limits sendErr to that one Send call
	// (1-indexed) instead of every call — how copies tests simulate a
	// transient failure partway through a multi-copy print.
	failOnCall int
	sent       []byte
	sentCalls  [][]byte
	calls      int
}

func (p *fakePrinter) Send(_ context.Context, data []byte) error {
	p.calls++
	if p.sendErr != nil && (p.failOnCall == 0 || p.calls == p.failOnCall) {
		return p.sendErr
	}
	p.sent = append([]byte(nil), data...)
	p.sentCalls = append(p.sentCalls, append([]byte(nil), data...))
	return nil
}

func (p *fakePrinter) Status(_ context.Context) (printer.Status, error) {
	return printer.Status{Online: true}, nil
}

func (p *fakePrinter) Close() error { return nil }

func TestService_Process_Success_ReturnsNilError(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	s.Printers = map[string]printer.Printer{"front-desk": &fakePrinter{}}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	j := &queue.Job{PrinterName: "front-desk", Receipt: validReceipt()}

	if err := s.Process(context.Background(), j); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}
}

func TestService_Process_UnconfiguredProfile_ReturnsNotFoundError(t *testing.T) {
	// A freshly constructed Service (app.New, Profiles never assigned) must
	// not panic — a nil map lookup is safe in Go — and must report the
	// missing configuration as apperr.KindNotFound rather than silently
	// falling back to a zero-value Profile.
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	fp := &fakePrinter{}
	s.Printers = map[string]printer.Printer{"front-desk": fp}
	j := &queue.Job{PrinterName: "front-desk", Receipt: validReceipt()}

	err := s.Process(context.Background(), j)
	if !apperr.Is(err, apperr.KindNotFound) {
		t.Fatalf("Process() error = %v, want apperr.KindNotFound", err)
	}
	if fp.calls != 0 {
		t.Errorf("printer.Send was called %d times, want 0 (an unresolved profile must never reach the printer)", fp.calls)
	}
}

func TestService_Process_UnconfiguredPrinter_ReturnsNotFoundError(t *testing.T) {
	// A Job whose PrinterName has a resolvable Profile but no Printer must
	// still fail with apperr.KindNotFound, not panic on a nil map lookup.
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	s.Profiles = map[string]printer.Profile{"does-not-exist": {}}
	j := &queue.Job{PrinterName: "does-not-exist", Receipt: validReceipt()}

	err := s.Process(context.Background(), j)
	if !apperr.Is(err, apperr.KindNotFound) {
		t.Fatalf("Process() error = %v, want apperr.KindNotFound", err)
	}
}

func TestService_Process_RenderingError_Propagates(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	fp := &fakePrinter{}
	s.Printers = map[string]printer.Printer{"front-desk": fp}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	j := &queue.Job{
		PrinterName: "front-desk",
		Receipt:     receipt.Receipt{Elements: []receipt.Element{unsupportedElement{}}},
	}

	err := s.Process(context.Background(), j)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Process() error = %v, want apperr.KindPermanent", err)
	}
	if fp.calls != 0 {
		t.Errorf("printer.Send was called %d times, want 0 (a rendering failure must never reach the printer)", fp.calls)
	}
}

func TestService_Process_InvalidProfileDefaultCut_ReturnsPermanentError(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	fp := &fakePrinter{}
	s.Printers = map[string]printer.Printer{"front-desk": fp}
	s.Profiles = map[string]printer.Profile{"front-desk": {SupportsCut: true, DefaultCut: "sideways"}}
	j := &queue.Job{PrinterName: "front-desk", Receipt: validReceipt()}

	err := s.Process(context.Background(), j)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Process() error = %v, want apperr.KindPermanent", err)
	}
	if fp.calls != 0 {
		t.Errorf("printer.Send was called %d times, want 0 (an encoding failure must never reach the printer)", fp.calls)
	}
}

func TestService_Process_DoesNotMutateJob(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	s.Printers = map[string]printer.Printer{"front-desk": &fakePrinter{}}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	created := time.Now().Add(-time.Hour)
	j := &queue.Job{
		ID:          "fixed-id",
		PrinterName: "front-desk",
		Receipt:     validReceipt(),
		State:       queue.JobRunning,
		Attempts:    1,
		CreatedAt:   created,
		UpdatedAt:   created,
	}
	before := *j

	if err := s.Process(context.Background(), j); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}

	if !reflect.DeepEqual(*j, before) {
		t.Errorf("Process() mutated the Job: got %+v, want unchanged %+v", *j, before)
	}
}

func TestService_Process_SendsEscposEncodedBytesToPrinter(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	fp := &fakePrinter{}
	s.Printers = map[string]printer.Printer{"front-desk": fp}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	j := &queue.Job{PrinterName: "front-desk", Receipt: validReceipt()}

	if err := s.Process(context.Background(), j); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}

	escInit := []byte{0x1B, 0x40} // ESC @: the init sequence every escpos.Encode output starts with
	if len(fp.sent) == 0 {
		t.Fatal("printer received no data")
	}
	if fp.sent[0] != escInit[0] || fp.sent[1] != escInit[1] {
		t.Errorf("printer received % x, want it to start with % x (ESC/POS init sequence)", fp.sent, escInit)
	}
}

func TestService_Process_ProfileWithCut_SendsFeedAndCutToPrinter(t *testing.T) {
	// This is the behavior the previous slice left unreachable: a Profile
	// resolved from Service.Profiles, not a hard-coded zero value, must
	// actually reach escpos.Encode and produce a trailing cut command.
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	fp := &fakePrinter{}
	s.Printers = map[string]printer.Printer{"front-desk": fp}
	s.Profiles = map[string]printer.Profile{"front-desk": {SupportsCut: true, DefaultCut: "partial"}}
	j := &queue.Job{PrinterName: "front-desk", Receipt: validReceipt()}

	if err := s.Process(context.Background(), j); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}

	wantCut := []byte{0x1D, 0x56, 0x01} // GS V 1: partial cut
	if !bytes.HasSuffix(fp.sent, wantCut) {
		t.Errorf("printer received % x, want it to end with % x (partial cut, per the resolved Profile)", fp.sent, wantCut)
	}
}

func TestService_Process_SendsDistinctBytesPerReceipt(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))

	shortPrinter := &fakePrinter{}
	s.Printers = map[string]printer.Printer{"front-desk": shortPrinter}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	short := &queue.Job{PrinterName: "front-desk", Receipt: receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: "hi"}}}}
	if err := s.Process(context.Background(), short); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}

	longPrinter := &fakePrinter{}
	s.Printers = map[string]printer.Printer{"front-desk": longPrinter}
	long := &queue.Job{PrinterName: "front-desk", Receipt: receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: "hello world"}}}}
	if err := s.Process(context.Background(), long); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}

	if len(shortPrinter.sent) == len(longPrinter.sent) {
		t.Error("Process() sent identically-sized output for different Receipts, want each Job's own rendered content to reach the printer")
	}
}

func TestService_Process_InvokesPrinterSendExactlyOnce(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	fp := &fakePrinter{}
	s.Printers = map[string]printer.Printer{"front-desk": fp}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	j := &queue.Job{PrinterName: "front-desk", Receipt: validReceipt()}

	if err := s.Process(context.Background(), j); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}

	if fp.calls != 1 {
		t.Errorf("printer.Send was called %d times, want exactly 1", fp.calls)
	}
}

func TestService_Process_PrinterSendError_Propagates(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	sendErr := apperr.Wrap(apperr.KindTransient, "printer.Send", errors.New("connection refused"))
	s.Printers = map[string]printer.Printer{"front-desk": &fakePrinter{sendErr: sendErr}}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	j := &queue.Job{PrinterName: "front-desk", Receipt: validReceipt()}

	err := s.Process(context.Background(), j)
	if !apperr.Is(err, apperr.KindTransient) {
		t.Fatalf("Process() error = %v, want apperr.KindTransient", err)
	}
	if !errors.Is(err, sendErr) {
		t.Errorf("Process() error = %v, want it to be sendErr unmodified", err)
	}
}

func TestService_Process_PrinterSendError_DoesNotMutateJob(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	sendErr := apperr.Wrap(apperr.KindTransient, "printer.Send", errors.New("connection refused"))
	s.Printers = map[string]printer.Printer{"front-desk": &fakePrinter{sendErr: sendErr}}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	created := time.Now().Add(-time.Hour)
	j := &queue.Job{
		ID:          "fixed-id",
		PrinterName: "front-desk",
		Receipt:     validReceipt(),
		State:       queue.JobRunning,
		Attempts:    1,
		CreatedAt:   created,
		UpdatedAt:   created,
	}
	before := *j

	_ = s.Process(context.Background(), j)

	if !reflect.DeepEqual(*j, before) {
		t.Errorf("Process() mutated the Job on a send error: got %+v, want unchanged %+v", *j, before)
	}
}

// solidPNG returns the encoded bytes of a width x height PNG filled with c.
func solidPNG(t *testing.T, width, height int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode() error = %v, want nil", err)
	}
	return buf.Bytes()
}

// TestService_Process_ReReadsAssetOnEachCall pins
// docs/adr/0019-retry-pipeline-granularity.md's decision: Process caches
// nothing across calls, so a retry (a second Process call for the same
// Job, exactly what queue.runClaimedJob does) always re-resolves a
// receipt.Asset's current content rather than reusing what an earlier
// call saw.
func TestService_Process_ReReadsAssetOnEachCall(t *testing.T) {
	ctx := context.Background()
	store := assets.NewMemoryStore()
	if err := store.Put(ctx, "logo.png", solidPNG(t, 4, 2, color.Black)); err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}

	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	s.Assets = store
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	j := &queue.Job{PrinterName: "front-desk", Receipt: receipt.Receipt{Elements: []receipt.Element{receipt.Asset{Name: "logo.png"}}}}

	first := &fakePrinter{}
	s.Printers = map[string]printer.Printer{"front-desk": first}
	if err := s.Process(ctx, j); err != nil {
		t.Fatalf("Process() (first) error = %v, want nil", err)
	}

	if err := store.Put(ctx, "logo.png", solidPNG(t, 8, 2, color.Black)); err != nil {
		t.Fatalf("Put() (overwrite) error = %v, want nil", err)
	}

	second := &fakePrinter{}
	s.Printers = map[string]printer.Printer{"front-desk": second}
	if err := s.Process(ctx, j); err != nil {
		t.Fatalf("Process() (second) error = %v, want nil", err)
	}

	if bytes.Equal(first.sent, second.sent) {
		t.Error("Process() sent identical bytes after the asset changed between calls, want each call to re-resolve and re-render the asset's current content")
	}
}

// countingAssetStore wraps an assets.Store to count Get calls, letting
// tests observe that Process resolves an Asset element once per call
// regardless of Receipt.Copies, without depending on layout/canvas
// internals.
type countingAssetStore struct {
	assets.Store
	getCalls int
}

func (s *countingAssetStore) Get(ctx context.Context, name string) ([]byte, error) {
	s.getCalls++
	return s.Store.Get(ctx, name)
}

func TestService_Process_CopiesOne_SendsExactlyOnce(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	fp := &fakePrinter{}
	s.Printers = map[string]printer.Printer{"front-desk": fp}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	j := &queue.Job{PrinterName: "front-desk", Receipt: receipt.Receipt{Copies: 1, Elements: validReceipt().Elements}}

	if err := s.Process(context.Background(), j); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}
	if fp.calls != 1 {
		t.Errorf("printer.Send was called %d times, want exactly 1", fp.calls)
	}
}

func TestService_Process_CopiesZero_TreatedAsOne(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	fp := &fakePrinter{}
	s.Printers = map[string]printer.Printer{"front-desk": fp}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	j := &queue.Job{PrinterName: "front-desk", Receipt: receipt.Receipt{Copies: 0, Elements: validReceipt().Elements}}

	if err := s.Process(context.Background(), j); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}
	if fp.calls != 1 {
		t.Errorf("printer.Send was called %d times, want exactly 1 (zero copies treated as one)", fp.calls)
	}
}

func TestService_Process_CopiesThree_SendsThreeIdenticalCopies(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	fp := &fakePrinter{}
	s.Printers = map[string]printer.Printer{"front-desk": fp}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	j := &queue.Job{PrinterName: "front-desk", Receipt: receipt.Receipt{Copies: 3, Elements: validReceipt().Elements}}

	if err := s.Process(context.Background(), j); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}
	if fp.calls != 3 {
		t.Fatalf("printer.Send was called %d times, want exactly 3", fp.calls)
	}
	for i, sent := range fp.sentCalls {
		if !bytes.Equal(sent, fp.sentCalls[0]) {
			t.Errorf("copy %d bytes differ from copy 0, want every copy byte-identical", i)
		}
	}
}

func TestService_Process_TransientFailureDuringSecondCopy_Propagates(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	sendErr := apperr.Wrap(apperr.KindTransient, "printer.Send", errors.New("connection refused"))
	fp := &fakePrinter{sendErr: sendErr, failOnCall: 2}
	s.Printers = map[string]printer.Printer{"front-desk": fp}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	j := &queue.Job{PrinterName: "front-desk", Receipt: receipt.Receipt{Copies: 3, Elements: validReceipt().Elements}}

	err := s.Process(context.Background(), j)
	if !apperr.Is(err, apperr.KindTransient) {
		t.Fatalf("Process() error = %v, want apperr.KindTransient", err)
	}
	if !errors.Is(err, sendErr) {
		t.Errorf("Process() error = %v, want it to be sendErr unmodified", err)
	}
	if fp.calls != 2 {
		t.Errorf("printer.Send was called %d times, want exactly 2 (stop at the failing copy, don't attempt copy 3)", fp.calls)
	}
}

func TestService_Process_MultipleCopies_RendersAndEncodesExactlyOnce(t *testing.T) {
	ctx := context.Background()
	store := &countingAssetStore{Store: assets.NewMemoryStore()}
	if err := store.Put(ctx, "logo.png", solidPNG(t, 4, 2, color.Black)); err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}

	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	s.Assets = store
	fp := &fakePrinter{}
	s.Printers = map[string]printer.Printer{"front-desk": fp}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	j := &queue.Job{PrinterName: "front-desk", Receipt: receipt.Receipt{Copies: 3, Elements: []receipt.Element{receipt.Asset{Name: "logo.png"}}}}

	if err := s.Process(ctx, j); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}
	if fp.calls != 3 {
		t.Fatalf("printer.Send was called %d times, want exactly 3", fp.calls)
	}
	if store.getCalls != 1 {
		t.Errorf("assets.Store.Get was called %d times, want exactly 1 (render/encode must run once regardless of copy count)", store.getCalls)
	}
}
