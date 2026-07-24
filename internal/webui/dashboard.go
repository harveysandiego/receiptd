package webui

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/web"
)

// dashboardTemplate clones baseTemplate and layers dashboard.tmpl's
// "content" override on top of the clone (see render.go) rather than
// parsing it into baseTemplate directly, so the stub pages' default
// content block is unaffected.
var dashboardTemplate = template.Must(template.Must(baseTemplate.Clone()).ParseFS(web.FS, "templates/dashboard.tmpl"))

// dashboardPage is the Dashboard's page model — the only data its
// template sees. Every field is computed by DashboardHandler; the
// template only interpolates them. Fields are flattened primitives rather
// than an embedded app.QueueSummary: the page model's shape is Dashboard's
// own to define, so a future field added to QueueSummary for some other
// caller's benefit doesn't silently appear on this page without a
// deliberate choice to add it here too.
type dashboardPage struct {
	Title string

	PrinterCount   int
	PrintersOnline int
	StatusMessage  string

	AssetCount int

	QueuePending   int
	QueueRunning   int
	QueueDone      int
	QueueFailed    int
	QueueCancelled int
	QueueTotal     int
}

// DashboardHandler serves GET / — the operator's landing-page overview
// (docs/ARCHITECTURE.md §10): configured printer/asset counts and the
// current queue summary, read from the application service seam. It is
// read-only — no forms, no client-side refresh
// (docs/adr/0025-dashboard-updates-via-polling.md's polling loop is a
// later slice's concern, not this one's).
type DashboardHandler struct {
	Service *app.Service
}

// NewDashboardHandler returns a DashboardHandler backed by svc.
func NewDashboardHandler(svc *app.Service) *DashboardHandler {
	return &DashboardHandler{Service: svc}
}

func (h *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	printers, err := h.Service.ListPrinters(ctx)
	if err != nil {
		renderError(w, "Dashboard", err)
		return
	}
	assetList, err := h.Service.ListAssets(ctx)
	if err != nil {
		renderError(w, "Dashboard", err)
		return
	}
	queueSummary, err := h.Service.QueueSummary(ctx)
	if err != nil {
		renderError(w, "Dashboard", err)
		return
	}

	online := 0
	for _, p := range printers {
		if p.Online {
			online++
		}
	}

	render(w, dashboardTemplate, http.StatusOK, dashboardPage{
		Title:          "Dashboard",
		PrinterCount:   len(printers),
		PrintersOnline: online,
		StatusMessage:  printerStatusMessage(online, len(printers)),
		AssetCount:     len(assetList),
		QueuePending:   queueSummary.Pending,
		QueueRunning:   queueSummary.Running,
		QueueDone:      queueSummary.Done,
		QueueFailed:    queueSummary.Failed,
		QueueCancelled: queueSummary.Cancelled,
		QueueTotal:     queueSummary.Total(),
	})
}

// printerStatusMessage turns online/total counts into the dashboard's
// "basic service status" line — the one piece of dashboard content that's
// a judgment call rather than a direct count, so it's decided once here
// rather than left to the template. This is presentation wording, not
// application behavior: it takes plain ints rather than calling Service,
// and stays private to Dashboard's own page model rather than becoming an
// app.Service method or a helper shared across pages — a future page
// showing the same counts is free to phrase them differently.
func printerStatusMessage(online, total int) string {
	switch {
	case total == 0:
		return "No printers configured"
	case online == total:
		return "All printers online"
	default:
		return fmt.Sprintf("%d of %d printers offline", total-online, total)
	}
}
