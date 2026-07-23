package api

import (
	"context"
	"net/http"
	"time"

	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// jobStatusService is the subset of app.Service that JobStatusHandler
// needs. *app.Service satisfies it structurally; tests supply a fake
// instead of wiring a real Queue. See docs/ARCHITECTURE.md §9 ("api:
// httptest against a fake app.Service").
type jobStatusService interface {
	JobStatus(ctx context.Context, id string) (*queue.Job, error)
}

// jobStatusResponse is the wire shape of a successful GET
// /api/v1/jobs/{id} response body. It mirrors queue.Job field-for-field
// rather than exposing queue.Job directly, keeping the wire format
// independent of the internal type's naming and free to diverge later.
type jobStatusResponse struct {
	ID          string          `json:"id"`
	PrinterName string          `json:"printer_name"`
	Receipt     receipt.Receipt `json:"receipt"`
	State       string          `json:"state"`
	Attempts    int             `json:"attempts"`
	LastError   string          `json:"last_error"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// JobStatusHandler adapts GET /api/v1/jobs/{id} onto
// jobStatusService.JobStatus. It holds no business logic of its own — an
// unknown id flows through to Service.JobStatus and gets the same
// apperr.KindNotFound treatment as any other.
type JobStatusHandler struct {
	Service jobStatusService
}

// NewJobStatusHandler returns a JobStatusHandler backed by svc.
func NewJobStatusHandler(svc jobStatusService) *JobStatusHandler {
	return &JobStatusHandler{Service: svc}
}

func (h *JobStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	job, err := h.Service.JobStatus(r.Context(), id)
	if err != nil {
		writeError(w, statusForError(err, http.StatusInternalServerError), err)
		return
	}

	writeJSON(w, http.StatusOK, jobStatusResponse{
		ID:          job.ID,
		PrinterName: job.PrinterName,
		Receipt:     job.Receipt,
		State:       string(job.State),
		Attempts:    job.Attempts,
		LastError:   sanitizedLastError(job.LastError),
		CreatedAt:   job.CreatedAt,
		UpdatedAt:   job.UpdatedAt,
	})
}

// jobFailureMessage replaces a failed Job's raw LastError in API
// responses. LastError comes from a background Processor
// (internal/queue/process.go) and may embed a filesystem path, a network
// error, or a printer's hostname/IP — the class of detail this package's
// trust boundary keeps out of client responses (see doc.go). The full
// detail is not lost: ProcessNext logs it and it stays on the stored Job.
const jobFailureMessage = "job processing failed; see server logs for details"

// sanitizedLastError returns raw unchanged if it's empty (a Job that
// hasn't failed has nothing to sanitize), or jobFailureMessage otherwise.
func sanitizedLastError(raw string) string {
	if raw == "" {
		return ""
	}
	return jobFailureMessage
}
