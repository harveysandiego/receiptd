package webui

import (
	"bytes"
	"html/template"
	"log"
	"net/http"

	"github.com/harveysandiego/receiptd/web"
)

// baseTemplate is parsed once from the embedded base layout
// (docs/adr/0022). A page with its own content clones baseTemplate and
// parses its own templates/*.tmpl file into the clone, rather than into
// baseTemplate directly: html/template treats redefining a block name
// ("content") as last-write-wins across the whole *template.Template, so
// a shared parse would let whichever page's file was parsed last silently
// override every other page's content block.
var baseTemplate = template.Must(template.ParseFS(web.FS, "templates/base.tmpl"))

// maxRequestBodyBytes bounds every webui POST body — a request whose
// Content-Type this package parses via r.ParseForm/ParseMultipartForm
// (assets.go's upload, preview.go, print.go). It happens to equal
// internal/api's own maxRequestBodyBytes (internal/api/status.go), but is
// its own constant rather than an import of that one: webui and api are
// sibling packages, neither depending on the other
// (docs/ARCHITECTURE.md §11), so sharing the constant would add a
// package dependency neither side otherwise needs.
const maxRequestBodyBytes = 10 << 20

// pageEnvelope pairs a page's own model with the sitewide chrome data
// base.tmpl needs, so no page model carries a field it doesn't own.
// base.tmpl passes .Page into every block, so a page template still sees
// exactly its own model.
type pageEnvelope struct {
	Page    any
	Version string
}

// render executes tmpl's "base" template into an in-memory buffer first,
// and only writes the Content-Type/status/body once execution succeeds —
// so a template error (a bug in a page's own data, not the client's
// fault) can never leave a response half-written with headers already
// committed to a status that turned out to be wrong. Every webui handler
// goes through this one function, so a template execution failure is
// logged consistently in exactly one place. render has no opinion about
// what tmpl or data mean — deciding those, and anything about page
// title, navigation, or layout, is each handler's/page model's job, not
// this file's.
func render(w http.ResponseWriter, version string, tmpl *template.Template, status int, data any) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", pageEnvelope{Page: data, Version: version}); err != nil {
		log.Printf("webui: render base: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}
