package webui

import (
	"net/http"

	"github.com/harveysandiego/receiptd/internal/app"
)

// StatusHandler serves GET /status, the printer/job status view polled
// by the dashboard's client-side JavaScript
// (docs/adr/0025-dashboard-updates-via-polling.md). Stub until the
// Dashboard slice replaces ServeHTTP's body.
type StatusHandler struct {
	Service *app.Service
}

// NewStatusHandler returns a StatusHandler backed by svc.
func NewStatusHandler(svc *app.Service) *StatusHandler {
	return &StatusHandler{Service: svc}
}

func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	renderStub(w, "Status")
}
