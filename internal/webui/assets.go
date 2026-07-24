package webui

import (
	"html/template"
	"io"
	"net/http"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/web"
)

// assetsTemplate clones baseTemplate and layers assets.tmpl's "content"
// override on top of the clone (see render.go/dashboard.go for why a
// clone, not a shared parse).
var assetsTemplate = template.Must(template.Must(baseTemplate.Clone()).ParseFS(web.FS, "templates/assets.tmpl"))

// maxAssetUploadBytes bounds a multipart upload body. It happens to equal
// internal/api's own maxRequestBodyBytes (internal/api/status.go), but is
// its own constant rather than an import of that one: webui and api are
// sibling packages, neither depending on the other
// (docs/ARCHITECTURE.md §11), and docs/adr/0026 scopes multipart decoding
// to this one webui handler specifically — sharing the constant would add
// a package dependency neither side otherwise needs.
const maxAssetUploadBytes = 10 << 20

// assetRow is one stored asset's presentation row. AssetSummary currently
// carries only a Name, but assetRow is still its own type, mirroring
// printerRow/dashboardPage: this page's template depends on its own
// model, never on app.AssetSummary directly.
type assetRow struct {
	Name string
}

// assetsPage is the Assets page's model — the only data its template
// sees.
type assetsPage struct {
	Title  string
	Assets []assetRow
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
		rows[i] = assetRow{Name: s.Name}
	}

	render(w, assetsTemplate, http.StatusOK, assetsPage{
		Title:  "Assets",
		Assets: rows,
	})
}

// Upload serves POST /assets. The uploaded name and file are handed
// straight to Service.UploadAsset — an empty or otherwise invalid name is
// assets.Store's own validateName rejecting it (apperr.KindValidation),
// not a rule Upload duplicates here.
func (h *AssetsHandler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAssetUploadBytes)
	if err := r.ParseMultipartForm(maxAssetUploadBytes); err != nil {
		renderError(w, "Assets", apperr.Wrap(apperr.KindValidation, "webui.Upload", err))
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		renderError(w, "Assets", apperr.Wrap(apperr.KindValidation, "webui.Upload", err))
		return
	}
	defer func() { _ = file.Close() }()

	// Read fully into memory rather than streamed: Service.UploadAsset's
	// contract is a plain []byte (docs/adr/0026), and maxAssetUploadBytes
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

// Delete serves POST /assets/{name}/delete.
func (h *AssetsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.Service.DeleteAsset(r.Context(), name); err != nil {
		renderError(w, "Assets", err)
		return
	}

	http.Redirect(w, r, "/assets", http.StatusSeeOther)
}
