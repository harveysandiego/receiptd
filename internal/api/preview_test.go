package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/api"
	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/assets"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// fakePreviewService is a test double for the interface PreviewHandler
// needs from app.Service, letting most handler tests run without a real
// Queue or renderer.
type fakePreviewService struct {
	png []byte
	err error

	calls      int
	gotReceipt receipt.Receipt
	gotPrinter string
}

func (f *fakePreviewService) Preview(_ context.Context, r receipt.Receipt, printerName string) ([]byte, error) {
	f.calls++
	f.gotReceipt = r
	f.gotPrinter = printerName
	return f.png, f.err
}

func validPreviewRequestBody() []byte {
	return []byte(`{"printer":"front-desk","receipt":{"version":1,"elements":[{"type":"text","content":"hello"}]}}`)
}

func TestPreviewHandler_Success_ReturnsPNGBytesWithStatusOK(t *testing.T) {
	want := []byte("fake-png-bytes")
	svc := &fakePreviewService{png: want}
	h := api.NewPreviewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/preview", bytes.NewReader(validPreviewRequestBody()))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !bytes.Equal(rec.Body.Bytes(), want) {
		t.Errorf("body = %v, want %v", rec.Body.Bytes(), want)
	}
}

func TestPreviewHandler_Success_SetsPNGContentType(t *testing.T) {
	svc := &fakePreviewService{png: []byte("fake-png-bytes")}
	h := api.NewPreviewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/preview", bytes.NewReader(validPreviewRequestBody()))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want %q", ct, "image/png")
	}
}

func TestPreviewHandler_Success_PassesReceiptToService(t *testing.T) {
	svc := &fakePreviewService{png: []byte("fake-png-bytes")}
	h := api.NewPreviewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/preview", bytes.NewReader(validPreviewRequestBody()))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if svc.calls != 1 {
		t.Fatalf("Service.Preview called %d times, want 1", svc.calls)
	}
	if len(svc.gotReceipt.Elements) != 1 {
		t.Fatalf("len(Receipt.Elements) = %d, want 1", len(svc.gotReceipt.Elements))
	}
	text, ok := svc.gotReceipt.Elements[0].(receipt.Text)
	if !ok {
		t.Fatalf("Receipt.Elements[0] = %T, want receipt.Text", svc.gotReceipt.Elements[0])
	}
	if text.Content != "hello" {
		t.Errorf("Content = %q, want %q", text.Content, "hello")
	}
	if svc.gotPrinter != "front-desk" {
		t.Errorf("Printer = %q, want %q", svc.gotPrinter, "front-desk")
	}
}

func TestPreviewHandler_MalformedJSON_ReturnsBadRequest(t *testing.T) {
	svc := &fakePreviewService{}
	h := api.NewPreviewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/preview", bytes.NewReader([]byte(`{"version":`)))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if svc.calls != 0 {
		t.Errorf("Service.Preview called %d times, want 0 for malformed JSON", svc.calls)
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.Error == "" || body.Error == "internal server error" {
		t.Errorf("error = %q, want the detailed JSON decode error (a 4xx response), not the generic 5xx message", body.Error)
	}
}

