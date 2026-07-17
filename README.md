# Receiptd

**Receipt Printer as a Service** — a self-hosted daemon that turns any
ESC/POS-compatible thermal receipt printer into an API-addressable
appliance on your home network.

<!-- All badge/clone/API URLs on this page assume this repo lives at
     github.com/harveysandiego/receiptd. If this repository is ever
     forked or renamed, update this page, go.mod, and .goreleaser.yml
     together in one commit — see the note at the top of go.mod. -->

[![CI](https://github.com/harveysandiego/receiptd/actions/workflows/ci.yml/badge.svg)](https://github.com/harveysandiego/receiptd/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/harveysandiego/receiptd.svg)](https://pkg.go.dev/github.com/harveysandiego/receiptd)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/harveysandiego/receiptd)](https://goreportcard.com/report/github.com/harveysandiego/receiptd)

> **Status:** pre-alpha, architecture-complete, implementation starting.
> See [Current status](#current-status) before trying to run this.

---

## Screenshots

_Coming soon — Web UI and printed receipt examples will be added as
Milestone 4 (Web UI) lands. See the [roadmap](#roadmap)._

<!--
![Web UI dashboard](docs/img/webui-dashboard.png)
![Printed receipt example](docs/img/receipt-example.jpg)
-->

## Motivation

Thermal receipt printers are cheap, fast, tiny, and satisfying — the kind
of hardware home-lab and Home Assistant tinkerers already reach for to
print a daily agenda, a shopping list, or a "someone rang the doorbell"
slip. But every project that talks to one directly ends up hard-coding a
specific printer's codepage quirks, cut commands, and vendor-specific
barcode/QR opcodes into whatever script or automation is doing the
printing.

Receiptd's goal is to make the printer disappear as a concern: point it at
your printer once, and every client — a REST call, a CLI, a browser — sends
a plain, printer-agnostic document and gets a printed receipt out the
other end. No client anywhere needs to know what an ESC/POS command is.

## Philosophy

- **The client never knows about the printer.** Not its width, its DPI,
  its cut command, its codepage — nothing. All of that lives server-side.
- **One document model, one rendering pipeline.** JSON, Markdown, and
  server-side templates all produce the same `Receipt` structure, which is
  rendered by exactly one pipeline. No parallel code paths to keep in sync.
- **Raster-first rendering.** Everything is painted onto a bitmap using an
  embedded font and sent to the printer as an image. This sidesteps printer
  codepage/i18n differences and vendor-specific QR/barcode command
  variance entirely — the printer only ever needs to support "print this
  raster," which is close to universal across ESC/POS-compatible hardware.
- **Small, static binary.** No CGO, no runtime dependencies, cross-compiled
  for ARM64 and AMD64 — it should run comfortably on a Raspberry Pi
  alongside everything else in your home lab.
- **Long-term maintainability over speed to v1.** This project is designed
  to be maintained for years, not shipped once. See
  [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full reasoning
  behind every significant design decision, and [docs/adr/](docs/adr/) for
  the record of *why* each one was made the way it was.

## Architecture overview

```
        ┌─────────┐   ┌──────────┐   ┌──────────┐
JSON ──▶│         │   │          │   │          │
Markdown│ Receipt │──▶│  Layout  │──▶│  Canvas  │──▶ ESC/POS ──▶ Printer
Template│ (model) │   │(measure) │   │ (paint)  │        │
        └─────────┘   └──────────┘   └────┬─────┘        └─▶ Async job queue
                                           │                  (retry, persist)
                                           ▼
                                     PNG preview
```

Every input format (raw JSON, Markdown, a server-side template like
"today's weather") is converted into the same `Receipt` document — an
ordered list of typed `Element`s (text, headings, images, QR codes,
tables, and so on), deliberately similar in spirit to Slack's Block Kit.
That single document is then run through one shared pipeline:

1. **Layout** — measure text, wrap lines, resolve images/assets, compute
   positions, using the target printer's declared capabilities
   (`printer.Profile`: width, DPI, cut support) but never its connection
   details.
2. **Canvas** — paint the laid-out document onto a 1-bit bitmap. This same
   bitmap can be encoded as a PNG (for browser/API preview) or handed to
   the printer encoder.
3. **ESC/POS encoding** — turn the bitmap into raster print commands plus
   minimal real ESC/POS (init, feed, cut), tailored to the target
   printer's profile.
4. **Printing** happens asynchronously via a persistent job queue, so a
   slow or temporarily offline printer never blocks the API caller.

Full detail, including every interface, the package layout, the error
philosophy, and the reasoning behind each decision, lives in
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and the
[Architecture Decision Records](docs/adr/).

## Features

- **REST API** for printing, previewing, and checking job status
- **CLI** (`receipt`) for scripting and quick ad-hoc prints
- **Web UI** for browsing/printing without touching a terminal
- **Element-based Receipt model**: text, headings, dividers, spacers,
  images, named assets, QR codes, barcodes, columns, tables, feed, cut
- **PNG preview** before anything hits paper
- **Async, persistent print queue** with retry/backoff for transient
  printer failures
- **Server-side templates** (e.g. a daily weather receipt) that compose
  over the same Receipt/preview/print pipeline as everything else
- **Named asset storage** for logos and reusable images
- **Optional bearer-token / basic auth**, on by default
- Single static binary — Linux/macOS/Windows, amd64/arm64, no CGO

## Current status

Receiptd's architecture is frozen (see
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) §11) and implementation is
starting, milestone by milestone, using test-driven development. There is
no working release yet. Track progress via the [roadmap](#roadmap) below
and the [milestones](https://github.com/harveysandiego/receiptd/milestones)
on GitHub. See [VERSIONING.md](VERSIONING.md) and
[CHANGELOG.md](CHANGELOG.md) for how releases are numbered and tracked.

## Installation

> Not yet published. The instructions below describe the intended
> installation paths once Milestone 5 (Packaging) lands; until then, build
> from source.

### From source

```sh
git clone https://github.com/harveysandiego/receiptd.git
cd receiptd
make build
./bin/receiptd --config config.yml
```

### Pre-built binaries

Once releases begin, binaries for Linux/macOS/Windows (amd64/arm64) will
be published on the [Releases](https://github.com/harveysandiego/receiptd/releases)
page — see `.goreleaser.yml`.

### Docker

Planned for Milestone 5. Once available:

```sh
docker run -d \
  --name receiptd \
  -p 8080:8080 \
  -v receiptd-data:/var/lib/receiptd \
  -e RECEIPTD_AUTH_TOKEN=changeme \
  ghcr.io/harveysandiego/receiptd:latest
```

### Raspberry Pi

Receiptd is designed with Raspberry Pi in mind from day one: a single
CGO-free ARM64 static binary with no runtime dependencies, low memory
footprint, and no GPU/desktop requirement. Run it directly as a `systemd`
service or via Docker exactly as above — either works well on a Pi 3/4/5.

## CLI examples

```sh
# Print plain text
receipt "Milk, eggs, bread"

# Print a JSON Receipt document
receipt render receipt.json --print

# Render a Receipt to a local PNG without printing (no daemon required)
receipt render receipt.json --out preview.png

# Print via the weather template
receipt weather --location "London"

# Check a job's status
receipt jobs status <job-id>
```

## REST API examples

```sh
# Preview a Receipt as a PNG, without printing it
curl -X POST http://receiptd.local:8080/api/v1/preview \
  -H "Authorization: Bearer $RECEIPTD_TOKEN" \
  -H "Content-Type: application/json" \
  -d @receipt.json \
  -o preview.png

# Print a Receipt
curl -X POST http://receiptd.local:8080/api/v1/print \
  -H "Authorization: Bearer $RECEIPTD_TOKEN" \
  -H "Content-Type: application/json" \
  -d @receipt.json

# Check job status
curl http://receiptd.local:8080/api/v1/jobs/<job-id> \
  -H "Authorization: Bearer $RECEIPTD_TOKEN"

# Weather template — build, preview, or print directly
curl -X POST http://receiptd.local:8080/api/v1/templates/weather \
  -H "Authorization: Bearer $RECEIPTD_TOKEN"
curl -X POST http://receiptd.local:8080/api/v1/templates/weather/print \
  -H "Authorization: Bearer $RECEIPTD_TOKEN"
```

## Roadmap

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#10-roadmap) for full
detail on each milestone's scope.

- [ ] **Milestone 1** — Local render, no server (`receipt`, `apperr`,
      `render/layout`, `render/canvas`, offline CLI preview)
- [ ] **Milestone 2** — REST API, job queue, auth (fake printer sink)
- [ ] **Milestone 3** — Real printer support (ESC/POS encoding, network
      transport, remaining Element types) — first physical print
- [ ] **Milestone 4** — Web UI
- [ ] **Milestone 5** — Packaging (Docker, multi-arch, release pipeline)
- [ ] **Milestone 6** — First real template + provider (weather)

## Contributing

Contributions are welcome — bug reports, feature requests, and pull
requests alike. Start with [CONTRIBUTING.md](CONTRIBUTING.md) for
development setup, coding standards, and how to propose changes
(including architectural ones). Please also read the
[Code of Conduct](CODE_OF_CONDUCT.md).

## Acknowledgements

Receiptd's architecture was designed collaboratively with
[Claude Code](https://claude.com/claude-code), and implementation is
being carried out with AI assistance milestone by milestone. This is
disclosed deliberately rather than incidentally: every AI-generated
change still goes through the same human review, tests, and linting as
any other contribution (see [CLAUDE.md](CLAUDE.md)'s "AI-assisted
development" section for the full policy), and AI-assisted contributions
from anyone else are welcome under those same terms — see
[CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE) © 2026 harveysandiego
