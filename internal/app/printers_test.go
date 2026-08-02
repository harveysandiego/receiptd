package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

// barrierPrinter is a printer.Printer test double whose Status blocks
// until either released or ctx is done, letting a test observe exactly
// when Status was invoked (via started) and control exactly when it
// returns — used below to prove ListPrinters checks printers
// concurrently rather than one at a time, and that a printer stuck past
// its deadline doesn't prevent the others' results from being reported.
type barrierPrinter struct {
	started chan struct{}
	release chan struct{}
}

func newBarrierPrinter() *barrierPrinter {
	return &barrierPrinter{started: make(chan struct{}), release: make(chan struct{})}
}

func (p *barrierPrinter) Send(_ context.Context, _ []byte) error { return nil }

func (p *barrierPrinter) Status(ctx context.Context) (printer.Status, error) {
	close(p.started)
	select {
	case <-p.release:
		return printer.Status{Online: true}, nil
	case <-ctx.Done():
		return printer.Status{}, ctx.Err()
	}
}

func (p *barrierPrinter) Close() error { return nil }

// TestService_ListPrinters_OneSlowPrinterTimesOut_PartialResultsStillReturned
// proves a printer that never answers is capped by its own derived
// deadline (here, the outer ctx's short timeout, which context.WithTimeout
// takes as the earlier of the two — see printerStatusTimeout) rather than
// blocking the call indefinitely, and that the healthy printer's real
// status is still reported alongside it — a partial result, not a failed
// whole request.
func TestService_ListPrinters_OneSlowPrinterTimesOut_PartialResultsStillReturned(t *testing.T) {
	slow := newBarrierPrinter() // never released — only reacts to ctx.Done()
	fast := &statusFakePrinter{status: printer.Status{Online: true}}

	s := newTestService()
	s.Printers = map[string]printer.Printer{"slow": slow, "fast": fast}
	s.Profiles = map[string]printer.Profile{"slow": {}, "fast": {}}
	s.Connections = map[string]app.ConnectionSummary{"slow": {}, "fast": {}}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	start := time.Now()
	summaries, err := s.ListPrinters(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("ListPrinters() error = %v, want nil (one slow printer must not fail the whole listing)", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("ListPrinters() took %v, want it bounded by the short outer deadline, not printerStatusTimeout's full 5s", elapsed)
	}
	if len(summaries) != 2 {
		t.Fatalf("len(ListPrinters()) = %d, want 2 (both printers reported, not just the fast one)", len(summaries))
	}

	byName := make(map[string]app.PrinterSummary, len(summaries))
	for _, s := range summaries {
		byName[s.Name] = s
	}
	if !byName["fast"].Online {
		t.Errorf("fast printer Online = %v, want true", byName["fast"].Online)
	}
	if byName["slow"].Online {
		t.Error("slow printer Online = true, want false — it never responded before the deadline")
	}
	if byName["slow"].StatusDetail == "" {
		t.Error("slow printer StatusDetail is empty, want a timeout/cancellation detail")
	}
}

// TestService_ListPrinters_MultipleSlowPrinters_CheckedConcurrentlyNotSerially
// proves three printers' Status methods are all invoked before any of
// them returns — if ListPrinters checked printers one at a time, only the
// first would have started by the time this test observes them, since
// nothing is released until all three have been confirmed started.
func TestService_ListPrinters_MultipleSlowPrinters_CheckedConcurrentlyNotSerially(t *testing.T) {
	printers := map[string]*barrierPrinter{"a": newBarrierPrinter(), "b": newBarrierPrinter(), "c": newBarrierPrinter()}

	s := newTestService()
	s.Printers = map[string]printer.Printer{}
	s.Profiles = map[string]printer.Profile{}
	s.Connections = map[string]app.ConnectionSummary{}
	for name, p := range printers {
		s.Printers[name] = p
		s.Profiles[name] = printer.Profile{}
		s.Connections[name] = app.ConnectionSummary{}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := s.ListPrinters(context.Background()); err != nil {
			t.Errorf("ListPrinters() error = %v, want nil", err)
		}
	}()

	for name, p := range printers {
		select {
		case <-p.started:
		case <-time.After(2 * time.Second):
			t.Fatalf("printer %q's Status was never invoked — printers are not being checked concurrently", name)
		}
	}

	for _, p := range printers {
		close(p.release)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ListPrinters never returned after every printer was released")
	}
}

// TestService_ListPrinters_ResultsSortedRegardlessOfCompletionOrder proves
// the final sort (ListPrinters' last step, after every goroutine has
// finished) overrides completion order rather than the other way around:
// "zzz" answers immediately while "aaa" is held until released last, so
// if sorting happened incrementally as results arrived — or didn't
// happen at all — "zzz" would end up first.
func TestService_ListPrinters_ResultsSortedRegardlessOfCompletionOrder(t *testing.T) {
	aaa := newBarrierPrinter() // released last, despite sorting first
	zzz := &statusFakePrinter{status: printer.Status{Online: true}}

	s := newTestService()
	s.Printers = map[string]printer.Printer{"aaa": aaa, "zzz": zzz}
	s.Profiles = map[string]printer.Profile{"aaa": {}, "zzz": {}}
	s.Connections = map[string]app.ConnectionSummary{"aaa": {}, "zzz": {}}

	resultCh := make(chan []app.PrinterSummary, 1)
	go func() {
		summaries, err := s.ListPrinters(context.Background())
		if err != nil {
			t.Errorf("ListPrinters() error = %v, want nil", err)
		}
		resultCh <- summaries
	}()

	<-aaa.started // zzz has necessarily already completed by this point
	close(aaa.release)

	select {
	case summaries := <-resultCh:
		if len(summaries) != 2 || summaries[0].Name != "aaa" || summaries[1].Name != "zzz" {
			t.Errorf("ListPrinters() = %+v, want [aaa, zzz] in that order despite zzz finishing first", summaries)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ListPrinters never returned")
	}
}

// TestService_ListPrinters_ContextCancellation_ReturnsPromptly proves
// cancelling ctx stops a stuck Status check immediately, rather than
// ListPrinters waiting out the rest of printerStatusTimeout regardless.
func TestService_ListPrinters_ContextCancellation_ReturnsPromptly(t *testing.T) {
	slow := newBarrierPrinter() // never released — only reacts to ctx.Done()

	s := newTestService()
	s.Printers = map[string]printer.Printer{"slow": slow}
	s.Profiles = map[string]printer.Profile{"slow": {}}
	s.Connections = map[string]app.ConnectionSummary{"slow": {}}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	var elapsed time.Duration
	go func() {
		defer close(done)
		start := time.Now()
		if _, err := s.ListPrinters(ctx); err != nil {
			t.Errorf("ListPrinters() error = %v, want nil", err)
		}
		elapsed = time.Since(start)
	}()

	<-slow.started
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ListPrinters did not return after ctx was cancelled")
	}
	if elapsed > time.Second {
		t.Errorf("ListPrinters took %v after cancellation, want it to return promptly rather than waiting out printerStatusTimeout", elapsed)
	}
}

func TestService_PrinterNames_NoPrinters_ReturnsEmptySlice(t *testing.T) {
	s := newTestService()

	names := s.PrinterNames()
	if len(names) != 0 {
		t.Errorf("PrinterNames() = %v, want empty", names)
	}
}

func TestService_PrinterNames_ReturnsSortedNames(t *testing.T) {
	s := newTestService()
	s.Printers = map[string]printer.Printer{
		"zebra":      &statusFakePrinter{},
		"front-desk": &statusFakePrinter{},
		"kitchen":    &statusFakePrinter{},
	}

	got := s.PrinterNames()
	want := []string{"front-desk", "kitchen", "zebra"}
	if len(got) != len(want) {
		t.Fatalf("PrinterNames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("PrinterNames() = %v, want %v", got, want)
			break
		}
	}
}

// TestService_PrinterNames_DoesNotProbeStatus proves PrinterNames reads
// only the Printers map's keys — a printer whose Status would block
// forever must not prevent PrinterNames from returning.
func TestService_PrinterNames_DoesNotProbeStatus(t *testing.T) {
	s := newTestService()
	s.Printers = map[string]printer.Printer{"front-desk": newBarrierPrinter()}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.PrinterNames()
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("PrinterNames did not return promptly — it must not call Status")
	}
}
