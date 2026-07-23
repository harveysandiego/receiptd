package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

// printService is the subset of app.Service that PrintHandler needs.
// *app.Service satisfies it structurally; tests supply a fake instead of
// wiring a real Queue. See docs/ARCHITECTURE.md §9 ("api: httptest
// against a fake app.Service").
type printService interface {
	Print(ctx context.Context, r receipt.Receipt, printerName, idempotencyKey string) (jobID string, err error)
}

// printRequest is the wire shape of a POST /api/v1/print request body.
type printRequest struct {
	Printer string          `json:"printer"`
	Receipt receipt.Receipt `json:"receipt"`
}

// printResponse is the wire shape of a successful POST /api/v1/print
// response body.
type printResponse struct {
	JobID string `json:"job_id"`
}

// PrintHandler adapts POST /api/v1/print onto printService.Print: decode
// the request, call Print, encode the result. It holds no business logic
// of its own — Receipt validation and enqueueing both happen in Service.
type PrintHandler struct {
	Service printService
}

// NewPrintHandler returns a PrintHandler backed by svc.
func NewPrintHandler(svc printService) *PrintHandler {
	return &PrintHandler{Service: svc}
}

func (h *PrintHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	var req printRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if isBodyTooLarge(err) {
			writeError(w, http.StatusRequestEntityTooLarge, err)
			return
		}
		writeError(w, statusForError(err, http.StatusBadRequest), err)
		return
	}

	// Idempotency-Key is the now-common convention this project isn't
	// inventing from scratch (popularized by Stripe's API) — optional, so
	// an absent header (idempotencyKey == "") is passed straight through
	// as "" and gets exactly today's always-create behavior from
	// Service.Print. See docs/adr/0020-idempotent-print-requests.md.
	idempotencyKey := r.Header.Get("Idempotency-Key")

	jobID, err := h.Service.Print(r.Context(), req.Receipt, req.Printer, idempotencyKey)
	if err != nil {
		writeError(w, statusForError(err, http.StatusInternalServerError), err)
		return
	}

	writeJSON(w, http.StatusAccepted, printResponse{JobID: jobID})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// internalServerErrorMessage is the fixed body every 5xx response uses in
// place of err.Error(). The API is the trust boundary (docs/ARCHITECTURE.md
// §5): a 4xx means the request itself was the problem, so err.Error() is
// actionable detail worth returning, but a 5xx means the server failed and
// err.Error() may leak a filesystem/database path, network error, or other
// implementation detail no client needs. The real err is logged
// server-side by writeError before being discarded.
const internalServerErrorMessage = "internal server error"

func writeError(w http.ResponseWriter, status int, err error) {
	message := err.Error()
	if status >= http.StatusInternalServerError {
		log.Printf("api: internal error: %v", err)
		message = internalServerErrorMessage
	}
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}
