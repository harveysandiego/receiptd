package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/api"
	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// fakePrintService is a test double for the interface PrintHandler needs
// from app.Service, letting most handler tests run without a real Queue.
type fakePrintService struct {
	jobID string
	err   error

	calls      int
	gotReceipt receipt.Receipt
	gotPrinter string
}

func (f *fakePrintService) Print(_ context.Context, r receipt.Receipt, printerName string) (string, error) {
	f.calls++
	f.gotReceipt = r
	f.gotPrinter = printerName
	return f.jobID, f.err
}

// noopProcessor is a queue.Processor test double for the real-Service
// tests below, which must observe that handling a request never
// processes the resulting Job.
type noopProcessor struct {
	calls int
}

func (p *noopProcessor) Process(_ context.Context, _ *queue.Job) error {
	p.calls++
	return nil
}

func validPrintRequestBody() []byte {
	return []byte(`{"printer":"front-desk","receipt":{"version":1,"elements":[{"type":"text","content":"hello"}]}}`)
}

func TestPrintHandler_Success_ReturnsAcceptedWithJobID(t *testing.T) {
	svc := &fakePrintService{jobID: "abc123"}
	h := api.NewPrintHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/print", bytes.NewReader(validPrintRequestBody()))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	var body struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.JobID != "abc123" {
		t.Errorf("job_id = %q, want %q", body.JobID, "abc123")
	}
}

func TestPrintHandler_Success_PassesPrinterAndReceiptToService(t *testing.T) {
	svc := &fakePrintService{jobID: "abc123"}
	h := api.NewPrintHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/print", bytes.NewReader(validPrintRequestBody()))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if svc.calls != 1 {
		t.Fatalf("Service.Print called %d times, want 1", svc.calls)
	}
	if svc.gotPrinter != "front-desk" {
		t.Errorf("printerName = %q, want %q", svc.gotPrinter, "front-desk")
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
}

func TestPrintHandler_MalformedJSON_ReturnsBadRequest(t *testing.T) {
	svc := &fakePrintService{}
	h := api.NewPrintHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/print", bytes.NewReader([]byte(`{"printer":`)))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if svc.calls != 0 {
		t.Errorf("Service.Print called %d times, want 0 for malformed JSON", svc.calls)
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

func TestPrintHandler_MalformedReceiptElement_ReturnsBadRequest(t *testing.T) {
	svc := &fakePrintService{}
	h := api.NewPrintHandler(svc)

	// Syntactically valid JSON, but "content" doesn't fit Text.Content (a
	// string) — mirrors receipt.TestReceipt_UnmarshalJSON_MalformedElement.
	body := []byte(`{"printer":"front-desk","receipt":{"version":1,"elements":[{"type":"text","content":123}]}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/print", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if svc.calls != 0 {
		t.Errorf("Service.Print called %d times, want 0 for malformed element", svc.calls)
	}
}

func TestPrintHandler_BodyTooLarge_ReturnsRequestEntityTooLarge(t *testing.T) {
	svc := &fakePrintService{}
	h := api.NewPrintHandler(svc)

	// A large, unterminated JSON string literal: syntactically valid so far
	// as the decoder can tell, so it keeps reading content bytes (rather
	// than failing fast on a syntax error) until MaxBytesReader cuts it off.
	body := append([]byte(`{"printer":"front-desk","receipt":{"elements":[{"type":"text","content":"`), bytes.Repeat([]byte("a"), 10<<20+1)...)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/print", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
	if svc.calls != 0 {
		t.Errorf("Service.Print called %d times, want 0 for an oversized body", svc.calls)
	}
}

func TestPrintHandler_ServiceError_MapsKindToStatus(t *testing.T) {
	tests := []struct {
		name string
		kind apperr.Kind
		want int
		// wantDetailed is true for a 4xx status, where the response body
		// must still carry the underlying error detail (actionable for the
		// caller), and false for a 5xx status, where it must instead be the
		// fixed generic message with no trace of "boom" or the Op the fake
		// Service wraps it with — see the goal this test was added for:
		// the API boundary must never leak implementation detail in a
		// server-side failure.
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
			svc := &fakePrintService{err: apperr.Wrap(tt.kind, "app.Service.Print", errors.New("boom"))}
			h := api.NewPrintHandler(svc)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/print", bytes.NewReader(validPrintRequestBody()))
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
				if strings.Contains(body.Error, "boom") || strings.Contains(body.Error, "app.Service.Print") {
					t.Errorf("error = %q, must not leak the underlying error or Op for a 5xx response", body.Error)
				}
			}
		})
	}
}

// TestPrintHandler_ServiceError_NonAppErrError_SanitizedByStatusNotKind
// proves the sanitisation policy keys off the response's HTTP status, not
// whether the error even has an apperr.Kind: a plain error (one
// statusForError can't classify, so it falls back to
// http.StatusInternalServerError) must be sanitized exactly like a
// classified apperr.KindPermanent error.
func TestPrintHandler_ServiceError_NonAppErrError_SanitizedByStatusNotKind(t *testing.T) {
	svc := &fakePrintService{err: errors.New("boom: /var/lib/receiptd/queue.db permission denied")}
	h := api.NewPrintHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/print", bytes.NewReader(validPrintRequestBody()))
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
	if strings.Contains(body.Error, "boom") || strings.Contains(body.Error, "queue.db") {
		t.Errorf("error = %q, must not leak the underlying error for a 5xx response even without an apperr.Kind", body.Error)
	}
}

// --- Tests against the real app.Service, proving the handler actually
// results in a queued Job and never triggers processing — not just that
// it calls an interface method correctly. ---

func TestPrintHandler_RealService_EnqueuesJob(t *testing.T) {
	store := queue.NewMemoryStore()
	svc := app.New(queue.New(store, &noopProcessor{}))
	h := api.NewPrintHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/print", bytes.NewReader(validPrintRequestBody()))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	var body struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	job, err := store.Get(context.Background(), body.JobID)
	if err != nil {
		t.Fatalf("store.Get(%q) error = %v, want nil", body.JobID, err)
	}
	if job.State != queue.JobPending {
		t.Errorf("job.State = %v, want %v", job.State, queue.JobPending)
	}
	if job.PrinterName != "front-desk" {
		t.Errorf("job.PrinterName = %q, want %q", job.PrinterName, "front-desk")
	}
}

func TestPrintHandler_RealService_NeverInvokesProcessor(t *testing.T) {
	store := queue.NewMemoryStore()
	proc := &noopProcessor{}
	svc := app.New(queue.New(store, proc))
	h := api.NewPrintHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/print", bytes.NewReader(validPrintRequestBody()))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if proc.calls != 0 {
		t.Errorf("processor.calls = %d, want 0 (request handling must never process)", proc.calls)
	}
}

func TestPrintHandler_RealService_InvalidReceipt_ReturnsBadRequestWithoutEnqueuing(t *testing.T) {
	store := queue.NewMemoryStore()
	svc := app.New(queue.New(store, &noopProcessor{}))
	h := api.NewPrintHandler(svc)

	body := []byte(`{"printer":"front-desk","receipt":{"version":1,"elements":[{"type":"text","content":""}]}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/print", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	jobs, err := store.List(context.Background(), queue.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v, want nil", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(store.List()) = %d, want 0 (invalid Receipt must not be enqueued)", len(jobs))
	}
}
