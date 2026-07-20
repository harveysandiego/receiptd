# Receiptd

**Receipt Printer as a Service** — a self-hosted daemon that turns any
ESC/POS-compatible thermal receipt printer into an API-addressable
appliance on your home network.

<!-- All badge/clone/API URLs on this page assume this repo lives at
     github.com/harveysandiego/receiptd. If this repository is ever
     forked or renamed, update this page, go.mod, and .goreleaser.yml
     together in one commit — see the note at the top of go.mod. -->

[![CI](https://github.com/harveysandiego/receiptd/actions/workflows/ci.yml/badge.svg)](https://github.com/harveysandiego/receiptd/actions/workflows/ci.yml)
[![CodeQL](https://github.com/harveysandiego/receiptd/actions/workflows/codeql.yml/badge.svg)](https://github.com/harveysandiego/receiptd/actions/workflows/codeql.yml)
[![codecov](https://codecov.io/gh/harveysandiego/receiptd/graph/badge.svg)](https://codecov.io/gh/harveysandiego/receiptd)
[![Go Reference](https://pkg.go.dev/badge/github.com/harveysandiego/receiptd.svg)](https://pkg.go.dev/github.com/harveysandiego/receiptd)
[![Go Version](https://img.shields.io/github/go-mod/go-version/harveysandiego/receiptd)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

> **Status:** pre-alpha. Milestones 1–3 (local render, REST API + queue +
> auth, real ESC/POS printer support) and Milestone 5 (Docker packaging,
> multi-arch image publishing, release pipeline) are implemented and
> tested — Receiptd has printed to real hardware — but no tag has been
> pushed yet, so nothing is actually published to the Releases page or
> GHCR as of this writing. See [Current status](#current-status) before
> trying to run this.

---

## Screenshots

_Coming soon — a printed receipt example (Receiptd already prints to real
hardware) and a Web UI dashboard screenshot once Milestone 4 (Web UI)
lands. See the [roadmap](#roadmap)._

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
JSON ──>│         │   │          │   │          │
Markdown│ Receipt │──>│  Layout  │──>│  Canvas  │──> ESC/POS ──> Printer
Template│ (model) │   │(measure) │   │ (paint)  │        │
        └─────────┘   └──────────┘   └────┬─────┘        └─> Async job queue
                                           │                  (retry, persist)
                                           v
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
proceeding milestone by milestone, using test-driven development.
Milestones 1, 2, 3, and 5 are done: the `receipt`/`render` pipeline, the
REST API (`preview`, `print`, job status), the persistent job queue,
Bearer-token auth (on by default; Basic auth exists in `auth` too, ready
for Milestone 4's Web UI), a CLI that talks to the API, ESC/POS encoding,
network printer transport, every Milestone 3 Element type (Image, Asset,
QRCode, Barcode, Columns, Table, Feed, Cut), and multi-architecture
container images published automatically on tagged releases. Receiptd
has printed successfully to real hardware (an Epson TM-m30II). Milestone
4 (Web UI) is still outstanding. Track progress via the
[roadmap](#roadmap) below and the
[milestones](https://github.com/harveysandiego/receiptd/milestones) on
GitHub. See [VERSIONING.md](VERSIONING.md) and
[CHANGELOG.md](CHANGELOG.md) for how releases are numbered and tracked.

## Installation

> No tagged release has been cut yet, so the Releases page and the GHCR
> package below are both still empty — the instructions in this section
> describe what becomes available once the first tag (e.g. `v0.1.0`) is
> pushed and `.github/workflows/release.yml` runs. Until then, build from
> source or build the [Dockerfile](Dockerfile) locally.

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

A multi-stage [Dockerfile](Dockerfile) produces a static `CGO_ENABLED=0`
binary layered onto a distroless, non-root runtime image with no shell
and no package manager. On every tagged release,
[`.github/workflows/release.yml`](.github/workflows/release.yml) builds
and publishes it for **linux/amd64** and **linux/arm64** to the GitHub
Container Registry as a single multi-arch manifest — `docker pull` and
`docker run` transparently get the right architecture, no `--platform`
flag needed:

```sh
docker pull ghcr.io/harveysandiego/receiptd:latest
```

Available tags mirror the Git tag: `latest` (the newest stable release),
a full version (`0.2.0`), a minor (`0.2`), and a major (`0`) — pick
whichever gives the stability guarantee you want, per
[VERSIONING.md](VERSIONING.md). Pre-release tags (`0.2.0-rc1`) only get
the full-version tag, never `latest`/minor/major, so they can't be pulled
accidentally by a floating tag.

Every pull request also builds both architectures (without publishing)
via the same [reusable workflow](.github/workflows/docker-image.yml), so
a change that breaks the arm64 build is caught before it merges, not at
release time.

> **Maintainer note:** a package published to GHCR via the default
> `GITHUB_TOKEN` is **private** until its visibility is changed by hand
> (Package settings → Change visibility, or link it to this repository)
> — GitHub doesn't expose a workflow-file setting for this. Do that once
> after the first tagged release publishes the package, otherwise
> `docker pull` above fails with "unauthorized" for anyone who isn't a
> repo collaborator.

`receiptd` needs a config file and a writable data directory (see
[Configuration](#configuration-required-for-docker) below). Given a
`config.yaml` in the current directory:

```sh
docker run -d \
  --name receiptd \
  -p 8080:8080 \
  -v "$(pwd)/config.yaml:/etc/receiptd/config.yaml:ro" \
  -v receiptd-data:/var/lib/receiptd \
  -e RECEIPTD_AUTH_TOKEN=changeme \
  ghcr.io/harveysandiego/receiptd:latest
```

- `-v .../config.yaml:/etc/receiptd/config.yaml:ro` — mounts your config
  read-only at the path `receiptd`'s `--config` flag defaults to inside
  the image (see [`Dockerfile`](Dockerfile)'s `CMD`).
- `-v receiptd-data:/var/lib/receiptd` — persists the bbolt job queue and
  stored assets across container restarts; without this, both reset
  every time the container is recreated. The image runs as uid/gid
  `65532` (distroless's `nonroot`), which already owns this path inside
  the image — a named volume like this one adopts that ownership
  automatically, but a bind-mounted host directory won't unless you
  `chown` it to `65532:65532` first.
- `-e RECEIPTD_AUTH_TOKEN=changeme` — required because `auth.enabled`
  defaults to `true` (see [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)
  §7); pick a real secret, not the literal word above. Omit only if your
  config has an explicit `auth: { enabled: false }`.

To build the image locally instead (e.g. before the first tag exists, or
while iterating on the Dockerfile itself), swap the last line above for
an image built from a clone of this repo:

```sh
git clone https://github.com/harveysandiego/receiptd.git
cd receiptd
docker build -t receiptd:local .
# then substitute receiptd:local for the ghcr.io image above
```

#### Configuration required for Docker

`server.address` must bind `0.0.0.0`, not `127.0.0.1` or `localhost` —
otherwise the port mapped with `-p` above can't reach the process inside
the container's network namespace. `assets.path` and `queue.path` must
live under `/var/lib/receiptd` (the volume mount point above) or their
state won't survive a container restart:

```yaml
server:
  address: "0.0.0.0:8080"
auth:
  enabled: true # token comes from RECEIPTD_AUTH_TOKEN, set above
logging:
  level: info
  format: auto
assets:
  path: /var/lib/receiptd/assets
queue:
  store: bbolt
  path: /var/lib/receiptd/queue.db
  max_attempts: 3
  retry_backoff: 5s
printers:
  - name: front-desk
    transport: network
    address: 192.168.1.50:9100 # your printer's IP:port
    width_mm: 80
    dpi: 203
web:
  enabled: false
```

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) §7 for the full
config schema.

### Raspberry Pi

Receiptd is designed with Raspberry Pi in mind from day one: a single
CGO-free ARM64 static binary with no runtime dependencies, low memory
footprint, and no GPU/desktop requirement. Run it directly as a `systemd`
service or via Docker exactly as above — either works well on a Pi 3/4/5.
`docker pull ghcr.io/harveysandiego/receiptd:latest` on a Pi pulls the
`linux/arm64` image directly out of the published manifest, no
`--platform` flag or local cross-build needed. Building locally works
the same way in the other direction: the [`Dockerfile`](Dockerfile) has
no architecture-specific assumptions, so
`docker buildx build --platform linux/arm64 .` cross-builds an ARM64
image from an amd64 machine without changes.

## CLI examples

```sh
# Render a Receipt to a local PNG, offline — no daemon required
receipt render receipt.json --out preview.png

# Preview a Receipt as a PNG via a running receiptd
receipt preview receipt.json --out preview.png --printer front-desk

# Print a Receipt via a running receiptd
receipt print receipt.json --printer front-desk

# Check a job's status
receipt jobs <job-id>
```

`preview`, `print`, and `jobs` read the same `config.yaml` `receiptd`
loads (`--config`, default `/etc/receiptd/config.yaml`) to find the daemon
and its Bearer token; `render` is fully offline and ignores `--config`.
Plain-text printing and the weather template (`receipt weather ...`) are
planned for later milestones — see the [roadmap](#roadmap).

## REST API examples

Both `/preview` and `/print` take the same request body shape: a
`printer` name alongside the `receipt` itself — a preview is only ever
rendered relative to a specific printer's paper width (see
[docs/adr/0006](docs/adr/0006-preview-requires-printer-profile.md)).

```sh
# Preview a Receipt as a PNG, without printing it
curl -X POST http://receiptd.local:8080/api/v1/preview \
  -H "Authorization: Bearer $RECEIPTD_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"printer": "front-desk", "receipt": {"version": 1, "elements": [{"type": "text", "content": "Hello"}]}}' \
  -o preview.png

# Print a Receipt
curl -X POST http://receiptd.local:8080/api/v1/print \
  -H "Authorization: Bearer $RECEIPTD_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"printer": "front-desk", "receipt": {"version": 1, "elements": [{"type": "text", "content": "Hello"}]}}'

# Check job status
curl http://receiptd.local:8080/api/v1/jobs/<job-id> \
  -H "Authorization: Bearer $RECEIPTD_TOKEN"
```

Template-backed convenience endpoints (e.g. `/api/v1/templates/weather`)
are planned for Milestone 6 and don't exist yet — see the
[roadmap](#roadmap).

## Roadmap

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#10-roadmap) for full
detail on each milestone's scope.

- [x] **Milestone 1** — Local render, no server (`receipt`, `apperr`,
      `render/layout`, `render/canvas`, offline CLI preview)
- [x] **Milestone 2** — REST API, job queue, auth (fake printer sink)
- [x] **Milestone 3** — Real printer support (ESC/POS encoding, network
      transport, remaining Element types) — first physical print
- [ ] **Milestone 4** — Web UI
- [x] **Milestone 5** — Packaging (Docker, multi-arch, release pipeline)
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
