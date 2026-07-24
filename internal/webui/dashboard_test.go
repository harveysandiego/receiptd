package webui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/assets"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/webui"
)

// dashboardFakePrinter is a printer.Printer test double letting each test
// fix exactly what Status reports.
type dashboardFakePrinter struct {
	status printer.Status
}

func (p *dashboardFakePrinter) Send(_ context.Context, _ []byte) error { return nil }
func (p *dashboardFakePrinter) Status(_ context.Context) (printer.Status, error) {
	return p.status, nil
}
func (p *dashboardFakePrinter) Close() error { return nil }

// dashboardNoopProcessor is a queue.Processor test double: DashboardHandler
// never calls Process, only Queue.List (via Service.QueueSummary), but
// queue.New requires one.
type dashboardNoopProcessor struct{}

func (dashboardNoopProcessor) Process(_ context.Context, _ *queue.Job) error { return nil }

func dashboardRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/", nil)
}

// TestDashboardHandler_RendersOverview_WithConfiguredData proves the
// happy path: one online and one offline printer, two stored assets, and
// jobs in different states all surface as expected content, sourced only
// through the ListPrinters/ListAssets/QueueSummary service seam.
func TestDashboardHandler_RendersOverview_WithConfiguredData(t *testing.T) {
	store := queue.NewMemoryStore()
	q := queue.New(store, dashboardNoopProcessor{})
	svc := app.New(q)
	svc.Printers = map[string]printer.Printer{
		"front-desk": &dashboardFakePrinter{status: printer.Status{Online: true}},
		"kitchen":    &dashboardFakePrinter{status: printer.Status{Online: false, Detail: "connection refused"}},
	}
	svc.Profiles = map[string]printer.Profile{
		"front-desk": {},
		"kitchen":    {},
	}
	svc.Connections = map[string]app.ConnectionSummary{
		"front-desk": {Transport: "network", Address: "192.168.1.50:9100"},
		"kitchen":    {Transport: "network", Address: "192.168.1.51:9100"},
	}
	svc.Assets = assets.NewMemoryStore()
	if err := svc.Assets.Put(context.Background(), "logo.png", []byte("x")); err != nil {
		t.Fatalf("Assets.Put: %v", err)
	}
	if err := svc.Assets.Put(context.Background(), "banner.png", []byte("y")); err != nil {
		t.Fatalf("Assets.Put: %v", err)
	}
	for _, j := range []*queue.Job{
		{ID: "1", PrinterName: "front-desk", State: queue.JobPending},
		{ID: "2", PrinterName: "front-desk", State: queue.JobDone},
		{ID: "3", PrinterName: "front-desk", State: queue.JobFailed},
	} {
		if err := store.Save(context.Background(), j); err != nil {
			t.Fatalf("Store.Save: %v", err)
		}
	}

	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, dashboardRequest())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"1 / 2 online",
		"1 of 2 printers offline",
		"2 stored",
		"Pending: 1",
		"Done: 1",
		"Failed: 1",
		"Cancelled: 0",
		"3 total",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q; body = %s", want, body)
		}
	}
}

// TestDashboardHandler_AllPrintersOnline_ReportsAllOnline proves the third
// branch of the status message (alongside "no printers configured" and
// "N of M offline", covered by the other two tests): every configured
// printer reachable reports "All printers online".
func TestDashboardHandler_AllPrintersOnline_ReportsAllOnline(t *testing.T) {
	svc := app.New(queue.New(queue.NewMemoryStore(), dashboardNoopProcessor{}))
	svc.Printers = map[string]printer.Printer{
		"front-desk": &dashboardFakePrinter{status: printer.Status{Online: true}},
	}
	svc.Profiles = map[string]printer.Profile{"front-desk": {}}
	svc.Connections = map[string]app.ConnectionSummary{"front-desk": {}}
	svc.Assets = assets.NewMemoryStore()

	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, dashboardRequest())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
	if body := rec.Body.String(); !strings.Contains(body, "All printers online") {
		t.Errorf("body = %s, want it to contain %q", body, "All printers online")
	}
}

// TestDashboardHandler_EmptyState_ReportsZeroCounts proves an instance
// with no configured printers, no stored assets, and an empty queue
// renders cleanly rather than erroring or showing blank/garbled content.
func TestDashboardHandler_EmptyState_ReportsZeroCounts(t *testing.T) {
	svc := app.New(queue.New(queue.NewMemoryStore(), dashboardNoopProcessor{}))
	svc.Assets = assets.NewMemoryStore()

	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, dashboardRequest())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"0 / 0 online",
		"No printers configured",
		"0 stored",
		"0 total",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q; body = %s", want, body)
		}
	}
}

// TestDashboardHandler_ServiceError_RendersGenericErrorWithoutLeakingDetail
// proves a service-layer failure (here, ListPrinters' construction-bug
// invariant: a Printers entry with no matching Profile) produces a
// non-200 status and a generic message, never the underlying error text.
func TestDashboardHandler_ServiceError_RendersGenericErrorWithoutLeakingDetail(t *testing.T) {
	svc := app.New(queue.New(queue.NewMemoryStore(), dashboardNoopProcessor{}))
	svc.Printers = map[string]printer.Printer{
		"front-desk": &dashboardFakePrinter{},
	}
	svc.Connections = map[string]app.ConnectionSummary{
		"front-desk": {},
	}
	// svc.Profiles deliberately left nil, forcing ListPrinters'
	// apperr.KindPermanent invariant-violation path.

	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, dashboardRequest())

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
