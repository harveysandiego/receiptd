package webui

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/web"
)

// printTemplate clones baseTemplate and layers print.tmpl's "content"
// override on top of the clone (see render.go/dashboard.go for why a
// clone, not a shared parse).
var printTemplate = template.Must(template.Must(baseTemplate.Clone()).ParseFS(web.FS, "templates/print.tmpl"))

// printPageTitle avoids repeating the same string literal across every
// render/renderError call below (goconst).
const printPageTitle = "Print"

// printPage is the Print page's model — the only data its template sees.
// ReceiptJSON and Printer echo back a rejected submission's form values;
// Message is a malformed-JSON validation message; JobID is set only on
// the GET that follows a successful submission's redirect (see Submit),
// never alongside ReceiptJSON/Printer/Message. CSRFToken is the current
// token (docs/adr/0027), embedded as the form's hidden field on every
// render — including a rejected submission's re-render, since the token
// doesn't vary by request and print.tmpl always includes the form.
type printPage struct {
	ReceiptJSON string
	Printer     string
	Message     string
	CSRFToken   string

	// JobID is confirmation text, echoed from the redirect's "job" query
	// parameter rather than looked up against the Queue — this page
	// confirms submission, not the Job's current state, which may
	// already differ by the time this GET runs.
	JobID string
}

// PrintHandler serves the Quick Print pages: the form (GET /print) and
// submission (POST /print). Every submission reaches printing only
// through Service.Print, the same enqueue-only path
// internal/api's POST /api/v1/print uses — PrintHandler never touches a
// Printer or the Queue directly.
type PrintHandler struct {
	Service *app.Service
}

// NewPrintHandler returns a PrintHandler backed by svc.
func NewPrintHandler(svc *app.Service) *PrintHandler {
	return &PrintHandler{Service: svc}
}

// Show serves GET /print: the form, plus a confirmation when a "job"
// query parameter is present — the redirect target of a successful
// Submit (see printPage.JobID).
func (h *PrintHandler) Show(w http.ResponseWriter, r *http.Request) {
	render(w, printTemplate, http.StatusOK, printPage{
		JobID:     r.URL.Query().Get("job"),
		CSRFToken: csrfToken(),
	})
}

// Submit serves POST /print. Decoding the submitted JSON is this
// handler's own concern, exactly as in PreviewHandler.Generate; once it
// decodes into a receipt.Receipt, Service.Print owns every other rule —
// content validation and job creation — so Submit duplicates none of it.
// A successful submission redirects (PRG) rather than rendering a body
// directly, so reloading or retrying the response never resubmits it.
func (h *PrintHandler) Submit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	if err := r.ParseForm(); err != nil {
		renderError(w, printPageTitle, apperr.Wrap(apperr.KindValidation, "webui.Print", err))
		return
	}
	if !verifyCSRF(r) {
		renderError(w, printPageTitle, apperr.Wrap(apperr.KindValidation, "webui.Print", errCSRF))
		return
	}

	receiptJSON := r.FormValue("receipt")
	printerName := r.FormValue("printer")

	var rc receipt.Receipt
	if err := json.Unmarshal([]byte(receiptJSON), &rc); err != nil {
		render(w, printTemplate, http.StatusBadRequest, printPage{
			ReceiptJSON: receiptJSON,
			Printer:     printerName,
			Message:     "The submitted receipt JSON could not be parsed. Check the JSON and try again.",
			CSRFToken:   csrfToken(),
		})
		return
	}

	jobID, err := h.Service.Print(r.Context(), rc, printerName, "")
	if err != nil {
		renderError(w, printPageTitle, err)
		return
	}

	http.Redirect(w, r, "/print?job="+url.QueryEscape(jobID), http.StatusSeeOther)
}
