package webui

import (
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

// render executes tmpl's "base" template, writing status and a text/html
// Content-Type first. Every webui handler goes through this one function,
// so a template execution failure is logged consistently in exactly one
// place. render has no opinion about what tmpl or data mean — deciding
// those, and anything about page title, navigation, or layout, is each
// handler's/page model's job, not this file's.
func render(w http.ResponseWriter, tmpl *template.Template, status int, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		log.Printf("webui: render base: %v", err)
	}
}
