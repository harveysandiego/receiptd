package webui

import (
	"net/http"

	"github.com/harveysandiego/receiptd/internal/app"
)

// PrintersHandler serves GET /printers, the read-only printer settings
// screen (docs/adr/0024-printer-settings-screen-is-read-only.md). Stub
// until the Printers slice replaces ServeHTTP's body.
type PrintersHandler struct {
	Service *app.Service
}

// NewPrintersHandler returns a PrintersHandler backed by svc.
func NewPrintersHandler(svc *app.Service) *PrintersHandler {
	return &PrintersHandler{Service: svc}
}

func (h *PrintersHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	renderStub(w, "Printers")
}
