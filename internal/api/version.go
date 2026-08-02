package api

import "net/http"

// versionResponse is the wire shape of a successful GET /api/v1/version
// response body.
type versionResponse struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// VersionHandler serves GET /api/v1/version. Unlike this package's other
// handlers it takes no service interface: build identity is fixed for the
// life of the process, so there is nothing to ask app.Service per request.
type VersionHandler struct {
	response versionResponse
}

// NewVersionHandler returns a VersionHandler reporting version, commit,
// and date — cmd/receiptd's -ldflags values.
func NewVersionHandler(version, commit, date string) *VersionHandler {
	return &VersionHandler{response: versionResponse{Version: version, Commit: commit, Date: date}}
}

func (h *VersionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// no-store for the same reason webui's /status sets it: a version
	// cached by a reverse proxy (docs/adr/0021) would outlive an upgrade.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, h.response)
}
