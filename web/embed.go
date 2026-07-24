// Package web holds internal/webui's templates and static assets,
// embedded into the binary so Receiptd stays a single Go build with no
// separate frontend toolchain (docs/adr/0022-webui-server-rendered-html-template.md).
package web

import "embed"

// FS embeds every file under templates/ and static/. internal/webui
// parses templates/*.tmpl via html/template and serves static/ directly;
// nothing under either directory is processed by a build step other than
// go build itself.
//
//go:embed templates static
var FS embed.FS
