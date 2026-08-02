package webui

import (
	"html/template"
	"log"
	"net/http"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/web"
)

// printersTemplate clones baseTemplate and layers printers.tmpl's
// "content" override on top of the clone (see render.go and
// dashboard.go's dashboardTemplate for why a clone, not a shared parse).
var printersTemplate = template.Must(template.Must(baseTemplate.Clone()).ParseFS(web.FS, "templates/printers.tmpl"))

// printerRow is one configured printer's presentation row. Fields are
// copied from app.PrinterSummary rather than embedding it, mirroring
// dashboardPage's flattening (dashboard.go): the row's shape is this
// page's own to define. Cut, PartialCut, and Status are pre-rendered
// strings so printers.tmpl stays pure interpolation rather than deciding
// how to phrase a bool or a status.
type printerRow struct {
	Name      string
	Transport string
	Address   string

	WidthDots int
	DPI       int

	Cut        string
	PartialCut string
	Status     string
	// StatusClass is the Status badge's CSS modifier ("online"/"offline").
	StatusClass string
}

// printersPage is the Printers page's model — the only data its template
// sees.
type printersPage struct {
	Printers []printerRow
}

// PrintersHandler serves GET /printers, the read-only printer settings
// screen (docs/adr/0024-printer-settings-screen-is-read-only.md).
type PrintersHandler struct {
	Service *app.Service
}

// NewPrintersHandler returns a PrintersHandler backed by svc.
func NewPrintersHandler(svc *app.Service) *PrintersHandler {
	return &PrintersHandler{Service: svc}
}

func (h *PrintersHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.Service.ListPrinters(r.Context())
	if err != nil {
		renderError(w, "Printers", err)
		return
	}

	rows := make([]printerRow, len(summaries))
	for i, s := range summaries {
		// StatusDetail is a raw transport-level error (a dial failure's
		// address/port/message) — logged here for an administrator, but
		// never rendered to the browser; see printerStatusText.
		if !s.Online && s.StatusDetail != "" {
			log.Printf("webui: printer %q offline: %s", s.Name, s.StatusDetail)
		}

		rows[i] = printerRow{
			Name:        s.Name,
			Transport:   s.Transport,
			Address:     s.Address,
			WidthDots:   s.WidthDots,
			DPI:         s.DPI,
			Cut:         capabilityText(s.SupportsCut),
			PartialCut:  capabilityText(s.SupportsPartialCut),
			Status:      printerStatusText(s.Online),
			StatusClass: printerStatusClass(s.Online),
		}
	}

	render(w, printersTemplate, http.StatusOK, printersPage{
		Printers: rows,
	})
}

// capabilityText renders one of PrinterSummary's SupportsCut/
// SupportsPartialCut flags as the table's Yes/No text.
func capabilityText(supported bool) string {
	if supported {
		return "Yes"
	}
	return "No"
}

// printerStatusText turns a printer's live reachability into the table's
// Status cell text. It deliberately never includes PrinterSummary's raw
// StatusDetail (a dial/transport error that can carry an OS-level error
// string) — unlike this package's earlier stance, that detail is now
// treated the same trust-boundary way a Job's LastError already is
// (internal/api/job_status.go): logged server-side for an administrator
// (ServeHTTP, above), never echoed to the browser. This isn't a retreat
// to a vague "Unknown error": Online/Offline is itself the operationally
// meaningful signal this table exists to show, and the printer's address
// — the one other fact StatusDetail could add — is already its own
// column (printerRow.Address), not hidden by this change.
func printerStatusText(online bool) string {
	if online {
		return "Online"
	}
	return "Offline"
}

// printerStatusClass returns printerRow.StatusClass for online.
func printerStatusClass(online bool) string {
	if online {
		return "online"
	}
	return "offline"
}
