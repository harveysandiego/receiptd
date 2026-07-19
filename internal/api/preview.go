package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

// previewService is the subset of app.Service that PreviewHandler needs.
// *app.Service satisfies it structurally; tests supply a fake instead of
// wiring a real Queue. See docs/ARCHITECTURE.md §9 ("api: httptest
// against a fake app.Service").
type previewService interface {
	Preview(ctx context.Context, r receipt.Receipt, printerName string) ([]byte, error)
}

// previewRequest is the wire shape of a POST /api/v1/preview request
// body, mirroring PrintHandler's printRequest — see
// docs/adr/0006-preview-requires-printer-profile.md for why Preview needs
// a target printer at all.
type previewRequest struct {
	Printer string          `json:"printer"`
	Receipt receipt.Receipt `json:"receipt"`
}

// PreviewHandler adapts POST /api/v1/preview onto previewService.Preview:
// decode the request body, call Preview, write back the PNG. It holds no
// business logic of its own — Receipt validation, printer resolution, and
// rendering all happen in Service.
type PreviewHandler struct {
	Service previewService
}

// NewPreviewHandler returns a PreviewHandler backed by svc.
func NewPreviewHandler(svc previewService) *PreviewHandler {
	return &PreviewHandler{Service: svc}
}

func (h *PreviewHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	var req previewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if isBodyTooLarge(err) {
			writeError(w, http.StatusRequestEntityTooLarge, err)
			return
		}
		writeError(w, statusForError(err, http.StatusBadRequest), err)
		return
	}

	pngBytes, err := h.Service.Preview(r.Context(), req.Receipt, req.Printer)
	if err != nil {
		writeError(w, statusForError(err, http.StatusInternalServerError), err)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pngBytes)
}
