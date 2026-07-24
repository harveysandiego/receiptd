package webui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/assets"
	"github.com/harveysandiego/receiptd/internal/queue"
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

// TestNewRouter_EveryResponse_CarriesSecurityHeaders proves the
// defensive headers apply uniformly — an HTML page, the JSON /status
// endpoint, and a static asset alike — since they're set by a middleware
// wrapping the whole router, not by render()/writeStatusJSON
// individually.
func TestNewRouter_EveryResponse_CarriesSecurityHeaders(t *testing.T) {
	svc := app.New(queue.New(queue.NewMemoryStore(), dashboardNoopProcessor{}))
	svc.Assets = assets.NewMemoryStore()
	router := webui.NewRouter(svc)

	for _, path := range []string{"/", "/status", "/static/style.css"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			h := rec.Header()
			if csp := h.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
				t.Errorf("Content-Security-Policy = %q, want it to contain %q", csp, "default-src 'self'")
			}
			if !strings.Contains(h.Get("Content-Security-Policy"), "img-src 'self' data:") {
				t.Errorf("Content-Security-Policy = %q, want img-src to allow data: (Preview's embedded PNG)", h.Get("Content-Security-Policy"))
			}
			if !strings.Contains(h.Get("Content-Security-Policy"), "object-src 'none'") {
				t.Errorf("Content-Security-Policy = %q, want object-src 'none' — this UI never embeds a plugin/object", h.Get("Content-Security-Policy"))
			}
			if !strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
				t.Errorf("Content-Security-Policy = %q, want frame-ancestors 'none'", h.Get("Content-Security-Policy"))
			}
			if got := h.Get("X-Frame-Options"); got != "DENY" {
				t.Errorf("X-Frame-Options = %q, want %q", got, "DENY")
			}
			if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
			}
			if got := h.Get("Referrer-Policy"); got != "same-origin" {
				t.Errorf("Referrer-Policy = %q, want %q", got, "same-origin")
			}
		})
	}
}
