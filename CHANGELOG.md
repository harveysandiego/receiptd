# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/) —
see [VERSIONING.md](VERSIONING.md) for what that means in practice during
the 0.x series.

## [Unreleased]

## [0.6.0] - 2026-08-03

### Added

- The running build is now readable from every operator-facing seam: a
  `receiptd --version` flag, `GET /api/v1/version`, a footer on every Web
  UI page, and a `receipt version` subcommand reporting the CLI's own
  build alongside the daemon's. `receiptd --version` needs no config file
  and `receipt version` still reports the CLI's build when the daemon is
  unreachable — both are meant to work while filing a bug about a daemon
  that won't start. The endpoint sits behind the same Bearer auth as every
  other route. See
  [ADR-0030](docs/adr/0030-build-version-surfaced-at-operator-seams.md).

- The Web UI's Print and Preview forms now suggest every configured
  printer's name in the printer field via a `<datalist>`, backed by a new
  `app.Service.PrinterNames`. It reads only the configured printers' names
  and deliberately does no `Status` probe, unlike `ListPrinters`, so it
  stays cheap enough to call on every page render and an unreachable
  printer can't slow the form down.
- Web UI theming: a Theme picker in the header offers System, Light, Dark
  and High contrast, persisted in `localStorage` and applied as a
  `data-theme` attribute. "System" stores nothing and defers to the
  browser's `prefers-color-scheme`.
- `app.js`, a sitewide progressive-enhancement script: active-nav
  highlighting, drag-and-drop onto the Assets file picker, a confirmation
  prompt on destructive submits (asset deletion), and submit buttons that
  disable themselves for the duration of a form navigation.
- The Printers page renders each printer's status as a coloured badge, and
  the Dashboard briefly flashes a Printers/Queue figure when polling
  changes it, so a value ticking over is noticeable without watching the
  tab.

- An asset browser on the Web UI's Assets page: each row now shows the
  asset's size, modified time, and — for common image formats — a
  thumbnail, served by a new `GET /assets/{name}/content`. An asset is
  served inline only when its extension is on a fixed image allowlist
  *and* its bytes sniff to the same type; everything else, including SVG,
  downloads as an attachment, since a caller controls both an asset's
  name and its content. See
  [ADR-0029](docs/adr/0029-asset-content-endpoint-inline-type-allowlist.md).
  Clicking a thumbnail opens the full-size asset in a native `<dialog>`
  without leaving the page; with JavaScript disabled the same thumbnail
  is a plain link to the asset, and a file whose bytes don't match its
  extension falls back to a download link rather than a broken image.

- A form-based receipt element builder on the Print and Preview pages
  (`web/static/builder.js`), covering every `receipt` element type
  including nested `columns`. It is progressive enhancement over the
  existing Receipt JSON textarea rather than a replacement: the server
  side is unchanged, "Use raw JSON instead" restores the textarea, and a
  receipt containing anything the builder can't represent leaves the raw
  editor in place instead of dropping content. Preview also flags when a
  displayed preview is older than the current input.

### Changed

- The Web UI stylesheet is a full redesign around CSS custom-property
  design tokens, replacing the previous minimal stylesheet. The Printers
  and Assets tables now scroll horizontally within their own container
  rather than widening the page on a narrow viewport.
