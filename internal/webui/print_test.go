package webui_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/webui"
)

// failingSaveStore is a queue.Store double whose Save always fails, so a
// test can exercise Service.Print's error path beyond receipt validation
// (the one failure memoryStore itself can never produce). Every other
// method panics — Submit's success path never reaches them.
type failingSaveStore struct{}

func (failingSaveStore) Save(context.Context, *queue.Job) error {
	return apperr.Wrap(apperr.KindPermanent, "test.failingSaveStore", fmt.Errorf("disk full"))
}
func (failingSaveStore) Get(context.Context, string) (*queue.Job, error) { panic("not used") }
func (failingSaveStore) List(context.Context, queue.Filter) ([]*queue.Job, error) {
	panic("not used")
}
func (failingSaveStore) NextPending(context.Context) (*queue.Job, error) { panic("not used") }
func (failingSaveStore) ClaimNextPending(context.Context, string) (*queue.Job, error) {
	panic("not used")
}
func (failingSaveStore) EnqueueIdempotent(context.Context, *queue.Job, time.Time) (*queue.Job, bool, error) {
	panic("not used")
}

// validPrintReceiptJSON is a Receipt that both decodes and validates, for
// tests exercising everything past JSON decoding.
const validPrintReceiptJSON = `{"version":1,"elements":[{"type":"text","content":"hello"}]}`

// printFormRequest builds a form-encoded POST /print request, mirroring
// how print.tmpl's form actually submits — including its hidden
// csrf_token field (docs/adr/0027). token normally comes from
// csrfTokenFromPage against a prior GET /print, the same way a real
// browser would read it out of the form it's submitting.
func printFormRequest(token, receiptJSON, printerName string) *http.Request {
	form := url.Values{"receipt": {receiptJSON}, "printer": {printerName}, "csrf_token": {token}}
	req := httptest.NewRequest(http.MethodPost, "/print", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestPrintHandler_Show_RendersFormAndExplanation(t *testing.T) {
	svc := app.New(queue.New(queue.NewMemoryStore(), dashboardNoopProcessor{}))
	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/print", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `action="/print"`) {
		t.Errorf("body = %s, want a form posting to /print", body)
	}
	if !strings.Contains(body, "print") && !strings.Contains(body, "Print") {
		t.Errorf("body = %s, want explanatory text about printing", body)
	}
	if strings.Contains(body, "confirmation-message") {
		t.Errorf("body = %s, want no confirmation before any submission", body)
	}
}

// TestPrintHandler_Show_WithJobQueryParam_RendersConfirmation proves the
// PRG redirect's "?job=" query parameter is what drives the confirmation
// on the redirected GET — the architectural counterpart to
// TestPrintHandler_Submit_ValidReceipt_RedirectsWithJobID below.
func TestPrintHandler_Show_WithJobQueryParam_RendersConfirmation(t *testing.T) {
	svc := app.New(queue.New(queue.NewMemoryStore(), dashboardNoopProcessor{}))
	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/print?job=abc123", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "abc123") {
		t.Errorf("body = %s, want the submitted job ID confirmed", body)
	}
	if strings.Contains(body, "printed") {
		t.Errorf("body = %s, want the confirmation to describe submission, not completed printing", body)
	}
}

// TestPrintHandler_Submit_ValidReceipt_RedirectsWithJobID proves a
// successful submission follows PRG: a 303 redirect back to GET /print
// carrying the new Job's ID, rather than rendering a response body
// directly from the POST.
func TestPrintHandler_Submit_ValidReceipt_RedirectsWithJobID(t *testing.T) {
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
		t.Fatalf("len(store.List()) = %d, want 1", len(jobs))
	}

	wantLocation := "/print?job=" + jobs[0].ID
	if loc := rec.Header().Get("Location"); loc != wantLocation {
		t.Errorf("Location = %q, want %q", loc, wantLocation)
	}
}

// TestPrintHandler_Submit_ValidReceipt_CreatesPendingJob is the
// architectural counterpart to Preview's
// TestPreviewHandler_Generate_ValidReceipt_DoesNotEnqueueAnything: this
// slice's whole reason to exist is that, unlike Preview, a successful
// submission creates a real Job.
func TestPrintHandler_Submit_ValidReceipt_CreatesPendingJob(t *testing.T) {
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
		t.Fatalf("len(store.List()) = %d, want 1", len(jobs))
	}
	if jobs[0].PrinterName != "front-desk" {
		t.Errorf("PrinterName = %q, want %q", jobs[0].PrinterName, "front-desk")
	}
	if jobs[0].State != queue.JobPending {
		t.Errorf("State = %v, want %v", jobs[0].State, queue.JobPending)
	}
}

// TestPrintHandler_Submit_MalformedFormBody_RendersGenericErrorWithoutLeakingDetail
// mirrors TestPreviewHandler_Generate_MalformedFormBody_RendersGenericErrorWithoutLeakingDetail.
func TestPrintHandler_Submit_MalformedFormBody_RendersGenericErrorWithoutLeakingDetail(t *testing.T) {
	svc := app.New(queue.New(queue.NewMemoryStore(), dashboardNoopProcessor{}))
	router := webui.NewRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/print", strings.NewReader("printer=%zz"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if body := rec.Body.String(); strings.Contains(body, "%zz") {
		t.Errorf("body leaks the underlying error detail: %s", body)
	}
}

