package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/api"
	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

func validReceipt() receipt.Receipt {
	return receipt.Receipt{
		Elements: []receipt.Element{receipt.Text{Content: "hello"}},
	}
}

// fakeJobStatusService is a test double for the interface JobStatusHandler
// needs from app.Service, letting most handler tests run without a real
// Queue.
type fakeJobStatusService struct {
	job *queue.Job
	err error

	calls int
	gotID string
}

func (f *fakeJobStatusService) JobStatus(_ context.Context, id string) (*queue.Job, error) {
	f.calls++
	f.gotID = id
	return f.job, f.err
}

func newJobStatusRequest(id string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+id, nil)
	req.SetPathValue("id", id)
	return req
}

func TestJobStatusHandler_Success_ReturnsJobAsJSON(t *testing.T) {
	job := &queue.Job{
		ID:          "abc123",
		PrinterName: "front-desk",
		Receipt:     validReceipt(),
		State:       queue.JobPending,
		Attempts:    2,
	}
	svc := &fakeJobStatusService{job: job}
	h := api.NewJobStatusHandler(svc)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newJobStatusRequest("abc123"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		ID          string `json:"id"`
		PrinterName string `json:"printer_name"`
		State       string `json:"state"`
		Attempts    int    `json:"attempts"`
		LastError   string `json:"last_error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.ID != job.ID {
		t.Errorf("id = %q, want %q", body.ID, job.ID)
	}
	if body.PrinterName != job.PrinterName {
		t.Errorf("printer_name = %q, want %q", body.PrinterName, job.PrinterName)
	}
	if body.State != string(job.State) {
		t.Errorf("state = %q, want %q", body.State, job.State)
	}
	if body.Attempts != job.Attempts {
		t.Errorf("attempts = %d, want %d", body.Attempts, job.Attempts)
	}
	if body.LastError != "" {
		t.Errorf("last_error = %q, want %q for a Job that hasn't failed", body.LastError, "")
	}
}

// TestJobStatusHandler_Success_FailedJob_SanitizesLastError proves
// LastError is never returned to a client verbatim: a Processor failure
// may embed a filesystem path, a network error, or a printer's
// hostname/IP (see internal/queue/process.go's ProcessNext, which is the
// only thing that ever sets LastError), none of which this package's
// trust boundary allows across in a client response — see doc.go and
// TestJobStatusHandler_RealService_TransientPrinterFailure_DoesNotLeakAddress
// for the same policy proven against a real Processor failure.
func TestJobStatusHandler_Success_FailedJob_SanitizesLastError(t *testing.T) {
	job := &queue.Job{
		ID:        "abc123",
		State:     queue.JobFailed,
		LastError: "dial tcp 192.168.1.50:9100: connect: connection refused",
	}
	svc := &fakeJobStatusService{job: job}
	h := api.NewJobStatusHandler(svc)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newJobStatusRequest("abc123"))

	var body struct {
		LastError string `json:"last_error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.LastError == "" {
		t.Error("last_error is empty, want a fixed non-empty message for a failed Job")
	}
	if strings.Contains(body.LastError, "192.168.1.50") {
		t.Errorf("last_error = %q, must not leak the printer's address", body.LastError)
	}
}

func TestJobStatusHandler_Success_PassesIDToService(t *testing.T) {
	svc := &fakeJobStatusService{job: &queue.Job{ID: "abc123"}}
	h := api.NewJobStatusHandler(svc)

	h.ServeHTTP(httptest.NewRecorder(), newJobStatusRequest("abc123"))

	if svc.calls != 1 {
		t.Fatalf("Service.JobStatus called %d times, want 1", svc.calls)
	}
	if svc.gotID != "abc123" {
		t.Errorf("id = %q, want %q", svc.gotID, "abc123")
	}
}

func TestJobStatusHandler_ServiceError_MapsKindToStatus(t *testing.T) {
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
			svc := &fakeJobStatusService{err: apperr.Wrap(tt.kind, "app.Service.JobStatus", errors.New("boom"))}
			h := api.NewJobStatusHandler(svc)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, newJobStatusRequest("abc123"))

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
				if strings.Contains(body.Error, "boom") || strings.Contains(body.Error, "app.Service.JobStatus") {
					t.Errorf("error = %q, must not leak the underlying error or Op for a 5xx response", body.Error)
				}
			}
		})
	}
}

// TestJobStatusHandler_ServiceError_NonAppErrError_SanitizedByStatusNotKind
// proves the sanitisation policy keys off the response's HTTP status, not
// whether the error even has an apperr.Kind — see
// TestPrintHandler_ServiceError_NonAppErrError_SanitizedByStatusNotKind's
// identical rationale.
func TestJobStatusHandler_ServiceError_NonAppErrError_SanitizedByStatusNotKind(t *testing.T) {
	svc := &fakeJobStatusService{err: errors.New("boom: /var/lib/receiptd/queue.db permission denied")}
	h := api.NewJobStatusHandler(svc)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newJobStatusRequest("abc123"))

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
// reflects real Queue state and never mutates or processes it. ---

