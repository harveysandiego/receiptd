# web

Embedded assets for `internal/webui` (Milestone 4), served via
`//go:embed templates static` (`embed.go`) — nothing here is processed by
a build step other than `go build` itself
(`docs/adr/0022-webui-server-rendered-html-template.md`).

- `templates/` — `html/template` `.tmpl` files, one per `internal/webui`
  page plus `base.tmpl`'s shared layout.
- `static/` — CSS and vanilla-JS static assets served under `/static/`:
  `style.css` (design tokens + theming), `app.js` (sitewide progressive
  enhancement), `dashboard.js`.
