package webui

import (
	"net/http"

	"github.com/harveysandiego/receiptd/internal/app"
)

// AssetsHandler serves the asset-management pages: listing (GET
// /assets), upload (POST /assets, multipart/form-data per
// docs/adr/0026-asset-upload-multipart-form-data.md), and delete (POST
// /assets/{name}/delete). Stub until the Assets slice replaces each
// method's body.
type AssetsHandler struct {
	Service *app.Service
}

// NewAssetsHandler returns an AssetsHandler backed by svc.
func NewAssetsHandler(svc *app.Service) *AssetsHandler {
	return &AssetsHandler{Service: svc}
}

// List serves GET /assets.
func (h *AssetsHandler) List(w http.ResponseWriter, _ *http.Request) {
	renderStub(w, "Assets")
}

// Upload serves POST /assets.
func (h *AssetsHandler) Upload(w http.ResponseWriter, _ *http.Request) {
	renderStub(w, "Assets")
}

// Delete serves POST /assets/{name}/delete.
func (h *AssetsHandler) Delete(w http.ResponseWriter, _ *http.Request) {
	renderStub(w, "Assets")
}
