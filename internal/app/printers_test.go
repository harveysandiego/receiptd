package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/queue"
)

// statusFakePrinter is a printer.Printer test double letting each test
// control exactly what Status reports, independent of Send/Close — none
// of which ListPrinters ever calls.
type statusFakePrinter struct {
	status    printer.Status
	statusErr error
	calls     int
}

func (p *statusFakePrinter) Send(_ context.Context, _ []byte) error { return nil }

func (p *statusFakePrinter) Status(_ context.Context) (printer.Status, error) {
	p.calls++
	if p.statusErr != nil {
		return printer.Status{}, p.statusErr
	}
	return p.status, nil
}

func (p *statusFakePrinter) Close() error { return nil }

func newTestService() *app.Service {
	return app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
}

func TestService_ListPrinters_NoPrinters_ReturnsEmptySlice(t *testing.T) {
	s := newTestService()

	summaries, err := s.ListPrinters(context.Background())
	if err != nil {
		t.Fatalf("ListPrinters() error = %v, want nil", err)
	}
	if len(summaries) != 0 {
		t.Errorf("len(ListPrinters()) = %d, want 0", len(summaries))
	}
}

func TestService_ListPrinters_ReturnsSummaryPerConfiguredPrinter(t *testing.T) {
	s := newTestService()
	s.Printers = map[string]printer.Printer{
		"front-desk": &statusFakePrinter{status: printer.Status{Online: true}},
	}
	s.Profiles = map[string]printer.Profile{
		"front-desk": {WidthDots: 576, DPI: 203, SupportsCut: true, SupportsPartialCut: true},
	}
	s.Connections = map[string]app.ConnectionSummary{
		"front-desk": {Transport: "network", Address: "192.168.1.50:9100"},
	}

	summaries, err := s.ListPrinters(context.Background())
	if err != nil {
		t.Fatalf("ListPrinters() error = %v, want nil", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("len(ListPrinters()) = %d, want 1", len(summaries))
	}

	got := summaries[0]
	want := app.PrinterSummary{
		Name:               "front-desk",
		Transport:          "network",
		Address:            "192.168.1.50:9100",
		WidthDots:          576,
		DPI:                203,
		SupportsCut:        true,
		SupportsPartialCut: true,
		Online:             true,
	}
	if got != want {
		t.Errorf("ListPrinters()[0] = %+v, want %+v", got, want)
	}
}

func TestService_ListPrinters_SortedByName(t *testing.T) {
	s := newTestService()
	names := []string{"zebra", "front-desk", "kitchen"}
	s.Printers = map[string]printer.Printer{}
	s.Profiles = map[string]printer.Profile{}
	s.Connections = map[string]app.ConnectionSummary{}
	for _, name := range names {
		s.Printers[name] = &statusFakePrinter{status: printer.Status{Online: true}}
		s.Profiles[name] = printer.Profile{}
		s.Connections[name] = app.ConnectionSummary{}
	}

	summaries, err := s.ListPrinters(context.Background())
	if err != nil {
		t.Fatalf("ListPrinters() error = %v, want nil", err)
	}
	if len(summaries) != 3 {
		t.Fatalf("len(ListPrinters()) = %d, want 3", len(summaries))
	}

	want := []string{"front-desk", "kitchen", "zebra"}
	for i, s := range summaries {
		if s.Name != want[i] {
			t.Errorf("ListPrinters()[%d].Name = %q, want %q (sorted by name)", i, s.Name, want[i])
		}
	}
}

func TestService_ListPrinters_OfflinePrinter_StillIncludedNotFailed(t *testing.T) {
	s := newTestService()
	s.Printers = map[string]printer.Printer{
		"front-desk": &statusFakePrinter{status: printer.Status{Online: false, Detail: "connection refused"}},
	}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	s.Connections = map[string]app.ConnectionSummary{"front-desk": {}}

	summaries, err := s.ListPrinters(context.Background())
	if err != nil {
		t.Fatalf("ListPrinters() error = %v, want nil (one offline printer must not fail the whole listing)", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("len(ListPrinters()) = %d, want 1", len(summaries))
	}
	if summaries[0].Online {
		t.Error("ListPrinters()[0].Online = true, want false")
	}
	if summaries[0].StatusDetail != "connection refused" {
		t.Errorf("ListPrinters()[0].StatusDetail = %q, want %q", summaries[0].StatusDetail, "connection refused")
	}
}

// TestService_ListPrinters_StatusError_TreatedAsOfflineNotFailed proves
// ListPrinters survives a printer.Printer.Status call that itself returns
// a non-nil error (rather than just Status.Online == false) — the only
// shipped implementation (printer.networkPrinter) never does this, but
// the interface allows it, and one printer's Status error must not fail
// the whole listing any more than a plain offline report does.
func TestService_ListPrinters_StatusError_TreatedAsOfflineNotFailed(t *testing.T) {
	s := newTestService()
	s.Printers = map[string]printer.Printer{
		"front-desk": &statusFakePrinter{statusErr: errors.New("boom")},
	}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	s.Connections = map[string]app.ConnectionSummary{"front-desk": {}}

	summaries, err := s.ListPrinters(context.Background())
	if err != nil {
		t.Fatalf("ListPrinters() error = %v, want nil", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("len(ListPrinters()) = %d, want 1", len(summaries))
	}
	if summaries[0].Online {
		t.Error("ListPrinters()[0].Online = true, want false")
	}
	if summaries[0].StatusDetail != "boom" {
		t.Errorf("ListPrinters()[0].StatusDetail = %q, want %q", summaries[0].StatusDetail, "boom")
	}
}

func TestService_ListPrinters_MissingProfile_ReturnsPermanentError(t *testing.T) {
	s := newTestService()
	s.Printers = map[string]printer.Printer{"front-desk": &statusFakePrinter{}}
	s.Connections = map[string]app.ConnectionSummary{"front-desk": {}}
	// s.Profiles deliberately left nil: buildPrinters always populates
	// Printers/Profiles/Connections together from the same config slice,
	// so a name present in Printers but missing from Profiles can only
	// mean a construction bug.

	_, err := s.ListPrinters(context.Background())
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("ListPrinters() error = %v, want apperr.KindPermanent", err)
	}
}

func TestService_ListPrinters_MissingConnection_ReturnsPermanentError(t *testing.T) {
	s := newTestService()
	s.Printers = map[string]printer.Printer{"front-desk": &statusFakePrinter{}}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	// s.Connections deliberately left nil — same invariant as above.

	_, err := s.ListPrinters(context.Background())
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("ListPrinters() error = %v, want apperr.KindPermanent", err)
	}
}

func TestService_ListPrinters_CallsStatusExactlyOnce(t *testing.T) {
	s := newTestService()
	fp := &statusFakePrinter{status: printer.Status{Online: true}}
	s.Printers = map[string]printer.Printer{"front-desk": fp}
	s.Profiles = map[string]printer.Profile{"front-desk": {}}
	s.Connections = map[string]app.ConnectionSummary{"front-desk": {}}

	if _, err := s.ListPrinters(context.Background()); err != nil {
		t.Fatalf("ListPrinters() error = %v, want nil", err)
	}
	if fp.calls != 1 {
		t.Errorf("Status was called %d times, want exactly 1", fp.calls)
	}
}
