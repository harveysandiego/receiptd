package webui

import (
	"net/http"

	"github.com/harveysandiego/receiptd/internal/app"
)

// PrintHandler serves the text-printing pages: the form (GET /print) and
// the submit action (POST /print). Stub until the Print slice replaces
// each method's body.
type PrintHandler struct {
	Service *app.Service
}

// NewPrintHandler returns a PrintHandler backed by svc.
func NewPrintHandler(svc *app.Service) *PrintHandler {
	return &PrintHandler{Service: svc}
}

// Show serves GET /print.
func (h *PrintHandler) Show(w http.ResponseWriter, _ *http.Request) {
	renderStub(w, "Print")
}

// Submit serves POST /print.
func (h *PrintHandler) Submit(w http.ResponseWriter, _ *http.Request) {
	renderStub(w, "Print")
}
