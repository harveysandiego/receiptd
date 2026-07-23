# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/) —
see [VERSIONING.md](VERSIONING.md) for what that means in practice during
the 0.x series.

## [Unreleased]

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

[Unreleased]: https://github.com/harveysandiego/receiptd/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/harveysandiego/receiptd/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/harveysandiego/receiptd/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/harveysandiego/receiptd/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/harveysandiego/receiptd/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/harveysandiego/receiptd/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/harveysandiego/receiptd/releases/tag/v0.1.0
