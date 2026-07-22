package api_test

import (
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
		LastError:   "boom",
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
	if body.LastError != job.LastError {
		t.Errorf("last_error = %q, want %q", body.LastError, job.LastError)
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

	jobID, err := svc.Print(ctx, validReceipt(), "front-desk")
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

	jobID, err := svc.Print(ctx, validReceipt(), "front-desk")
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

	jobID, err := svc.Print(ctx, validReceipt(), "front-desk")
	if err != nil {
		t.Fatalf("Print() error = %v, want nil", err)
	}

	h.ServeHTTP(httptest.NewRecorder(), newJobStatusRequest(jobID))

	if proc.calls != 0 {
		t.Errorf("processor.calls = %d, want 0 (JobStatus must never invoke the Processor)", proc.calls)
	}
}
