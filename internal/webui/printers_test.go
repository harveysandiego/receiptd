package webui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/webui"
)

func printersRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/printers", nil)
}

// TestPrintersHandler_RendersConfiguredPrinters_WithExpectedFields proves
// the happy path: every PrinterSummary field this page is responsible for
// showing (name, transport, address, profile, live status) appears in the
// rendered body, for both an online and an offline printer — and that an
// offline printer's raw StatusDetail (a transport-level error, here
// "connection refused") is never leaked into that body, only logged
// server-side (printers.go's ServeHTTP).
func TestPrintersHandler_RendersConfiguredPrinters_WithExpectedFields(t *testing.T) {
	svc := app.New(queue.New(queue.NewMemoryStore(), dashboardNoopProcessor{}))
	svc.Printers = map[string]printer.Printer{
		"front-desk": &dashboardFakePrinter{status: printer.Status{Online: true}},
		"kitchen":    &dashboardFakePrinter{status: printer.Status{Online: false, Detail: "connection refused"}},
	}
	svc.Profiles = map[string]printer.Profile{
		"front-desk": {WidthDots: 384, DPI: 203, SupportsCut: true, SupportsPartialCut: true},
		"kitchen":    {WidthDots: 576, DPI: 300, SupportsCut: false, SupportsPartialCut: false},
	}
	svc.Connections = map[string]app.ConnectionSummary{
		"front-desk": {Transport: "network", Address: "192.168.1.50:9100"},
		"kitchen":    {Transport: "network", Address: "192.168.1.51:9100"},
	}

	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, printersRequest())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"front-desk",
		"192.168.1.50:9100",
		"384",
		"203",
		"Yes",
		"Online",
		"kitchen",
		"192.168.1.51:9100",
		"576",
		"300",
		"No",
		"Offline",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q; body = %s", want, body)
		}
	}
	if strings.Contains(body, "connection refused") {
		t.Errorf("body leaks the printer's raw StatusDetail: %s", body)
	}
}

// TestPrintersHandler_DeterministicOrdering_PrintersSortedByName proves
// the table renders in a stable order regardless of Go's undefined map
// iteration order, matching ListPrinters' own name-sort contract.
func TestPrintersHandler_DeterministicOrdering_PrintersSortedByName(t *testing.T) {
	svc := app.New(queue.New(queue.NewMemoryStore(), dashboardNoopProcessor{}))
	svc.Printers = map[string]printer.Printer{
		"zzz-printer": &dashboardFakePrinter{status: printer.Status{Online: true}},
		"aaa-printer": &dashboardFakePrinter{status: printer.Status{Online: true}},
	}
	svc.Profiles = map[string]printer.Profile{
		"zzz-printer": {},
		"aaa-printer": {},
	}
	svc.Connections = map[string]app.ConnectionSummary{
		"zzz-printer": {},
		"aaa-printer": {},
	}

	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, printersRequest())

	body := rec.Body.String()
	first := strings.Index(body, "aaa-printer")
	second := strings.Index(body, "zzz-printer")
	if first == -1 || second == -1 || first > second {
		t.Errorf("body = %s, want aaa-printer to appear before zzz-printer", body)
	}
}

// TestPrintersHandler_OfflineWithoutDetail_ReportsBareOffline proves an
// offline printer with no StatusDetail at all still reports plainly
// "Offline", with no stray trailing separator from the (now removed)
// detail-suffix formatting.
func TestPrintersHandler_OfflineWithoutDetail_ReportsBareOffline(t *testing.T) {
	svc := app.New(queue.New(queue.NewMemoryStore(), dashboardNoopProcessor{}))
	svc.Printers = map[string]printer.Printer{
		"front-desk": &dashboardFakePrinter{status: printer.Status{Online: false}},
	}
	svc.Profiles = map[string]printer.Profile{"front-desk": {}}
	svc.Connections = map[string]app.ConnectionSummary{"front-desk": {}}

	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, printersRequest())

	body := rec.Body.String()
	if !strings.Contains(body, "Offline") {
		t.Errorf("body = %s, want it to report the printer offline", body)
	}
	if strings.Contains(body, "Offline:") {
		t.Errorf("body = %s, want no detail suffix when Status reported none", body)
	}
}

// TestPrintersHandler_EmptyState_RendersNoPrintersMessage proves an
// instance with no configured printers renders a clear empty state
// instead of an empty or malformed table.
func TestPrintersHandler_EmptyState_RendersNoPrintersMessage(t *testing.T) {
	svc := app.New(queue.New(queue.NewMemoryStore(), dashboardNoopProcessor{}))

	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, printersRequest())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
	if body := rec.Body.String(); !strings.Contains(body, "No printers configured") {
		t.Errorf("body = %s, want it to contain the empty-state message", body)
	}
}

// TestPrintersHandler_ServiceError_RendersGenericErrorWithoutLeakingDetail
// proves a service-layer failure (here, ListPrinters' construction-bug
// invariant: a Printers entry with no matching Profile) produces a
// non-200 status and a generic message, never the underlying error text —
// the same contract dashboard_test.go pins for DashboardHandler.
func TestPrintersHandler_ServiceError_RendersGenericErrorWithoutLeakingDetail(t *testing.T) {
	svc := app.New(queue.New(queue.NewMemoryStore(), dashboardNoopProcessor{}))
	svc.Printers = map[string]printer.Printer{
		"front-desk": &dashboardFakePrinter{},
	}
	svc.Connections = map[string]app.ConnectionSummary{
		"front-desk": {},
	}
	// svc.Profiles deliberately left nil, forcing ListPrinters' apperr.KindPermanent invariant-violation path.

	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, printersRequest())

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusInternalServerError, rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, "no configured Profile") {
		t.Errorf("body leaks the underlying error detail: %s", body)
	}
	if !strings.Contains(body, "could not load") {
		t.Errorf("body = %s, want a generic could-not-load message", body)
	}
}
