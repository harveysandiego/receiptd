package webui

import (
	"html/template"
	"log"
	"net/http"

	"github.com/harveysandiego/receiptd/web"
)

// baseTemplate is parsed once from the embedded base layout
// (docs/adr/0022). A future page slice adds its own templates/*.tmpl file
// defining "content" (and optionally "title") and parses it alongside
// base.tmpl — html/template's standard block-override pattern — rather
// than changing this shared parse.
var baseTemplate = template.Must(template.ParseFS(web.FS, "templates/base.tmpl"))

// render executes name against baseTemplate, writing status and a
// text/html Content-Type first. Every webui handler goes through this
// one function, so a template execution failure is logged consistently
// in exactly one place. render has no opinion about what name or data
// mean — deciding those, and anything about page title, navigation, or
// layout, is each handler's/page model's job, not this file's.
func render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := baseTemplate.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("webui: render %s: %v", name, err)
	}
}

// stubPage is base.tmpl's "content" block's default data shape.
type stubPage struct {
	Title   string
	Message string
}

// renderStub writes a 501 placeholder page for a page whose slice hasn't
// landed yet. It exists only for this Infrastructure slice's stub
// period: once a page's own slice gives it real content and its own
// data, that handler stops calling renderStub and calls render directly
// — render itself is the only permanent part of this file.
func renderStub(w http.ResponseWriter, title string) {
	render(w, http.StatusNotImplemented, "base", stubPage{
		Title:   title,
		Message: title + " is not implemented yet.",
	})
}
