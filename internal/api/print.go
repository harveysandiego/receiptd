package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

// printService is the subset of app.Service that PrintHandler needs.
// *app.Service satisfies it structurally; tests supply a fake instead of
// wiring a real Queue. See docs/ARCHITECTURE.md §9 ("api: httptest
// against a fake app.Service").
type printService interface {
	Print(ctx context.Context, r receipt.Receipt, printerName string) (jobID string, err error)
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
	var req printRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, statusForError(err, http.StatusBadRequest), err)
		return
	}

	jobID, err := h.Service.Print(r.Context(), req.Receipt, req.Printer)
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

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: err.Error()})
}
