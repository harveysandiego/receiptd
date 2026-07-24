package webui

import (
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/web"
)

// previewTemplate clones baseTemplate and layers preview.tmpl's "content"
// override on top of the clone (see render.go/dashboard.go for why a
// clone, not a shared parse).
var previewTemplate = template.Must(template.Must(baseTemplate.Clone()).ParseFS(web.FS, "templates/preview.tmpl"))

// previewPageTitle avoids repeating the same string literal across every
// render/renderError call below (goconst).
const previewPageTitle = "Preview"

// previewPage is the Preview page's model — the only data its template
// sees. ReceiptJSON and Printer echo back the submitted form values (so a
// rejected submission doesn't make the user retype it); Message and
// ImageDataURI are mutually exclusive outcomes of a POST, both empty on
// the initial GET.
type previewPage struct {
	ReceiptJSON string
	Printer     string

	// Message is a malformed-JSON validation message; empty when the
	// submission decoded cleanly (or on the initial GET).
	Message string

	// ImageDataURI is the rendered PNG as a "data:image/png;base64,..."
	// URI, embedded directly rather than served from a second route — this
	// page is one synchronous request, per this slice's scope. Empty means
	// no preview has been generated yet. The type is template.URL, not
	// string: html/template rejects an untrusted data: URL unless it's
	// explicitly marked trusted this way.
	ImageDataURI template.URL
}

// PreviewHandler serves the receipt preview pages: the form (GET
// /preview) and the rendered preview (POST /preview). It never enqueues a
// Job or touches a printer directly — every render goes through
// Service.Preview, the same read-only path app.Service.Preview itself
// guarantees (docs/adr/0006-preview-requires-printer-profile.md).
type PreviewHandler struct {
	Service *app.Service
}

// NewPreviewHandler returns a PreviewHandler backed by svc.
func NewPreviewHandler(svc *app.Service) *PreviewHandler {
	return &PreviewHandler{Service: svc}
}

// Show serves GET /preview: an empty form and the placeholder state.
func (h *PreviewHandler) Show(w http.ResponseWriter, _ *http.Request) {
	render(w, previewTemplate, http.StatusOK, previewPage{})
}

// Generate serves POST /preview. Decoding the submitted JSON is this
// handler's own concern (a malformed body re-renders the form with a
// validation message, never reaching Service); once it decodes into a
// receipt.Receipt, Preview owns every other rule — content validation,
// printer resolution, rendering — so Generate duplicates none of it.
// Preview has no CSRF check (docs/adr/0027): it never mutates state, so
// it sits outside that decision's scope the same way it sits outside the
// PRG pattern every state-changing page follows.
func (h *PreviewHandler) Generate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	if err := r.ParseForm(); err != nil {
		renderError(w, previewPageTitle, apperr.Wrap(apperr.KindValidation, "webui.Preview", err))
		return
	}

	receiptJSON := r.FormValue("receipt")
	printerName := r.FormValue("printer")

	var rc receipt.Receipt
	if err := json.Unmarshal([]byte(receiptJSON), &rc); err != nil {
		render(w, previewTemplate, http.StatusBadRequest, previewPage{
			ReceiptJSON: receiptJSON,
			Printer:     printerName,
			Message:     "The submitted receipt JSON could not be parsed. Check the JSON and try again.",
		})
		return
	}

	pngBytes, err := h.Service.Preview(r.Context(), rc, printerName)
	if err != nil {
		renderError(w, previewPageTitle, err)
		return
	}

	render(w, previewTemplate, http.StatusOK, previewPage{
		ReceiptJSON:  receiptJSON,
		Printer:      printerName,
		ImageDataURI: template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)),
	})
}
