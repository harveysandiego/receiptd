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
> tested — Receiptd has printed to real hardware, and
> [v0.4.0](https://github.com/harveysandiego/receiptd/releases/tag/v0.4.0)
> is tagged and published, including multi-arch images at
> `ghcr.io/harveysandiego/receiptd`. See [Current status](#current-status)
> before trying to run this.

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
- **One document model, one rendering pipeline.** A `Receipt` — today,
  built from raw JSON via the API or CLI — is rendered by exactly one
  pipeline. No parallel code paths to keep in sync. Server-side templates
  (Milestone 6, not yet implemented — see [Roadmap](#roadmap)) are
  designed to build that same `Receipt` structure when they land, rather
  than a separate rendering path of their own.
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
JSON ──>│ Receipt │──>│  Layout  │──>│  Canvas  │──> ESC/POS ──> Printer
        │ (model) │   │(measure) │   │ (paint)  │        │
        └─────────┘   └──────────┘   └────┬─────┘        └─> Async job queue
                                           │                  (retry, persist)
                                           v
                                     PNG preview
```

A raw JSON `Receipt` — today, the only input source — is an ordered list
of typed `Element`s (text, headings, images, QR codes, tables, and so on),
deliberately similar in spirit to Slack's Block Kit. A server-side
template producing that same `Receipt` structure (e.g. "today's weather")
is planned for Milestone 6 — see [Roadmap](#roadmap) — but doesn't exist
yet. That document is run through one shared pipeline:

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
- **Element-based Receipt model**: text, headings, dividers, spacers,
  images, named assets, QR codes, barcodes, columns, tables, lists
  (bulleted, numbered, checkbox), feed, cut
- **PNG preview** before anything hits paper
- **Async, persistent print queue** with retry/backoff for transient
  printer failures, one independent worker per configured printer so a
  slow or offline printer never blocks another
- **Idempotent print requests** via an optional `Idempotency-Key` header,
  so a retried request never prints twice
- **Multi-copy printing** via `Receipt.copies` — one render/encode, sent
  to the printer that many times
- **Graceful shutdown** on `SIGTERM`/`SIGINT`, letting an in-flight print
  finish rather than cutting it off mid-stream
- **Startup crash recovery** for any Job left `running` by a previous
  crash or unclean death
- **Named asset storage** for logos and reusable images
- **Optional bearer-token / basic auth**, on by default
- Single static binary — Linux/macOS/Windows, amd64/arm64, no CGO

Planned but not yet implemented — see [Roadmap](#roadmap): a **Web UI**
(Milestone 4) and **server-side templates**, e.g. a daily weather receipt
(Milestone 6).

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

### From source

```sh
git clone https://github.com/harveysandiego/receiptd.git
cd receiptd
make build
./bin/receiptd --config config.yml
```

### Pre-built binaries

Binaries for Linux/macOS/Windows (amd64/arm64) are published on the
[Releases](https://github.com/harveysandiego/receiptd/releases) page —
see `.goreleaser.yml`.

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
a full version (`0.1.0`), a minor (`0.1`), and a major (`0`) — pick
whichever gives the stability guarantee you want, per
[VERSIONING.md](VERSIONING.md). Pre-release tags (`0.2.0-rc1`) only get
the full-version tag, never `latest`/minor/major, so they can't be pulled
accidentally by a floating tag.

Every pull request also builds both architectures (without publishing)
via the same [reusable workflow](.github/workflows/docker-image.yml), so
a change that breaks the arm64 build is caught before it merges, not at
release time.

> **Maintainer note:** GHCR packages published via the default
> `GITHUB_TOKEN` from a public repository inherit that repository's
> public visibility automatically — verified for this package by pulling
> its manifest anonymously (no login) right after the first publish.
> There's no dedicated workflow-file setting for this either way, so if a
> package ever comes up private unexpectedly, check Package settings →
> Change visibility rather than assuming a workflow change is needed.

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
    model: epson-tm-m30ii # or use profile: for hardware not in the catalogue
web:
  enabled: false
```

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) §7 for the full
config schema, including the `profile:` alternative to `model:` for
printers not yet in the built-in catalogue.

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

### Graceful shutdown and restart grace periods

On `SIGTERM` or `SIGINT`, `receiptd` stops accepting new HTTP requests and
new queue Jobs immediately, but lets anything already in flight — in
particular, a print Job already streaming raster bytes to a printer —
finish naturally rather than cutting it off mid-transmission, which could
otherwise leave a printer holding a partially fed or garbled receipt. This
drain is bounded by a fixed internal deadline, currently **30 seconds**
(see [ADR-0018](docs/adr/0018-graceful-shutdown.md)): if something is
still in flight when the deadline is reached, or a second `SIGTERM`/
`SIGINT` arrives while already draining, `receiptd` exits immediately
regardless.

Because of that internal deadline, **the external grace period your
orchestrator waits before sending `SIGKILL` must be set comfortably longer
than 30 seconds**, or the orchestrator's own kill races — and likely
preempts — this clean shutdown, silently turning every restart back into
an abrupt kill:

- **Docker**: the default 10-second grace period is too short — raise it,
  e.g. `docker stop --time 40 receiptd`, or `stop_grace_period: 40s` in a
  Compose file.
- **systemd**: set `TimeoutStopSec=40s` (or higher) in the unit file;
  systemd's own default (90s) is already safe, but setting it explicitly
  is worth doing rather than relying on the default going unnoticed.

If the internal 30-second deadline is ever retuned, the recommended grace
period above should move with it — see the ADR for the full reasoning.

### Reverse proxy and TLS

Receiptd speaks plain HTTP only — it never terminates TLS itself, and has
no certificate configuration of any kind (see
[ADR-0021](docs/adr/0021-transport-security-via-reverse-proxy.md)). The
supported way to expose it beyond a trusted network is behind a reverse
proxy (Caddy, nginx, Traefik, SWAG, HAProxy, Apache, ...) that owns
certificate issuance/renewal and the public-facing edge. Receiptd itself
is only ever responsible for application-layer authentication
(`auth.enabled`, Bearer/basic) — transport security is the proxy's job,
not something to reimplement here.

- **Direct Internet exposure of `receiptd` with no reverse proxy in front
  of it is not a supported deployment model.**
- On a fully trusted local network (a homelab LAN, an isolated Docker
  network), plain HTTP with no reverse proxy at all is a legitimate
  choice — transport encryption was never needed there in the first
  place, not something Receiptd is securing on your behalf either way.

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

# Print a Receipt idempotently — retrying with the same key (e.g. after a
# timeout) returns the original job_id instead of printing a second time
curl -X POST http://receiptd.local:8080/api/v1/print \
  -H "Authorization: Bearer $RECEIPTD_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: 2026-07-23-front-desk-order-42" \
  -d '{"printer": "front-desk", "receipt": {"version": 1, "elements": [{"type": "text", "content": "Hello"}]}}'

# Check job status
curl http://receiptd.local:8080/api/v1/jobs/<job-id> \
  -H "Authorization: Bearer $RECEIPTD_TOKEN"
```

`/print` accepts an optional `Idempotency-Key` header (unrelated to the
`printer` field): supplying the same key on a retry returns the original
request's `job_id` instead of enqueuing a second print, for 24 hours from
the first request. Omitting it (every client that predates this feature)
enqueues a new Job every time, exactly as before — see
[ADR-0020](docs/adr/0020-idempotent-print-requests.md).

A `Receipt`'s top-level `copies` field controls how many physical copies
one Job prints: the render → layout → encode pipeline runs once, and the
resulting ESC/POS bytes are sent to the printer `copies` times. Omitting
`copies` (or setting it to `0`) prints exactly one copy; a negative value,
or one over 100, is rejected at validation time. A transient send failure
partway through fails the whole Job for the queue to retry as one unit
(docs/adr/0019-retry-pipeline-granularity.md), so a retry after a partial
copy run can produce duplicate physical copies — expected, not a bug.

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
