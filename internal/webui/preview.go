package webui

import (
	"net/http"

	"github.com/harveysandiego/receiptd/internal/app"
)

// PreviewHandler serves the receipt preview pages: the form (GET
// /preview) and the rendered preview (POST /preview). Stub until the
// Preview slice replaces each method's body.
type PreviewHandler struct {
	Service *app.Service
}

// NewPreviewHandler returns a PreviewHandler backed by svc.
func NewPreviewHandler(svc *app.Service) *PreviewHandler {
	return &PreviewHandler{Service: svc}
}

// Show serves GET /preview.
func (h *PreviewHandler) Show(w http.ResponseWriter, _ *http.Request) {
	renderStub(w, "Preview")
}

// Generate serves POST /preview.
func (h *PreviewHandler) Generate(w http.ResponseWriter, _ *http.Request) {
	renderStub(w, "Preview")
}