// TestPrintHandler_Submit_OversizedBody_Rejected proves a request body
// over maxRequestBodyBytes is rejected by http.MaxBytesReader before
// r.ParseForm ever finishes reading it — a request this large is
// rejected before verifyCSRF even runs, so no token is needed here.
func TestPrintHandler_Submit_OversizedBody_Rejected(t *testing.T) {
	svc := app.New(queue.New(queue.NewMemoryStore(), dashboardNoopProcessor{}))
	router := webui.NewRouter(svc)

	form := url.Values{"receipt": {strings.Repeat("a", 11<<20)}, "printer": {"front-desk"}}
	req := httptest.NewRequest(http.MethodPost, "/print", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
}

// TestPrintHandler_Submit_MalformedJSON_RendersValidationMessage proves
// JSON that can't be decoded into a receipt.Receipt is reported by the
// Web UI itself and never reaches Service.Print — no Job is created.
func TestPrintHandler_Submit_MalformedJSON_RendersValidationMessage(t *testing.T) {
	store := queue.NewMemoryStore()
	svc := app.New(queue.New(store, dashboardNoopProcessor{}))

	router := webui.NewRouter(svc)
	token := csrfTokenFromPage(t, router, "/print")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, printFormRequest(token, "not json", "front-desk"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "could not be parsed") {
		t.Errorf("body = %s, want a validation message about the malformed JSON", body)
	}
	if !strings.Contains(body, "not json") {
		t.Errorf("body = %s, want the submitted JSON repopulated in the form", body)
	}

	jobs, err := store.List(context.Background(), queue.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v, want nil", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(store.List()) = %d, want 0 (malformed JSON must not create a Job)", len(jobs))
	}
}

// TestPrintHandler_Submit_InvalidReceiptContent_RendersGenericErrorWithoutLeakingDetail
// proves a Receipt that decodes fine but fails receipt.Validate() is an
// application-layer error, not "malformed JSON" — routed through the
// standard renderError mechanism, and no Job is created.
func TestPrintHandler_Submit_InvalidReceiptContent_RendersGenericErrorWithoutLeakingDetail(t *testing.T) {
	store := queue.NewMemoryStore()
	svc := app.New(queue.New(store, dashboardNoopProcessor{}))

	router := webui.NewRouter(svc)
	token := csrfTokenFromPage(t, router, "/print")
	rec := httptest.NewRecorder()
	body := `{"version":1,"elements":[{"type":"text","content":""}]}`
	router.ServeHTTP(rec, printFormRequest(token, body, "front-desk"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if respBody := rec.Body.String(); !strings.Contains(respBody, "could not load") {
		t.Errorf("body = %s, want the generic could-not-load message", respBody)
	}

	jobs, err := store.List(context.Background(), queue.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v, want nil", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(store.List()) = %d, want 0 (an invalid Receipt must not create a Job)", len(jobs))
	}
}

// TestPrintHandler_Submit_UnconfiguredPrinter_StillCreatesJob documents
// Service.Print's actual, already-established behavior (the same method
// internal/api's POST /api/v1/print calls): unlike Preview, which must
// resolve a Profile synchronously to render pixels
// (docs/adr/0006-preview-requires-printer-profile.md), Print only
// validates and enqueues — an unconfigured PrinterName is not rejected
// at submission time, only later, when Queue processing actually
// resolves it. Quick Print does not duplicate a printer-existence check
// Service.Print itself doesn't perform.
func TestPrintHandler_Submit_UnconfiguredPrinter_StillCreatesJob(t *testing.T) {
	store := queue.NewMemoryStore()
	svc := app.New(queue.New(store, dashboardNoopProcessor{}))

	router := webui.NewRouter(svc)
	token := csrfTokenFromPage(t, router, "/print")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, printFormRequest(token, validPrintReceiptJSON, "does-not-exist"))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusSeeOther, rec.Body)
	}

	jobs, err := store.List(context.Background(), queue.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v, want nil", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(store.List()) = %d, want 1", len(jobs))
	}
	if jobs[0].PrinterName != "does-not-exist" {
		t.Errorf("PrinterName = %q, want %q", jobs[0].PrinterName, "does-not-exist")
	}
}

// TestPrintHandler_Submit_QueueSaveFails_RendersGenericErrorWithoutLeakingDetail
// proves a Service.Print failure that isn't receipt validation (here, the
// Store rejecting the write) still reaches the standard renderError path
// rather than a raw 500 or a leaked error string.
func TestPrintHandler_Submit_QueueSaveFails_RendersGenericErrorWithoutLeakingDetail(t *testing.T) {
	svc := app.New(queue.New(failingSaveStore{}, dashboardNoopProcessor{}))

	router := webui.NewRouter(svc)
	token := csrfTokenFromPage(t, router, "/print")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, printFormRequest(token, validPrintReceiptJSON, "front-desk"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusInternalServerError, rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, "disk full") {
		t.Errorf("body leaks the underlying error detail: %s", body)
	}
	if !strings.Contains(body, "could not load") {
		t.Errorf("body = %s, want the generic could-not-load message", body)
	}
}
