package webui

import (
	"io/fs"
	"net/http"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/web"
)

// NewRouter builds the Web UI's full route table (docs/ARCHITECTURE.md
// §10), every handler backed by svc. NewRouter adds no authentication of
// its own — the caller wraps the returned Handler in auth.Basic
// (docs/adr/0023-webui-authentication-reuses-shared-token.md) and mounts
// it separately from internal/api's mux
// (docs/adr/0022-webui-server-rendered-html-template.md).
func NewRouter(svc *app.Service) http.Handler {
	mux := http.NewServeMux()

	// "/{$}" (not "/"), so the dashboard only claims the exact root path —
	// a bare "/" is a subtree match in Go's ServeMux and would silently
	// swallow every unmatched path (e.g. a typo'd URL) into the dashboard
	// instead of 404ing.
	mux.Handle("GET /{$}", NewDashboardHandler(svc))
	mux.Handle("GET /status", NewStatusHandler(svc))
	mux.Handle("GET /printers", NewPrintersHandler(svc))

	assets := NewAssetsHandler(svc)
	mux.HandleFunc("GET /assets", assets.List)
	mux.HandleFunc("POST /assets", assets.Upload)
	mux.HandleFunc("POST /assets/{name}/delete", assets.Delete)

	preview := NewPreviewHandler(svc)
	mux.HandleFunc("GET /preview", preview.Show)
	mux.HandleFunc("POST /preview", preview.Generate)

	print := NewPrintHandler(svc)
	mux.HandleFunc("GET /print", print.Show)
	mux.HandleFunc("POST /print", print.Submit)

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	return securityHeaders(mux)
}

// securityHeaders sets a fixed set of defensive response headers on every
// Web UI response — HTML pages, the /status JSON endpoint, and static
// assets alike, since it wraps the whole router rather than render()
// alone. The policy is deliberately as tight as this UI actually allows:
//
//   - Content-Security-Policy: everything ('self') except images, which
//     also allow data: for Preview's embedded PNG
//     (previewPage.ImageDataURI) — no inline script or style exists
//     anywhere in web/templates, so neither gets 'unsafe-inline', and
//     dashboard.js is the only script, served same-origin. object-src and
//     frame-ancestors are pinned to 'none' rather than left to
//     default-src's 'self' fallback: this UI never embeds a plugin/object
//     and is never meant to be framed by anyone, including itself, so
//     'self' would be strictly more permission than it ever needs.
//   - X-Frame-Options: DENY is the same "never framed" rule as
//     frame-ancestors above, for browsers that don't act on the CSP
//     directive.
//   - X-Content-Type-Options: nosniff, since every response's
//     Content-Type is already set deliberately (render/writeStatusJSON/
//     http.FileServerFS's own content-type sniffing by extension).
//   - Referrer-Policy: same-origin, so a link out of this UI (there are
//     none today, but templates change) never leaks the full URL,
//     including any path, to a third-party site.
func securityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; img-src 'self' data:; style-src 'self'; " +
		"script-src 'self'; base-uri 'none'; object-src 'none'; " +
		"form-action 'self'; frame-ancestors 'none'"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// staticFS serves web/static/ under /static/, above. fs.Sub only errors
// if "static" isn't a directory in web.FS, which would be a build-time
// bug (the directory is embedded at compile time), not something that
// can fail at runtime.
var staticFS = mustSubFS(web.FS, "static")

func mustSubFS(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
