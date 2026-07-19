package app_test

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// fakePrinter is a printer.Printer test double that records every Send call
// and either succeeds (copying data into sent) or returns sendErr, letting
// tests observe how Process orchestrates the printer stage without needing
// a real transport. Status/Close are never exercised through Process, so
// they return fixed, uninteresting values.
type fakePrinter struct {
	sendErr error
	sent    []byte
	calls   int
}

func (p *fakePrinter) Send(_ context.Context, data []byte) error {
	p.calls++
	if p.sendErr != nil {
		return p.sendErr
	}
	p.sent = append([]byte(nil), data...)
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
	// receipt.Divider is a valid Element (passes Validate) but is not yet
	// supported by render/layout.Build, which only handles receipt.Text and
	// receipt.Heading — this is the current pipeline's real error path, not
	// a contrived one.
	j := &queue.Job{
		PrinterName: "front-desk",
		Receipt:     receipt.Receipt{Elements: []receipt.Element{receipt.Divider{Style: "solid"}}},
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