func TestPreviewHandler_MalformedReceiptElement_ReturnsBadRequest(t *testing.T) {
	svc := &fakePreviewService{}
	h := api.NewPreviewHandler(svc)

	// Syntactically valid JSON, but "content" doesn't fit Text.Content (a
	// string) — mirrors TestPrintHandler_MalformedReceiptElement_ReturnsBadRequest.
	body := []byte(`{"printer":"front-desk","receipt":{"version":1,"elements":[{"type":"text","content":123}]}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/preview", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if svc.calls != 0 {
		t.Errorf("Service.Preview called %d times, want 0 for malformed element", svc.calls)
	}
}

func TestPreviewHandler_BodyTooLarge_ReturnsRequestEntityTooLarge(t *testing.T) {
	svc := &fakePreviewService{}
	h := api.NewPreviewHandler(svc)

	// A large, unterminated JSON string literal: syntactically valid so far
	// as the decoder can tell, so it keeps reading content bytes (rather
	// than failing fast on a syntax error) until MaxBytesReader cuts it off.
	body := append([]byte(`{"printer":"front-desk","receipt":{"elements":[{"type":"text","content":"`), bytes.Repeat([]byte("a"), 10<<20+1)...)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/preview", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
	if svc.calls != 0 {
		t.Errorf("Service.Preview called %d times, want 0 for an oversized body", svc.calls)
	}
}

func TestPreviewHandler_ServiceError_MapsKindToStatus(t *testing.T) {
	tests := []struct {
		name string
		kind apperr.Kind
		want int
		// wantDetailed is true for a 4xx status (body must still carry the
		// underlying detail) and false for a 5xx status (body must be the
		// fixed generic message, with no trace of "boom" or the Op) — see
		// TestPrintHandler_ServiceError_MapsKindToStatus's identical table
		// for the full rationale.
		wantDetailed bool
	}{
		{"validation", apperr.KindValidation, http.StatusBadRequest, true},
		{"not found", apperr.KindNotFound, http.StatusNotFound, true},
		{"unauthorized", apperr.KindUnauthorized, http.StatusUnauthorized, true},
		{"transient", apperr.KindTransient, http.StatusServiceUnavailable, false},
		{"permanent", apperr.KindPermanent, http.StatusInternalServerError, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakePreviewService{err: apperr.Wrap(tt.kind, "app.Service.Preview", errors.New("boom"))}
			h := api.NewPreviewHandler(svc)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/preview", bytes.NewReader(validPreviewRequestBody()))
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}

			var body struct {
				Error string `json:"error"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode response body: %v", err)
			}

			if tt.wantDetailed {
				if !strings.Contains(body.Error, "boom") {
					t.Errorf("error = %q, want it to contain the underlying error detail %q", body.Error, "boom")
				}
			} else {
				if body.Error != "internal server error" {
					t.Errorf("error = %q, want the generic message %q", body.Error, "internal server error")
				}
				if strings.Contains(body.Error, "boom") || strings.Contains(body.Error, "app.Service.Preview") {
					t.Errorf("error = %q, must not leak the underlying error or Op for a 5xx response", body.Error)
				}
			}
		})
	}
}

// TestPreviewHandler_ServiceError_NonAppErrError_SanitizedByStatusNotKind
// proves the sanitisation policy keys off the response's HTTP status, not
// whether the error even has an apperr.Kind — see
// TestPrintHandler_ServiceError_NonAppErrError_SanitizedByStatusNotKind's
// identical rationale.
func TestPreviewHandler_ServiceError_NonAppErrError_SanitizedByStatusNotKind(t *testing.T) {
	svc := &fakePreviewService{err: errors.New("boom: /var/lib/receiptd/assets permission denied")}
	h := api.NewPreviewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/preview", bytes.NewReader(validPreviewRequestBody()))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.Error != "internal server error" {
		t.Errorf("error = %q, want the generic message %q", body.Error, "internal server error")
	}
	if strings.Contains(body.Error, "boom") || strings.Contains(body.Error, "assets") {
		t.Errorf("error = %q, must not leak the underlying error for a 5xx response even without an apperr.Kind", body.Error)
	}
}

// --- Tests against the real app.Service, proving the handler actually
// renders a PNG via the existing pipeline and never touches the queue. ---

func TestPreviewHandler_RealService_ReturnsDecodablePNG(t *testing.T) {
	store := queue.NewMemoryStore()
	svc := app.New(queue.New(store, &noopProcessor{}))
	svc.Profiles = map[string]printer.Profile{"front-desk": {}}
	h := api.NewPreviewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/preview", bytes.NewReader(validPreviewRequestBody()))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if _, err := png.Decode(rec.Body); err != nil {
		t.Fatalf("png.Decode() error = %v, want a valid PNG", err)
	}
}

func TestPreviewHandler_RealService_NeverEnqueuesOrProcesses(t *testing.T) {
	store := queue.NewMemoryStore()
	proc := &noopProcessor{}
	svc := app.New(queue.New(store, proc))
	svc.Profiles = map[string]printer.Profile{"front-desk": {}}
	h := api.NewPreviewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/preview", bytes.NewReader(validPreviewRequestBody()))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if proc.calls != 0 {
		t.Errorf("processor.calls = %d, want 0 (preview must not process)", proc.calls)
	}

	jobs, err := store.List(context.Background(), queue.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v, want nil", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(store.List()) = %d, want 0 (preview must not enqueue a Job)", len(jobs))
	}
}

// TestPreviewHandler_RealService_MissingAsset_ReturnsNotFoundWithoutLeakingPath
// runs a Receipt referencing an asset that was never Put through the real
// layout.Build -> assets.FilesystemStore.Get path, proving the resulting
// 404's body carries an actionable "not found" message but never the
// store's filesystem root — the exact detail assets.Store.Get used to
// leak via the raw *fs.PathError os.ReadFile returns.
func TestPreviewHandler_RealService_MissingAsset_ReturnsNotFoundWithoutLeakingPath(t *testing.T) {
	store := queue.NewMemoryStore()
	svc := app.New(queue.New(store, &noopProcessor{}))
	svc.Profiles = map[string]printer.Profile{"front-desk": {}}
	root := t.TempDir()
	svc.Assets = assets.NewFilesystemStore(root)
	h := api.NewPreviewHandler(svc)

	body := []byte(`{"printer":"front-desk","receipt":{"version":1,"elements":[{"type":"asset","name":"missing-logo.png"}]}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/preview", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var respBody struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if respBody.Error == "" {
		t.Error("error is empty, want an actionable not-found message")
	}
	if strings.Contains(respBody.Error, root) {
		t.Errorf("error = %q, must not leak the assets store's filesystem root %q", respBody.Error, root)
	}
}

func TestPreviewHandler_RealService_InvalidReceipt_ReturnsBadRequest(t *testing.T) {
	store := queue.NewMemoryStore()
	svc := app.New(queue.New(store, &noopProcessor{}))
	h := api.NewPreviewHandler(svc)

	body := []byte(`{"printer":"front-desk","receipt":{"version":1,"elements":[{"type":"text","content":""}]}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/preview", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
