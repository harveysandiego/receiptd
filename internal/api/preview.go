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
	Preview(ctx context.Context, r receipt.Receipt) ([]byte, error)
}

// PreviewHandler adapts POST /api/v1/preview onto previewService.Preview:
// decode the request body as a Receipt, call Preview, write back the PNG.
// It holds no business logic of its own — Receipt validation and
// rendering both happen in Service. Unlike PrintHandler's request body,
// there is no wrapping envelope: Preview takes only a Receipt, so the
// request body is the Receipt's JSON directly.
type PreviewHandler struct {
	Service previewService
}

// NewPreviewHandler returns a PreviewHandler backed by svc.
func NewPreviewHandler(svc previewService) *PreviewHandler {
	return &PreviewHandler{Service: svc}
}

func (h *PreviewHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var rcpt receipt.Receipt
	if err := json.NewDecoder(r.Body).Decode(&rcpt); err != nil {
		writeError(w, statusForError(err, http.StatusBadRequest), err)
		return
	}

	pngBytes, err := h.Service.Preview(r.Context(), rcpt)
	if err != nil {
		writeError(w, statusForError(err, http.StatusInternalServerError), err)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pngBytes)
}