- `assets.Store.List` now returns `[]assets.Info` (name, size, modified
  time) rather than `[]string`, and `app.AssetSummary` carries the size
  and modified time through to the Web UI. Both implementations already
  held those fields at the point `List` ran (on Unix this costs one
  additional `lstat` per entry; on Windows it's free), avoiding the
  `Stat`-per-row a bolted-on alternative would need. `Get`, `Put`, and
  `Delete` are unchanged — in particular `Get`, the only method
  `render/layout.Build` calls, so nothing downstream of a Receipt is
  affected. See [ADR-0028](docs/adr/0028-asset-store-list-returns-info.md).

### Documentation

- `SECURITY.md`'s Scope section said the Web UI was planned but not yet
  implemented, which stopped being true in v0.5.0. It now describes the
  Web UI as a real surface and names Web UI XSS/CSRF and the serving of
  stored asset bytes as in scope, so a reporter isn't told a whole
  surface is out of scope.

### Fixed

- `receiptd --version`, which `.github/ISSUE_TEMPLATE/bug_report.md` asks
  bug reporters to run, exited with a flag error instead of printing a
  version — the daemon only ever registered `-config`.
- `internal/queue`'s `EnqueueIdempotent` tests opened a bbolt store and
  never closed it, so `t.TempDir()`'s cleanup could not remove the
  database file and eight tests failed. Only ever visible on Windows,
  which refuses to unlink a file that is still open where Linux allows
  it — so `go test ./...` was reliably red on a Windows checkout and
  green everywhere else.
- The Assets page's size column panicked (index out of range) for any
  stored asset of 1 PiB or larger, taking down `GET /assets` entirely
  until the offending file was removed.

## [0.5.1] - 2026-07-25

### Fixed

- The published `v0.5.0` container image build was broken: `.dockerignore`
  excluded `web/` entirely and the `Dockerfile` never copied it into the
  build stage, both predating Milestone 4, from before `internal/webui`
  embedded `web/`'s templates and static assets via `//go:embed`. `go
  build`/`go test` outside Docker were never affected (this checkout
  always has `web/` on disk), which is why it went unnoticed until the
  actual release build ran. `v0.5.0`'s binaries and GitHub Release are
  unaffected and remain as published; `v0.5.0` has no container image —
  use `v0.5.1` or later for `ghcr.io/harveysandiego/receiptd`.
- A pre-existing, intermittent data race in
  `TestDaemon_Run_ReconcilesOrphanedRunningJobBeforeAnyWorkerStarts`
  (`cmd/receiptd`): the test read `daemon.workerCancel` from its main
  goroutine without waiting for a proper synchronization point back to
  the goroutine running `daemon.run()`, which sets it. No production
  code was affected — `run()`/`startWorker()` already have no such race
  in the real (non-test) code path.

## [0.5.0] - 2026-07-24

### Added

- Web UI (Milestone 4): a server-rendered `html/template` frontend in
  `internal/webui`, opt-in via `web.enabled` and mounted alongside the
  REST API behind the same shared-token auth (`auth.Basic` in place of
  `auth.Bearer`) — no login page, no session, no framework, per
  [ADR-0022](docs/adr/0022-webui-server-rendered-html-template.md) and
  [ADR-0023](docs/adr/0023-webui-authentication-reuses-shared-token.md).
  Pages: a dashboard (printer/asset/queue counts), a read-only printer
  settings screen
  ([ADR-0024](docs/adr/0024-printer-settings-screen-is-read-only.md)),
  asset management (`multipart/form-data` upload, list, delete, per
  [ADR-0026](docs/adr/0026-asset-upload-multipart-form-data.md)), a
  receipt preview form (renders a PNG, never prints), and a quick-print
  form (submits a Receipt, always creating a Job on success — the same
  `app.Service.Print` path `POST /api/v1/print` already uses). Every
  state-changing action (asset upload/delete, quick print) redirects to
  a GET after succeeding (POST/redirect/GET), so reloading the result
  page never repeats the action. See the README's
  [Web UI](README.md#web-ui) section.
- Dashboard client-side polling: `GET /status` serves a JSON snapshot of
  the same printer/queue counts the dashboard renders, and
  `web/static/dashboard.js` — the Web UI's one script — polls it every 5
  seconds to refresh the dashboard's Printers/Queue cards without a page
  reload, per
  [ADR-0025](docs/adr/0025-dashboard-updates-via-polling.md).

### Changed

- Web UI printer status checks (backing the dashboard, `/printers`, and
  `/status`) now run concurrently, one goroutine per configured printer,
  each bounded by a 5s timeout well under the HTTP server's 30s
  `WriteTimeout` — a slow or offline printer can no longer delay the
  others or the whole request; results stay sorted by name regardless of
  completion order.
- `internal/webui`'s shared `render()` now buffers template execution and
  only commits headers/status once it succeeds, so a template bug can no
  longer leave a response half-written with the wrong status already
  sent.
- `web/static/dashboard.js`'s polling now self-schedules its next request
  only once the current one finishes, rather than a fixed timer, making
  overlapping requests structurally impossible, and pauses while the
  browser tab is hidden, resuming on `visibilitychange`.

### Security

- CSRF protection for every state-changing Web UI route (`POST /print`,
  `/assets`, `/assets/{name}/delete`): a per-process HMAC-signed token
  embedded as each protected form's hidden field, verified in constant
  time — no session, no cookie, per
  [ADR-0027](docs/adr/0027-webui-csrf-protection-via-per-process-hmac-token.md).
  `POST /preview` is unaffected — it never mutates state.
- Every Web UI response now carries a defensive set of security headers
  (Content-Security-Policy, `X-Frame-Options`, `X-Content-Type-Options`,
  `Referrer-Policy`), applied uniformly by a middleware wrapping the
  whole router.
- `POST /preview` and `POST /print` now apply the same request body size
  limit asset uploads already had, preventing an oversized body from
  consuming excessive memory.
- The Printers page no longer shows a printer's raw connection error
  (transport-level dial failure text) to the browser — it's logged
  server-side for an administrator instead, with the page showing a
  plain Online/Offline.

### Documentation

- `docs/ARCHITECTURE.md` §10 gained a "Milestone 4 as built" section
  documenting the Web UI's route table and the shared handler/page-model/
  PRG patterns every page follows, replacing several `internal/webui`
  code comments' previously-unfulfilled references to "the route table"
  in that section; that section now also covers CSRF protection, security
  headers, and concurrent printer status.
- ADR-0022 through ADR-0027 are all marked `Accepted` after confirming
  each decision's implementation, including ADR-0025's dashboard polling
  and the new ADR-0027 (Web UI CSRF protection).

## [0.4.0] - 2026-07-23

### Added

- `Receipt.copies` is now implemented: a Job prints that many physical
  copies, rendering and encoding once and repeating only the final send to
  the printer. Previously the field was decoded and round-tripped but had
  no effect — every Job printed exactly once regardless of its value.
  `copies` must be within `[0, 100]`; a value over 100 is rejected at
  validation time, so one request can't monopolize a printer.
- Startup crash recovery: `receiptd` now reconciles any `Job` left
  `running` by a previous crash or unclean death before it starts
  processing anything new. A recovered Job is automatically requeued
  (retried) if it still has retry budget left, or failed visibly with an
  `interrupted: daemon restarted...` `LastError` if it doesn't — instead of
  sitting stuck and invisible in `running` forever, per
  [ADR-0017](docs/adr/0017-queue-lifecycle-crash-recovery.md).
- Idempotent print requests: `POST /api/v1/print` accepts an optional
  `Idempotency-Key` header. Retrying the same key returns the original
  Job's ID instead of enqueuing a second print, for 24 hours, per
  [ADR-0020](docs/adr/0020-idempotent-print-requests.md). Omitting the
  header keeps today's behavior unchanged.
- Graceful shutdown: `receiptd` now handles `SIGTERM`/`SIGINT` by stopping
  new HTTP requests and queue claims immediately, letting in-flight work
  (in particular a Job already streaming raster bytes to a printer) finish
  naturally, bounded by a 30-second internal deadline, per
  [ADR-0018](docs/adr/0018-graceful-shutdown.md). Operators must raise
  their `SIGTERM`→`SIGKILL` grace period above this deadline — see the
  README's "Graceful shutdown and restart grace periods" section.

### Changed

- Each configured printer now has its own background worker, so a slow or
  offline printer can no longer block Jobs queued for a different, healthy
  printer, per
  [ADR-0016](docs/adr/0016-queue-concurrency-per-printer-workers.md). For
  the common single-printer deployment this is no observable change. With
  multiple printers configured, there is no longer a single global claim
  order across all of them — only per-printer ordering is guaranteed
  (already arbitrary with respect to enqueue order, as Job IDs are random
  hex, not time-ordered).

### Fixed

- The background queue worker no longer crashes the whole daemon if
  rendering, encoding, or printing panics: the panic is recovered, logged
  with the Job ID and a stack trace, and the Job is failed
  (`apperr.KindPermanent`, not retried) — later Jobs are still processed
  normally.
- `queue.max_attempts` and `queue.retry_backoff` now actually take
  effect. Previously `config.Validate` accepted and required them, but
  the queue worker silently used its own hardcoded 3 attempts/5s backoff
  instead.
- A queued Job's retry backoff wait is now interruptible by context
  cancellation instead of always sleeping out the full delay.
- `receiptd`'s HTTP server now sets `ReadTimeout`, `ReadHeaderTimeout`,
  `WriteTimeout`, and `IdleTimeout` instead of using `net/http`'s
  no-timeout defaults, so a slow or stalled client can no longer hold a
  server goroutine open indefinitely.

### Security

- `text`/`heading`'s `size`, `divider`'s `size`, and `barcode`'s `height`
  are now bounded (`apperr.KindValidation` above 100, 100, and 10,000
  dots respectively) — previously only a negative value was rejected, so
  an oversized value could force an excessive allocation or overflow an
  integer further down the rendering pipeline.

## [0.3.1] - 2026-07-22

### Security

- The REST API no longer includes wrapped error detail, filesystem/database
  paths, network errors, or `apperr.Error` operation names in a **5xx**
  response body — those are now logged server-side, with clients getting a
  fixed `{"error":"internal server error"}` message instead. **4xx**
  responses (validation failures, malformed JSON, not-found, unauthorized)
  are unchanged and still return the detailed, actionable message.

## [0.3.0] - 2026-07-21

### Changed

- **Breaking:** `printers[]` config entries now require either a known
  `model:` (looked up in a small built-in `printer.ModelProfiles`
  catalogue, currently `epson-tm-m30ii`) or an explicit `profile:` block
  — never both, never neither — instead of a flat set of profile fields
  on the entry itself. The old `width_mm` field is retired entirely (no
  alias); its replacement inside `profile:` is `printable_width_mm`,
  which must be the printhead's actual printable width, not the paper
  roll width. No migration shim is provided — see
  [ADR-0015](docs/adr/0015-printer-model-catalogue.md) for why a
  paper-width heuristic was rejected in favor of this split.

## [0.2.0] - 2026-07-21

### Added

- `list` Element type: bulleted, numbered, and checkbox lists as one
  `receipt.List`/`ListItem` shape with a closed-enum `Kind`, per
  [ADR-0014](docs/adr/0014-list-elements.md). Renders through the
  existing text-layout pipeline — markers, semantic indentation, and
  hanging-indent word-wrap are all resolved by `layout.Build`, with no new
  drawing primitive in `render/canvas`.

## [0.1.1] - 2026-07-20

### Fixed

- Stale godoc comments in `cmd/receiptd` and `internal/api` that still
  described Milestone 2/3 as in-progress or future work (a fake-printer
  worker, a "will become" printer.Connection claim, a pending Image/Asset
  body-size consideration) — all of that shipped in
  [0.1.0](#010---2026-07-20), so the comments now describe current
  behavior instead of an outdated plan. No behavior change.

## [0.1.0] - 2026-07-20

First tagged release. Covers
[Milestones 1, 2, 3, and 5](docs/ARCHITECTURE.md#10-roadmap) — Milestone 4
(Web UI) and Milestone 6 (first template + provider) remain outstanding.

### Added

- `receipt`: the Receipt model, JSON polymorphism, and every Element type
  — Text, Heading, Divider, Spacer, Image, Asset, QRCode, Barcode,
  Columns, Table, Feed, Cut — each with a fast, local `Validate()`.
- `render/layout` and `render/canvas`: Receipt + printer `Profile` →
  `Document` → `Canvas` (bitmap), including text wrapping/alignment,
  image/QR/barcode rasterization, and table/column layout.
- `render/escpos`: Canvas → ESC/POS byte encoding, and `printer`'s
  `Profile`/`Connection` model and network transport for real hardware —
  Receiptd has printed successfully to a physical Epson TM-m30II.
- A REST API (`/api/v1/preview`, `/api/v1/print`,
  `GET /api/v1/jobs/{id}`) backed by a persistent, bbolt-backed job
  `queue` with retry/backoff, and `auth` (Bearer-token by default, Basic
  also available).
- `assets`: named asset storage (filesystem and in-memory) for images
  referenced by receipts.
- The `receipt` CLI (`render`, `preview`, `print`, `jobs`) and the
  `receiptd` daemon (`cmd/receiptd`).
- A multi-stage `Dockerfile` producing a static, non-root, distroless
  runtime image, and automated multi-architecture (linux/amd64,
  linux/arm64) publishing to `ghcr.io/harveysandiego/receiptd` on tagged
  releases via a reusable Buildx workflow that also validates PRs — see
  the Docker section of [README.md](README.md#docker).
- Repository scaffolding: architecture documentation, ADRs, CI/CD, and
  contribution guidelines — see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

[Unreleased]: https://github.com/harveysandiego/receiptd/compare/v0.6.0...HEAD
[0.6.0]: https://github.com/harveysandiego/receiptd/compare/v0.5.1...v0.6.0
[0.5.1]: https://github.com/harveysandiego/receiptd/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/harveysandiego/receiptd/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/harveysandiego/receiptd/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/harveysandiego/receiptd/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/harveysandiego/receiptd/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/harveysandiego/receiptd/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/harveysandiego/receiptd/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/harveysandiego/receiptd/releases/tag/v0.1.0
