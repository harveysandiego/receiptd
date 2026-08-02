package webui

import (
	"fmt"
	"html/template"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/web"
)

// formatSize renders a byte count for display, in the largest unit that
// keeps it under four digits.
func formatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for size := n / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

// assetsTemplate clones baseTemplate and layers assets.tmpl's "content"
// override on top of the clone (see render.go/dashboard.go for why a
// clone, not a shared parse).
var assetsTemplate = template.Must(template.Must(baseTemplate.Clone()).ParseFS(web.FS, "templates/assets.tmpl"))

// assetRow is one stored asset's presentation row, mirroring
// printerRow/dashboardPage: this page's template depends on its own
// model, never on app.AssetSummary directly. Size and Modified are
// pre-formatted strings so the template does no arithmetic; IsImage
// decides between a thumbnail and a download link.
type assetRow struct {
	Name     string
	Size     string
	Modified string
	IsImage  bool
}

// assetsPage is the Assets page's model — the only data its template
// sees. CSRFToken (docs/adr/0027) is the hidden field both the upload
// form and every per-row delete form embed.
type assetsPage struct {
	Assets    []assetRow
	CSRFToken string
}

// AssetsHandler serves the asset-management pages: listing (GET
// /assets), upload (POST /assets, multipart/form-data per
// docs/adr/0026-asset-upload-multipart-form-data.md), and delete (POST
// /assets/{name}/delete). Every method reaches Service only through
// ListAssets/UploadAsset/DeleteAsset, never assets.Store directly.
type AssetsHandler struct {
	Service *app.Service
}

// NewAssetsHandler returns an AssetsHandler backed by svc.
func NewAssetsHandler(svc *app.Service) *AssetsHandler {
	return &AssetsHandler{Service: svc}
}

// List serves GET /assets.
func (h *AssetsHandler) List(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.Service.ListAssets(r.Context())
	if err != nil {
		renderError(w, "Assets", err)
		return
	}

	rows := make([]assetRow, len(summaries))
	for i, s := range summaries {
		rows[i] = assetRow{
			Name:     s.Name,
			Size:     formatSize(s.Size),
			Modified: s.ModTime.Format("2006-01-02 15:04"),
			IsImage:  isInlineExtension(s.Name),
		}
	}

	render(w, assetsTemplate, http.StatusOK, assetsPage{
		Assets:    rows,
		CSRFToken: csrfToken(),
	})
}

// Upload serves POST /assets. The uploaded name and file are handed
// straight to Service.UploadAsset — an empty or otherwise invalid name is
// assets.Store's own validateName rejecting it (apperr.KindValidation),
// not a rule Upload duplicates here.
func (h *AssetsHandler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	if err := r.ParseMultipartForm(maxRequestBodyBytes); err != nil {
		renderError(w, "Assets", apperr.Wrap(apperr.KindValidation, "webui.Upload", err))
		return
	}
	if !verifyCSRF(r) {
		renderError(w, "Assets", apperr.Wrap(apperr.KindValidation, "webui.Upload", errCSRF))
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		renderError(w, "Assets", apperr.Wrap(apperr.KindValidation, "webui.Upload", err))
		return
	}
	defer func() { _ = file.Close() }()

	// Read fully into memory rather than streamed: Service.UploadAsset's
	// contract is a plain []byte (docs/adr/0026), and maxRequestBodyBytes
	// above already bounds this to a small, known size before
	// ParseMultipartForm even runs — an intentional consequence of that
	// contract and cap, not an oversight.
	data, err := io.ReadAll(file)
	if err != nil {
		renderError(w, "Assets", apperr.Wrap(apperr.KindPermanent, "webui.Upload", err))
		return
	}

	name := r.FormValue("name")
	if err := h.Service.UploadAsset(r.Context(), name, data); err != nil {
		renderError(w, "Assets", err)
		return
	}

	http.Redirect(w, r, "/assets", http.StatusSeeOther)
}

// Content serves GET /assets/{name}/content: the asset's own bytes, so
// the Assets page can show a thumbnail and link to the full-size image.
// Anything not on the inline allowlist downloads instead of rendering
// (docs/adr/0029-asset-content-endpoint-inline-type-allowlist.md). It has
// no CSRF check for the same reason GET/POST preview doesn't — it mutates
// nothing.
func (h *AssetsHandler) Content(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	data, err := h.Service.GetAsset(r.Context(), name)
	if err != nil {
		renderError(w, "Assets", err)
		return
	}

	// The disposition carries the asset's name in both cases: the URL ends
	// in /content, so without it a browser's save-as would offer
	// "content".
	disposition := "attachment"
	if ct, ok := inlineType(name, data); ok {
		disposition = "inline"
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	// Put overwrites in place, so a cached response would show the
	// previous asset under an unchanged name.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// Delete serves POST /assets/{name}/delete.
func (h *AssetsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	if err := r.ParseForm(); err != nil {
		renderError(w, "Assets", apperr.Wrap(apperr.KindValidation, "webui.Delete", err))
		return
	}
	if !verifyCSRF(r) {
		renderError(w, "Assets", apperr.Wrap(apperr.KindValidation, "webui.Delete", errCSRF))
		return
	}

	name := r.PathValue("name")
	if err := h.Service.DeleteAsset(r.Context(), name); err != nil {
		renderError(w, "Assets", err)
		return
	}

	http.Redirect(w, r, "/assets", http.StatusSeeOther)
}
