package webui_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"html"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/webui"
)

// validPreviewReceiptJSON is a Receipt that both decodes and validates,
// for tests exercising everything past JSON decoding.
const validPreviewReceiptJSON = `{"version":1,"elements":[{"type":"text","content":"hello"}]}`

// newPreviewTestService returns a Service configured with one printer,
// "front-desk", so Preview's printerName resolution has a Profile to
// succeed against (docs/adr/0006-preview-requires-printer-profile.md).
func newPreviewTestService() *app.Service {
	svc := app.New(queue.New(queue.NewMemoryStore(), dashboardNoopProcessor{}))
	svc.Profiles = map[string]printer.Profile{"front-desk": {}}
	return svc
}

// previewFormRequest builds a form-encoded POST /preview request, mirroring
// how preview.tmpl's form actually submits (no JavaScript, no multipart —
// just the two text fields the task specifies).
func previewFormRequest(receiptJSON, printerName string) *http.Request {
	form := url.Values{"receipt": {receiptJSON}, "printer": {printerName}}
	req := httptest.NewRequest(http.MethodPost, "/preview", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestPreviewHandler_Show_RendersFormAndPlaceholder(t *testing.T) {
	router := webui.NewRouter(newPreviewTestService())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/preview", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `action="/preview"`) {
		t.Errorf("body = %s, want a form posting to /preview", body)
	}
	if !strings.Contains(body, "No preview generated yet") {
		t.Errorf("body = %s, want the explanatory placeholder before any submission", body)
	}
	if strings.Contains(body, "<img") {
		t.Errorf("body = %s, want no preview image before any submission", body)
	}
}

// TestPreviewHandler_Generate_ValidReceipt_RendersPreviewImage proves the
// happy path end to end: a submitted Receipt reaches Service.Preview and
// the returned PNG bytes come back as a decodable, embedded image.
func TestPreviewHandler_Generate_ValidReceipt_RendersPreviewImage(t *testing.T) {
	router := webui.NewRouter(newPreviewTestService())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, previewFormRequest(validPreviewReceiptJSON, "front-desk"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
	body := rec.Body.String()

	const marker = `src="data:image/png;base64,`
	start := strings.Index(body, marker)
	if start == -1 {
		t.Fatalf("body = %s, want an embedded PNG data URI", body)
	}
	start += len(marker)
	end := strings.IndexByte(body[start:], '"')
	if end == -1 {
		t.Fatalf("body = %s, want a closing quote terminating the data URI", body)
	}

	// html/template HTML-entity-encodes characters like '+' inside the
	// attribute value (e.g. "&#43;"); a browser decodes these back before
	// treating the result as a data URI, so the test does the same.
	decoded, err := base64.StdEncoding.DecodeString(html.UnescapeString(body[start : start+end]))
	if err != nil {
		t.Fatalf("base64.DecodeString() error = %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(decoded)); err != nil {
		t.Fatalf("png.Decode() error = %v, want a valid PNG", err)
	}
}

// TestPreviewHandler_Generate_MalformedFormBody_RendersGenericErrorWithoutLeakingDetail
// proves a request body that isn't valid application/x-www-form-urlencoded
// (r.ParseForm's own decode failure) is rejected the same way as any other
// bad request, not a 500 — mirroring
// TestAssetsHandler_Upload_MalformedMultipartBody_RendersValidationError's
// equivalent case for the Assets slice's own form-decoding step.
func TestPreviewHandler_Generate_MalformedFormBody_RendersGenericErrorWithoutLeakingDetail(t *testing.T) {
	router := webui.NewRouter(newPreviewTestService())

	req := httptest.NewRequest(http.MethodPost, "/preview", strings.NewReader("printer=%zz"))
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

// TestPreviewHandler_Generate_OversizedBody_Rejected proves a request
// body over maxRequestBodyBytes is rejected by http.MaxBytesReader before
// r.ParseForm ever finishes reading it, the same size cap internal/api
// already applies (internal/api/status.go's own maxRequestBodyBytes).
func TestPreviewHandler_Generate_OversizedBody_Rejected(t *testing.T) {
	router := webui.NewRouter(newPreviewTestService())

	form := url.Values{"receipt": {strings.Repeat("a", 11<<20)}, "printer": {"front-desk"}}
	req := httptest.NewRequest(http.MethodPost, "/preview", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
}

// TestPreviewHandler_Generate_MalformedJSON_RendersValidationMessage proves
// JSON that can't even be decoded into a receipt.Receipt is reported by the
// Web UI itself, as a validation message on the Preview page — distinct
// from an application-layer error, which uses the generic error page (see
// the RendersGenericErrorWithoutLeakingDetail tests below).
func TestPreviewHandler_Generate_MalformedJSON_RendersValidationMessage(t *testing.T) {
	router := webui.NewRouter(newPreviewTestService())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, previewFormRequest("not json", "front-desk"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<img") {
		t.Errorf("body = %s, want no preview image for malformed JSON", body)
	}
	if !strings.Contains(body, "could not be parsed") {
		t.Errorf("body = %s, want a validation message about the malformed JSON", body)
	}
	if !strings.Contains(body, "not json") {
		t.Errorf("body = %s, want the submitted JSON repopulated in the form", body)
	}
}

// TestPreviewHandler_Generate_InvalidReceiptContent_RendersGenericErrorWithoutLeakingDetail
// proves a Receipt that decodes fine but fails receipt.Validate() (empty
// Text content) is an application-layer error, not "malformed JSON" — it
// goes through the standard renderError mechanism, confirming Preview
// doesn't duplicate receipt.Validate's own rules in the Web UI.
func TestPreviewHandler_Generate_InvalidReceiptContent_RendersGenericErrorWithoutLeakingDetail(t *testing.T) {
	router := webui.NewRouter(newPreviewTestService())
	rec := httptest.NewRecorder()
	body := `{"version":1,"elements":[{"type":"text","content":""}]}`
	router.ServeHTTP(rec, previewFormRequest(body, "front-desk"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
	respBody := rec.Body.String()
	if !strings.Contains(respBody, "could not load") {
		t.Errorf("body = %s, want the generic could-not-load message", respBody)
	}
}

func TestPreviewHandler_Generate_UnconfiguredPrinter_RendersGenericErrorWithoutLeakingDetail(t *testing.T) {
	router := webui.NewRouter(newPreviewTestService())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, previewFormRequest(validPreviewReceiptJSON, "does-not-exist"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, "does-not-exist") {
		t.Errorf("body leaks the requested printer name: %s", body)
	}
	if !strings.Contains(body, "could not load") {
		t.Errorf("body = %s, want the generic could-not-load message", body)
	}
}

// TestPreviewHandler_Generate_ValidReceipt_DoesNotEnqueueAnything pins this
// slice's central constraint: Preview is read-only rendering, never a print
// job.
func TestPreviewHandler_Generate_ValidReceipt_DoesNotEnqueueAnything(t *testing.T) {
	store := queue.NewMemoryStore()
	svc := app.New(queue.New(store, dashboardNoopProcessor{}))
	svc.Profiles = map[string]printer.Profile{"front-desk": {}}

	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, previewFormRequest(validPreviewReceiptJSON, "front-desk"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}

	jobs, err := store.List(context.Background(), queue.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v, want nil", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(store.List()) = %d, want 0 (Preview must not enqueue a Job)", len(jobs))
	}
}
