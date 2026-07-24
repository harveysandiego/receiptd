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

	return mux
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
