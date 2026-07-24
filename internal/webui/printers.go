package webui

import (
	"html/template"
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
}

// printersPage is the Printers page's model — the only data its template
// sees.
type printersPage struct {
	Title    string
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
		rows[i] = printerRow{
			Name:       s.Name,
			Transport:  s.Transport,
			Address:    s.Address,
			WidthDots:  s.WidthDots,
			DPI:        s.DPI,
			Cut:        capabilityText(s.SupportsCut),
			PartialCut: capabilityText(s.SupportsPartialCut),
			Status:     printerStatusText(s.Online, s.StatusDetail),
		}
	}

	render(w, printersTemplate, http.StatusOK, printersPage{
		Title:    "Printers",
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
// Status cell text. detail is PrinterSummary's own point-in-time
// printer.Printer.Status detail (docs/adr/0024) — a dial/transport error
// about reaching the printer itself, not arbitrary internal state, so
// unlike a Job's LastError (internal/api/job_status.go, which can carry
// receipt content) it's shown verbatim rather than sanitized.
func printerStatusText(online bool, detail string) string {
	if online {
		return "Online"
	}
	if detail == "" {
		return "Offline"
	}
	return "Offline: " + detail
}
