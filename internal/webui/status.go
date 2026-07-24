package webui

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/harveysandiego/receiptd/internal/app"
)

// statusResponse is GET /status's JSON body — the subset of dashboardPage's
// fields that can change from outside this browser tab (printer
// reachability, queue state), polled on a timer by dashboard.js
// (docs/adr/0025-dashboard-updates-via-polling.md) to refresh the
// dashboard's Printers/Queue cards without a full page reload. AssetCount
// isn't included: it only changes through this same UI's own
// upload/delete actions, which already redirect to a fresh page load.
type statusResponse struct {
	PrinterCount   int    `json:"printer_count"`
	PrintersOnline int    `json:"printers_online"`
	StatusMessage  string `json:"status_message"`

	QueuePending   int `json:"queue_pending"`
	QueueRunning   int `json:"queue_running"`
	QueueDone      int `json:"queue_done"`
	QueueFailed    int `json:"queue_failed"`
	QueueCancelled int `json:"queue_cancelled"`
	QueueTotal     int `json:"queue_total"`
}

// StatusHandler serves GET /status — dashboard.js's poll target
// (docs/adr/0025-dashboard-updates-via-polling.md), not a general-purpose
// status API; extending its response for some other consumer should be
// weighed against that scope rather than assumed. It reads through the
// same Service.ListPrinters/Service.QueueSummary calls DashboardHandler
// itself makes, so there is no second code path, and — per
// docs/adr/0022-webui-server-rendered-html-template.md's "no JSON
// round-trip through internal/api from the browser's own UI" — this JSON
// is produced directly by webui rather than by calling internal/api.
type StatusHandler struct {
	Service *app.Service
}

// NewStatusHandler returns a StatusHandler backed by svc.
func NewStatusHandler(svc *app.Service) *StatusHandler {
	return &StatusHandler{Service: svc}
}

func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	printers, err := h.Service.ListPrinters(ctx)
	if err != nil {
		writeStatusError(w, err)
		return
	}
	queueSummary, err := h.Service.QueueSummary(ctx)
	if err != nil {
		writeStatusError(w, err)
		return
	}

	online := onlinePrinterCount(printers)

	writeStatusJSON(w, http.StatusOK, statusResponse{
		PrinterCount:   len(printers),
		PrintersOnline: online,
		StatusMessage:  printerStatusMessage(online, len(printers)),
		QueuePending:   queueSummary.Pending,
		QueueRunning:   queueSummary.Running,
		QueueDone:      queueSummary.Done,
		QueueFailed:    queueSummary.Failed,
		QueueCancelled: queueSummary.Cancelled,
		QueueTotal:     queueSummary.Total(),
	})
}

// writeStatusJSON sets Cache-Control: no-store so neither the browser nor
// an intermediary reverse proxy (docs/adr/0021-transport-security-via-reverse-proxy.md)
// ever serves a stale poll response — dashboard.js's whole premise is that
// each poll reflects current state, which a cached response would quietly
// defeat.
func writeStatusJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeStatusError applies the same trust boundary renderError does for
// HTML pages (never echoing err's own message to the browser) to this
// one JSON endpoint — a separate small helper because the encoding
// differs, not a second error philosophy.
func writeStatusError(w http.ResponseWriter, err error) {
	log.Printf("webui: status: %v", err)
	writeStatusJSON(w, statusForErr(err), struct {
		Error string `json:"error"`
	}{Error: "Status could not load right now. Please try again shortly."})
}