func TestJobStatusHandler_RealService_UnknownID_ReturnsNotFound(t *testing.T) {
	store := queue.NewMemoryStore()
	svc := app.New(queue.New(store, &noopProcessor{}))
	h := api.NewJobStatusHandler(svc)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newJobStatusRequest("does-not-exist"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestJobStatusHandler_RealService_MissingID_ReturnsNotFound(t *testing.T) {
	store := queue.NewMemoryStore()
	svc := app.New(queue.New(store, &noopProcessor{}))
	h := api.NewJobStatusHandler(svc)

	// No path value set at all — mirrors what r.PathValue("id") returns
	// when a request reaches the handler without the "id" wildcard having
	// matched anything. JobStatusHandler adds no special-casing for this;
	// it flows through to Service.JobStatus exactly like any other unknown
	// ID and gets the same apperr.KindNotFound treatment.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestJobStatusHandler_RealService_ExistingJob_ReflectsQueueState(t *testing.T) {
	store := queue.NewMemoryStore()
	svc := app.New(queue.New(store, &noopProcessor{}))
	h := api.NewJobStatusHandler(svc)
	ctx := context.Background()

	jobID, err := svc.Print(ctx, validReceipt(), "front-desk", "")
	if err != nil {
		t.Fatalf("Print() error = %v, want nil", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newJobStatusRequest(jobID))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.ID != jobID {
		t.Errorf("id = %q, want %q", body.ID, jobID)
	}
	if body.State != string(queue.JobPending) {
		t.Errorf("state = %q, want %q", body.State, queue.JobPending)
	}
}

func TestJobStatusHandler_RealService_DoesNotModifyQueueState(t *testing.T) {
	store := queue.NewMemoryStore()
	svc := app.New(queue.New(store, &noopProcessor{}))
	h := api.NewJobStatusHandler(svc)
	ctx := context.Background()

	jobID, err := svc.Print(ctx, validReceipt(), "front-desk", "")
	if err != nil {
		t.Fatalf("Print() error = %v, want nil", err)
	}
	before, err := store.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("store.Get() error = %v, want nil", err)
	}

	h.ServeHTTP(httptest.NewRecorder(), newJobStatusRequest(jobID))

	after, err := store.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("store.Get() error = %v, want nil", err)
	}
	if after.State != before.State || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("handler changed stored Job: before = %+v, after = %+v", *before, *after)
	}
}

func TestJobStatusHandler_RealService_NeverInvokesProcessor(t *testing.T) {
	store := queue.NewMemoryStore()
	proc := &noopProcessor{}
	svc := app.New(queue.New(store, proc))
	h := api.NewJobStatusHandler(svc)
	ctx := context.Background()

	jobID, err := svc.Print(ctx, validReceipt(), "front-desk", "")
	if err != nil {
		t.Fatalf("Print() error = %v, want nil", err)
	}

	h.ServeHTTP(httptest.NewRecorder(), newJobStatusRequest(jobID))

	if proc.calls != 0 {
		t.Errorf("processor.calls = %d, want 0 (JobStatus must never invoke the Processor)", proc.calls)
	}
}

// TestJobStatusHandler_RealService_TransientPrinterFailure_DoesNotLeakAddress
// runs a Job through the real Print -> ProcessNext -> printer.Send
// pipeline against a printer.NewNetworkPrinter that cannot be reached
// (the same net.Listen-then-Close technique
// internal/printer/network_test.go uses for a deterministic connection
// failure), proving the resulting JobFailed Job's LastError — which
// printer.Send populates with the dial error, including the printer's own
// address — never reaches an API client verbatim.
func TestJobStatusHandler_RealService_TransientPrinterFailure_DoesNotLeakAddress(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // nothing listening on addr now

	svc := &app.Service{}
	svc.Printers = map[string]printer.Printer{
		"front-desk": printer.NewNetworkPrinter(printer.Connection{Transport: "network", Address: addr}),
	}
	svc.Profiles = map[string]printer.Profile{"front-desk": {}}
	// maxAttempts=1 fails on the first attempt with no retry backoff to
	// wait out, keeping the test fast and deterministic.
	svc.Queue = queue.NewWithRetry(queue.NewMemoryStore(), svc, 1, time.Millisecond)

	ctx := context.Background()
	jobID, err := svc.Print(ctx, validReceipt(), "front-desk", "")
	if err != nil {
		t.Fatalf("Print() error = %v, want nil", err)
	}
	if err := svc.Queue.ProcessNext(ctx); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}

	h := api.NewJobStatusHandler(svc)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newJobStatusRequest(jobID))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		State     string `json:"state"`
		LastError string `json:"last_error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.State != string(queue.JobFailed) {
		t.Fatalf("state = %q, want %q (Process must have failed against an unreachable printer)", body.State, queue.JobFailed)
	}
	if body.LastError == "" {
		t.Error("last_error is empty, want a fixed non-empty message for a failed Job")
	}
	if strings.Contains(body.LastError, addr) {
		t.Errorf("last_error = %q, must not leak the printer's address %q", body.LastError, addr)
	}
}
