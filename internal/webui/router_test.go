package webui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/webui"
)

// TestNewRouter_EveryStubRoute_Returns501 proves every route in the
// Milestone 4 contract (docs/ARCHITECTURE.md §10) that still lacks its
// own slice exists and answers 501. GET / is covered separately in
// dashboard_test.go now that the Dashboard slice has replaced its stub.
func TestNewRouter_EveryStubRoute_Returns501(t *testing.T) {
	svc := &app.Service{}
	router := webui.NewRouter(svc)

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/status"},
		{http.MethodGet, "/printers"},
		{http.MethodGet, "/assets"},
		{http.MethodPost, "/assets"},
		{http.MethodPost, "/assets/logo.png/delete"},
		{http.MethodGet, "/preview"},
		{http.MethodPost, "/preview"},
		{http.MethodGet, "/print"},
		{http.MethodPost, "/print"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotImplemented {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body)
			}
		})
	}
}

// TestNewRouter_StubPage_RendersPlaceholderHTML proves a stub route
// actually executes the shared base template rather than writing a bare
// status code — the render helper and base.tmpl are wired together, not
// just present.
func TestNewRouter_StubPage_RendersPlaceholderHTML(t *testing.T) {
	router := webui.NewRouter(&app.Service{})

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want a text/html prefix", ct)
	}
	if !strings.Contains(rec.Body.String(), "Status is not implemented yet.") {
		t.Errorf("body = %s, want it to mention the page is not implemented", rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `href="/printers"`) {
		t.Errorf("body = %s, want the shared base layout's nav to be present", rec.Body)
	}
}

// TestNewRouter_StaticAssets_Served proves web/static/style.css is
// reachable under /static/, the one route outside the routing contract
// table that the base layout depends on (<link rel="stylesheet"
// href="/static/style.css">).
func TestNewRouter_StaticAssets_Served(t *testing.T) {
	router := webui.NewRouter(&app.Service{})

	req := httptest.NewRequest(http.MethodGet, "/static/style.css", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.Len() == 0 {
		t.Error("body is empty, want style.css contents")
	}
}

// TestNewRouter_UnknownRoute_NotFound proves the router doesn't swallow
// every unmatched path behind the dashboard's "GET /" registration —
// http.ServeMux's "/" pattern is a subtree match, so this pins that an
// arbitrary unknown path still 404s rather than silently reaching
// DashboardHandler.
func TestNewRouter_UnknownRoute_NotFound(t *testing.T) {
	router := webui.NewRouter(&app.Service{})

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
