package webui

import (
	"net/http"

	"github.com/harveysandiego/receiptd/internal/app"
)

// DashboardHandler serves GET / — the operator's landing-page overview
// (docs/ARCHITECTURE.md §10). Stub until the Dashboard slice replaces
// ServeHTTP's body.
type DashboardHandler struct {
	Service *app.Service
}

// NewDashboardHandler returns a DashboardHandler backed by svc.
func NewDashboardHandler(svc *app.Service) *DashboardHandler {
	return &DashboardHandler{Service: svc}
}

func (h *DashboardHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	renderStub(w, "Dashboard")
}
