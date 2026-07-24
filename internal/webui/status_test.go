package webui_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/assets"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/webui"
)

func statusRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/status", nil)
}

// TestStatusHandler_RendersJSON_MatchingDashboard proves GET /status's
// full contract as dashboard.js's polling target
// (docs/adr/0025-dashboard-updates-via-polling.md): 200, a JSON (not
// HTML) Content-Type, Cache-Control: no-store so a poll is never served
// stale, and a body reporting the same printer/queue counts
// DashboardHandler renders into HTML — sourced only through the
// ListPrinters/QueueSummary service seam.
func TestStatusHandler_RendersJSON_MatchingDashboard(t *testing.T) {
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
	router.ServeHTTP(rec, statusRequest())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want an application/json prefix", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", cc, "no-store")
	}

	var got struct {
		PrinterCount   int    `json:"printer_count"`
		PrintersOnline int    `json:"printers_online"`
		StatusMessage  string `json:"status_message"`
		QueuePending   int    `json:"queue_pending"`
		QueueRunning   int    `json:"queue_running"`
		QueueDone      int    `json:"queue_done"`
		QueueFailed    int    `json:"queue_failed"`
		QueueCancelled int    `json:"queue_cancelled"`
		QueueTotal     int    `json:"queue_total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(%s): %v", rec.Body, err)
	}

	want := struct {
		PrinterCount   int
		PrintersOnline int
		StatusMessage  string
		QueuePending   int
		QueueRunning   int
		QueueDone      int
		QueueFailed    int
		QueueCancelled int
		QueueTotal     int
	}{
		PrinterCount: 2, PrintersOnline: 1, StatusMessage: "1 of 2 printers offline",
		QueuePending: 1, QueueRunning: 0, QueueDone: 1, QueueFailed: 1, QueueCancelled: 0, QueueTotal: 3,
	}
	if got.PrinterCount != want.PrinterCount || got.PrintersOnline != want.PrintersOnline ||
		got.StatusMessage != want.StatusMessage || got.QueuePending != want.QueuePending ||
		got.QueueRunning != want.QueueRunning || got.QueueDone != want.QueueDone ||
		got.QueueFailed != want.QueueFailed || got.QueueCancelled != want.QueueCancelled ||
		got.QueueTotal != want.QueueTotal {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// TestStatusHandler_ServiceError_RendersGenericJSONErrorWithoutLeakingDetail
// proves a service-layer failure produces a non-200 status and a generic
// JSON error message, never the underlying error text — the same trust
// boundary renderError applies to HTML pages, applied here to /status's
// JSON body.
func TestStatusHandler_ServiceError_RendersGenericJSONErrorWithoutLeakingDetail(t *testing.T) {
	svc := app.New(queue.New(errQueueSummaryStore{}, dashboardNoopProcessor{}))
	svc.Assets = assets.NewMemoryStore()

	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, statusRequest())

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusInternalServerError, rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, "disk error") {
		t.Errorf("body leaks the underlying error detail: %s", body)
	}
	if !strings.Contains(body, "could not load") {
		t.Errorf("body = %s, want a generic could-not-load message", body)
	}
}

// errQueueSummaryStore is a queue.Store test double whose List always
// fails, letting the service-error test observe QueueSummary's error
// propagation without a real Store.
type errQueueSummaryStore struct{}

func (errQueueSummaryStore) Save(context.Context, *queue.Job) error { panic("not used") }
func (errQueueSummaryStore) Get(context.Context, string) (*queue.Job, error) {
	panic("not used")
}
func (errQueueSummaryStore) List(context.Context, queue.Filter) ([]*queue.Job, error) {
	return nil, apperr.Wrap(apperr.KindPermanent, "queue.Store.List", errors.New("disk error"))
}
func (errQueueSummaryStore) NextPending(context.Context) (*queue.Job, error) {
	panic("not used")
}
func (errQueueSummaryStore) ClaimNextPending(context.Context, string) (*queue.Job, error) {
	panic("not used")
}
func (errQueueSummaryStore) EnqueueIdempotent(context.Context, *queue.Job, time.Time) (*queue.Job, bool, error) {
	panic("not used")
}
