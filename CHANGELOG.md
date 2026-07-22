# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/) —
see [VERSIONING.md](VERSIONING.md) for what that means in practice during
the 0.x series.

## [Unreleased]

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

[Unreleased]: https://github.com/harveysandiego/receiptd/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/harveysandiego/receiptd/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/harveysandiego/receiptd/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/harveysandiego/receiptd/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/harveysandiego/receiptd/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/harveysandiego/receiptd/releases/tag/v0.1.0
