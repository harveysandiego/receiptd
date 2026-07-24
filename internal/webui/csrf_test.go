package webui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/webui"
)

// csrfTokenFromPage GETs path through router and scrapes the hidden
// csrf_token field's value out of the rendered HTML, mirroring how a real
// browser reads a protected form before submitting it (docs/adr/0027).
// The token itself is a per-process value, not per-router or per-Service,
// but fetching it through the router under test — rather than relying on
// that implementation detail — is what a real client actually does.
func csrfTokenFromPage(t *testing.T, router http.Handler, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want %d, body = %s", path, rec.Code, http.StatusOK, rec.Body)
	}

	body := rec.Body.String()
	const marker = `name="csrf_token" value="`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("GET %s: no csrf_token field found in body: %s", path, body)
	}
	rest := body[i+len(marker):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		t.Fatalf("GET %s: malformed csrf_token field in body: %s", path, body)
	}
	return rest[:j]
}

// TestCSRF_MissingToken_RejectsPrintSubmission proves a POST /print
// carrying no csrf_token field at all is rejected before Service.Print is
// ever reached — no Job is created.
func TestCSRF_MissingToken_RejectsPrintSubmission(t *testing.T) {
	store := queue.NewMemoryStore()
	svc := app.New(queue.New(store, dashboardNoopProcessor{}))
	router := webui.NewRouter(svc)

	form := url.Values{"receipt": {validPrintReceiptJSON}, "printer": {"front-desk"}}
	req := httptest.NewRequest(http.MethodPost, "/print", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
	jobs, err := store.List(context.Background(), queue.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v, want nil", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(store.List()) = %d, want 0 (a missing CSRF token must not create a Job)", len(jobs))
	}
}

// TestCSRF_InvalidToken_RejectsPrintSubmission proves a POST /print
// carrying a well-formed but wrong csrf_token value is rejected the same
// way a missing one is.
func TestCSRF_InvalidToken_RejectsPrintSubmission(t *testing.T) {
	store := queue.NewMemoryStore()
	svc := app.New(queue.New(store, dashboardNoopProcessor{}))
	router := webui.NewRouter(svc)

	form := url.Values{
		"receipt":    {validPrintReceiptJSON},
		"printer":    {"front-desk"},
		"csrf_token": {"not-the-real-token"},
	}
	req := httptest.NewRequest(http.MethodPost, "/print", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
	jobs, err := store.List(context.Background(), queue.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v, want nil", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(store.List()) = %d, want 0 (an invalid CSRF token must not create a Job)", len(jobs))
	}
}

// TestCSRF_ValidToken_AllowsPrintSubmission proves a token scraped from
// the real GET /print form (the only realistic way a legitimate browser
// obtains one) is accepted.
func TestCSRF_ValidToken_AllowsPrintSubmission(t *testing.T) {
	store := queue.NewMemoryStore()
	svc := app.New(queue.New(store, dashboardNoopProcessor{}))
	router := webui.NewRouter(svc)

	token := csrfTokenFromPage(t, router, "/print")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, printFormRequest(token, validPrintReceiptJSON, "front-desk"))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusSeeOther, rec.Body)
	}
	jobs, err := store.List(context.Background(), queue.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v, want nil", err)
	}
	if len(jobs) != 1 {
		t.Errorf("len(store.List()) = %d, want 1 (a valid CSRF token must let the submission through)", len(jobs))
	}
}

// TestCSRF_MissingToken_RejectsAssetUpload and
// TestCSRF_MissingToken_RejectsAssetDelete prove the same protection is
// wired into the other two protected routes docs/adr/0027 names, not
// just Print.
func TestCSRF_MissingToken_RejectsAssetUpload(t *testing.T) {
	svc := newAssetsTestService()
	router := webui.NewRouter(svc)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, uploadRequest(t, "", "logo.png", "fake-png-bytes"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
}

func TestCSRF_MissingToken_RejectsAssetDelete(t *testing.T) {
	svc := newAssetsTestService()
	router := webui.NewRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/assets/logo.png/delete", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
}
