package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestPrintHandler_ServiceError_MapsKindToStatus(t *testing.T) {
	tests := []struct {
		name string
		kind apperr.Kind
		want int
	}{
		{"validation", apperr.KindValidation, http.StatusBadRequest},
		{"not found", apperr.KindNotFound, http.StatusNotFound},
		{"unauthorized", apperr.KindUnauthorized, http.StatusUnauthorized},
		{"transient", apperr.KindTransient, http.StatusServiceUnavailable},
		{"permanent", apperr.KindPermanent, http.StatusInternalServerError},
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
		})
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
