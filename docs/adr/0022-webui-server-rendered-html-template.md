# 0022. Web UI is server-rendered `html/template`, not a JS SPA

Status: Proposed

## Context

`internal/webui/doc.go` already gestures at the intended shape: "HTML
handlers (`html/template` plus embedded static assets from `web/`), backed
by the same `app.Service` used by `api`." `web/README.md` likewise
describes `web/` as "embedded static assets and HTML templates for
`internal/webui`" — empty until Milestone 4. Neither file commits to that
direction with a recorded rationale; this ADR is that rationale.

Milestone 4 (`docs/ARCHITECTURE.md` §10) scopes the Web UI to: printer
status, quick actions, preview, text printing, image upload, asset
management, and printer settings. All of it reads and writes through
`internal/app.Service` — the same entry point `internal/api` already uses,
per the same rule (`docs/ARCHITECTURE.md` §1: "the only thing `api`/
`webui` call into for business logic").

Two ways to build that UI were on the table: a server-rendered Go
`html/template` frontend serving embedded static assets (`//go:embed`
from `web/`), or a JavaScript single-page application with its own build
toolchain (npm/yarn, a bundler, a component framework) consuming
`internal/api` as its data source.

The constraints this decision has to fit are frozen elsewhere and not
re-litigated here: single Go binary, `CGO_ENABLED=0`
(`docs/ARCHITECTURE.md` §11), a Raspberry Pi/homelab deployment target, a
reverse proxy terminating TLS in front of this UI (`docs/adr/0021-
transport-security-via-reverse-proxy.md`), and shared-token
Bearer/Basic authentication with no users, roles, sessions, or cookies
(`internal/auth`). This project's one extension mechanism is compile-time
registry + blank-import (`docs/adr/0004-extension-model.md`), and package
creation is deliberately non-speculative (`CLAUDE.md`).

## Decision

`internal/webui` renders HTML server-side using the standard library's
`html/template`, and serves its CSS/JS/image assets embedded into the
binary via `//go:embed` from `web/`. There is no separate frontend build
step, no `package.json`, no bundler, and no client-side framework.

Concretely:

- `internal/webui` handlers call `internal/app.Service` directly (the same
  contract `internal/api` follows), execute a `html/template`, and write
  the response — no JSON round-trip through `internal/api` from the
  browser's own UI.
- Any interactivity (asset upload progress, a job status poll) is plain,
  dependency-free JavaScript shipped as a static file under `web/`, or a
  full-page form submission where that's simpler. No SPA routing, no
  virtual DOM, no client-side state store.
- Authentication for `webui` routes is a separate decision — see
  `docs/adr/0023-webui-authentication-reuses-shared-token.md` — and is not
  restated here.
- `web/` holds the `.tmpl` files and static assets embedded via
  `//go:embed`; nothing under `web/` is compiled by a separate toolchain
  before `go build` runs.

## Consequences

- `go build` remains the entire build: one toolchain, one binary, no
  `npm install`/`npm run build` step to keep working, document, or run in
  CI. This matches the single-binary, `CGO_ENABLED=0`, Raspberry-Pi
  deployment target directly — nothing new to cross-compile per
  architecture.
- No new dependency surface: no `package.json`/`node_modules` to patch for
  security advisories, no bundler config to maintain for years the way
  `CLAUDE.md` asks this project to plan for. The only new import is the
  standard library's `html/template` plus `embed`.
- `internal/webui` stays a thin HTML-rendering layer over the same
  `app.Service` contract `internal/api` already uses — no second,
  JSON-shaped API surface has to be designed, versioned, or kept
  request-compatible with a separate frontend release cadence.
- The UI is plain HTML by default: every page load is a full round trip,
  and anything the team later wants to feel "app-like" (live job status,
  optimistic UI) has to be hand-rolled in small, dependency-free
  JavaScript rather than reached for off a framework's shelf. This is a
  real, named cost against developer convenience, accepted deliberately
  per `CLAUDE.md`'s "long-term maintainability prioritized over developer
  convenience or speed-to-ship."
- Nothing about this decision touches `internal/api`: it keeps existing
  unchanged as the JSON surface for the CLI and any external integration,
  and does not gain a dependency on `internal/webui` or vice versa.
- If a future requirement genuinely needs SPA-grade interactivity (e.g. a
  live multi-printer dashboard with push updates), that is grounds for a
  new ADR to justify the added toolchain on its own merits — not something
  to introduce piecemeal into `web/` on the assumption that this decision
  was provisional.

## Alternatives considered

- **JavaScript SPA (React/Vue/Svelte + npm + bundler), consuming
  `internal/api`.** Rejected: introduces a second build toolchain and a
  `node_modules` dependency tree to a project whose deployment target is a
  single `CGO_ENABLED=0` Go binary on a Raspberry Pi, and whose stated
  priority is years-long maintainability over developer convenience. It
  would also duplicate authentication handling (the SPA would need its own
  token/credential flow against `internal/api`, parallel to whatever
  `internal/auth` already enforces) for a UI whose actual scope —
  printer status, quick actions, preview, text printing, image upload,
  asset management, printer settings — does not need client-side routing
  or a component framework to express.
- **Server-rendered HTML via a third-party Go templating/component
  library** (e.g. `templ`, htmx as a required dependency beyond a static
  script include). Rejected for now: the standard library's `html/template`
  already covers this milestone's scope, and reaching for an extra
  dependency without a second concrete need violates this project's
  default ("minimal packages, no speculative abstractions" — `CLAUDE.md`).
  Nothing here forecloses using a small vanilla-JS progressive enhancement
  where it clearly helps; it forecloses adopting a new required dependency
  or build step to get there.
