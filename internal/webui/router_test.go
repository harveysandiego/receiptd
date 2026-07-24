package webui_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/webui"
)

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
