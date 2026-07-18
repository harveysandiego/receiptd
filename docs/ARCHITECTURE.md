# Receiptd — Architecture

Status: **frozen for implementation** (see §11). This revision incorporates
eight refinements requested after the previous draft: Profile/Connection
split, behavior-based Element interface, restored Asset element, template
convenience endpoints, a Font interface, an explicit error philosophy, an
aggressive package-count reduction, and moving auth into Milestone 2.

---

## 1. Package layout

Organized by responsibility, in dependency order (each package may depend on
anything above it in this list; nothing may depend downward). Compared to the
previous draft, this revision **removes five packages** by folding
single-implementation code into the package that actually uses it, and adds
one (`apperr`) because it's genuinely foundational — see §11 for the
full before/after reasoning.

```
internal/
  apperr/                Typed error taxonomy (Kind + wrapping). Zero
                         internal dependencies — this is why it's a
                         separate package: almost everything below needs
                         it, and it must not pull in any domain package.

  receipt/               Domain model: Element interface (Validate() error),
                         concrete Element types, Receipt struct, JSON
                         polymorphism via registry. Depends on apperr/.

  printer/               Profile (capabilities), Connection (transport
                         details), Printer interface (Send/Status/Close),
                         and the network transport implementation.
                         Depends on apperr/. See below for why Profile,
                         Connection, and the one existing transport all
                         live in one package for now.

  render/
    layout/               Receipt + Profile -> Document (positioned draw
                         instructions, pure data, no pixels). Owns the
                         Font interface and its one embedded-monospace
                         implementation (folded in — see §11). Depends on
                         receipt/, printer/ (Profile only), assets/
                         (to resolve Asset elements), apperr/.
    canvas/                Document -> Canvas (monochrome bitmap) via
                         Paint(). Also owns EncodePNG() directly on
                         Canvas — PNG encoding is a couple of lines
                         around image/png and doesn't need an
                         "Output interface" to justify a package (§11).
                         Depends on render/layout/, apperr/.
    escpos/                Canvas -> ESC/POS bytes (raster commands +
                         init/feed/cut), Profile-aware chunking. Kept
                         separate from canvas/ because it's real,
                         growable protocol complexity with its own
                         golden-byte test suite. Depends on render/canvas/,
                         printer/ (Profile only), apperr/.

  queue/                 Job, JobState, Store interface, Queue/Worker.
                         Both Store implementations (bbolt-backed and
                         in-memory) live here as plain files — they don't
                         need their own packages (§11). Depends on
                         receipt/, apperr/.

  templates/              Template interface + registry (Register/Get/List).
                         Depends on receipt/, apperr/. Concrete templates
                         (e.g. templates/weather) arrive as their own
                         packages when actually built (Milestone 6+).

  providers/               Not a package itself — a directory grouping
                         only. Each domain (providers/weather/...) is an
                         independent leaf package defining the interface
                         shape that fits its own data; there is no shared
                         "Provider" interface to hold. Arrives with
                         Milestone 6.

  assets/                 Store interface (Get/Put/Delete/List) +
                         filesystem implementation. Depends on apperr/.

  config/                 YAML config struct + loader + validation.
                         Depends on printer/ (Profile+Connection shape),
                         queue/ (store choice), assets/ (path), apperr/.

  auth/                   Bearer-token (API/CLI) and Basic-Auth (browser)
                         middleware, one shared token check. Kept separate
                         from api/ and webui/ specifically because both of
                         those depend on it and must not depend on each
                         other. Depends on config/, apperr/.

  app/                    Service/use-case layer: wires layout, canvas,
                         escpos, printer, queue, templates, providers,
                         assets together; implements queue.Processor.
                         The only package api/ and webui/ call into for
                         business logic. Depends on everything above.

  api/                    REST handlers (JSON), versioned under /api/v1.
                         Depends on app/, auth/.

  webui/                  HTML handlers (html/template + embedded static
                         assets), same Service underneath. Depends on
                         app/, auth/.

cmd/
  receiptd/               Composition root: load config, construct
                         concrete Connections -> Printer instances,
                         register templates/providers via blank imports,
                         wire app.Service, start queue worker + HTTP
                         server. This is the only place that ever
                         constructs a printer.Connection.
  receipt/                 CLI (Cobra). Talks to receiptd's REST API.
                         Depends on receipt/ directly for ad-hoc local
                         commands (e.g. `receipt "Hello"` builds a
                         Receipt client-side before POSTing it), and on
                         render/layout+canvas for an offline
                         `receipt render x.json --out preview.png` path
                         that doesn't need a running daemon.

web/                      Embedded static assets/templates for webui/.
docs/
```

### Why Profile, Connection, and Printer all live in one `printer` package

Per your request, capabilities and transport are now two separate **types**:

```go
type Profile struct {
    WidthDots          int
    DPI                int
    MarginLeftDots     int
    MarginRightDots    int
    SupportsCut        bool
    SupportsPartialCut bool
    DefaultCut         string // "full" | "partial"
    MaxImageHeightDots int    // 0 = no chunking needed
}

type Connection struct {
    Transport string // "network" (v0.1); "usb" | "bluetooth" | "serial" later
    Address   string // network: host:port
    Device    string // usb: device path (future)
    MAC       string // bluetooth: MAC address (future)
}
```

The separation you want — "the renderer only knows capabilities, the
transport only knows how to reach the printer" — is enforced **structurally,
not just by package boundary**: `render/layout` and `render/canvas` only ever
receive a `Profile` value; they never see a `Connection`. `cmd/receiptd` is
the *only* code in the entire system that constructs a `Connection` and
turns it into a live `Printer` via `network.New(conn)`. `app.Service` holds
`map[string]printer.Profile` (for rendering) and `map[string]printer.Printer`
(already-constructed transport instances, for sending) — it receives neither
raw `Connection` values nor the network dial logic. That's a stronger
guarantee than "don't import the wrong package" would give you, since no
function signature anywhere in `render/*` even accepts a `Connection`.

Given that, `Connection` living in the same Go package as `Profile` doesn't
weaken the separation — Go packages group types, they don't restrict which
of a package's exported types get used where. Putting the (currently single)
network transport implementation in this same package rather than a
`printer/network` subpackage follows the same "evolve packages over time"
principle as §11: when a second transport (USB, Bluetooth) actually gets
built, that's the natural point to decide whether splitting into
subpackages earns its keep — likely yes then, since e.g. USB may pull in a
platform-specific dependency you won't want polluting this package's import
graph. Not worth doing preemptively for one implementation.

---

## 2. Core interfaces

```go
// apperr
type Kind int

const (
    KindUnknown Kind = iota
    KindValidation
    KindNotFound
    KindTransient  // retryable: printer offline, transport timeout
    KindPermanent  // not retryable: renderer failure, bad state
    KindUnauthorized
)

type Error struct {
    Kind Kind
    Op   string // "layout.Build", "assets.Get", etc.
    Err  error
}

func (e *Error) Error() string { /* ... */ }
func (e *Error) Unwrap() error { return e.Err }

func Wrap(kind Kind, op string, err error) *Error
func Is(err error, kind Kind) bool // errors.As + Kind match
```

```go
// receipt
type Element interface {
    Validate() error
}

type Receipt struct {
    Version  int       `json:"version"`
    Copies   int       `json:"copies,omitempty"`
    Elements []Element `json:"elements"`
}

func (r Receipt) Validate() error // aggregates each Element's Validate()
                                   // via errors.Join, wrapped as apperr.KindValidation
```

No `Kind()` string method on the interface — the JSON `type` discriminator is
a serialization concern handled by the registry during unmarshal, and
`layout` dispatches on the *Go* concrete type via a type switch (idiomatic,
uses the compiler's own type information — no need for a hand-rolled
identity method to do the same job twice).

```go
// printer
type Printer interface {
    Send(ctx context.Context, data []byte) error
    Status(ctx context.Context) (Status, error)
    Close() error
}

type Status struct {
    Online bool
    Detail string
}
```

```go
// render/layout
type Font interface {
    Measure(s string) int                          // width in dots
    LineHeight() int                                // dots
    Glyph(r rune) (bitmap GlyphBitmap, advance int)
}

type GlyphBitmap struct {
    Width, Height int
    Bits          []byte // 1bpp packed rows
}

type Document struct {
    WidthDots  int
    HeightDots int
    Font       Font    // carried on the Document so layout and canvas
                        // are guaranteed to use the same Font instance —
                        // measurements and painted glyphs can never
                        // silently disagree
    Blocks     []Block
}

func Build(ctx context.Context, r receipt.Receipt, p printer.Profile, a assets.Store) (Document, error)
```

`Font` is the one interface in this design with a single implementation and
no immediate second one planned — a deliberate exception to "don't design an
interface before a second implementation exists," made because you asked for
it explicitly as future-proofing and because it's genuinely narrow (three
methods) and cheap either way: `layout` and `canvas` need a clean boundary
around "the font" regardless, so the interface costs essentially nothing
over a concrete struct while keeping a future proportional/TTF font swap
from touching either package's internals.

```go
// render/canvas
type Canvas struct { /* 1-bit bitmap, Document.WidthDots x HeightDots */ }

func Paint(doc layout.Document) (*Canvas, error)
func (c *Canvas) EncodePNG() ([]byte, error)
```

```go
// render/escpos
func Encode(c *canvas.Canvas, p printer.Profile) ([]byte, error)
```

No `Output` interface in v0.1 — see §11. `app.Service` calls
`canvas.EncodePNG()` for preview and `escpos.Encode()` for print, directly.

```go
// queue
type JobState string

const (
    JobPending, JobRunning, JobDone, JobFailed, JobCancelled JobState = "pending", "running", "done", "failed", "cancelled"
)

type Job struct {
    ID          string
    PrinterName string
    Receipt     receipt.Receipt
    State       JobState
    Attempts    int
    LastError   string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type Store interface {
    Save(ctx context.Context, j *Job) error
    Get(ctx context.Context, id string) (*Job, error)
    List(ctx context.Context, f Filter) ([]*Job, error)
}

func NewBoltStore(path string) (Store, error) // store_bolt.go
func NewMemoryStore() Store                    // store_memory.go

type Processor interface {
    Process(ctx context.Context, j *Job) error
}

type Queue struct { /* Store + Processor, retry/backoff (apperr.KindTransient only), cancellation */ }

func (q *Queue) Enqueue(ctx context.Context, j *Job) error
func (q *Queue) Cancel(ctx context.Context, id string) error
```

`Queue` itself stays a concrete struct, not an interface — there is exactly
one implementation and no reason to swap it, unlike `Store` where bolt vs.
memory is a real, present choice. Not every seam needs to be an interface;
this one doesn't vary.

```go
// templates
type Params map[string]string

type Template interface {
    Name() string
    Build(ctx context.Context, p Params) (receipt.Receipt, error)
}

func Register(t Template)
func Get(name string) (Template, bool)
func List() []string
```

```go
// providers/weather (example — each domain defines its own shape)
type Provider interface {
    Current(ctx context.Context, location string) (Data, error)
}
```

```go
// assets
type Store interface {
    Get(ctx context.Context, name string) ([]byte, error)
    Put(ctx context.Context, name string, data []byte) error
    Delete(ctx context.Context, name string) error
    List(ctx context.Context) ([]string, error)
}
```

```go
// app
type Service struct {
    Layout    layoutFunc // render/layout.Build, injected for testability
    Profiles  map[string]printer.Profile
    Printers  map[string]printer.Printer
    Queue     *queue.Queue
    Assets    assets.Store
    Templates *templates.Registry
}

func (s *Service) Print(ctx context.Context, r receipt.Receipt, printerName string) (jobID string, err error)
func (s *Service) Preview(ctx context.Context, r receipt.Receipt) ([]byte, error) // PNG only for now
func (s *Service) RunTemplate(ctx context.Context, name string, p templates.Params) (receipt.Receipt, error)
func (s *Service) JobStatus(ctx context.Context, id string) (*queue.Job, error)
func (s *Service) Process(ctx context.Context, j *queue.Job) error // satisfies queue.Processor
```

---

## 3. Receipt model

### Element types (v0.1)

| Type       | Key fields                                                        |
|------------|--------------------------------------------------------------------|
| `text`     | `content`, `align`, `bold`, `size`                                  |
| `heading`  | `content` (implies bold + large)                                    |
| `divider`  | `style` (solid/dashed, optional)                                    |
| `spacer`   | `height` (dots)                                                     |
| `image`    | `data` (inline base64) — always bytes the client already has        |
| `asset`    | `name`, `width`, `align` — resolved by name via `assets.Store` at layout time |
| `qrcode`   | `content`, `size`, `error_correction`                                |
| `barcode`  | `content`, `symbology`, `height`, `show_text`                        |
| `columns`  | `columns: []{ weight int, elements: []Element }` — recursive        |
| `table`    | `headers: []string`, `rows: [][]string` — flat, no nested Elements  |
| `feed`     | `lines`                                                              |
| `cut`      | `mode` (full/partial) — optional; renderer supplies `Profile.DefaultCut` if absent |

### Image vs. Asset — restored as separate types

I'm persuaded — this reverses the merge I suggested last round. `Image`
means "here are the bytes"; `Asset` means "look this name up." They render
identically today (both end up as a bitmap in the `Document`), but the
*resolution step* is exactly where future flexibility belongs: an Asset
could later resolve through an SVG rasterizer, a generated/templated image,
a company logo with theme-aware variants, or a cache — none of which needs
the Receipt/Element JSON schema to change, only what `assets.Store.Get`
does internally. Collapsing them into one type would have baked "images are
always literal bytes" into the public API contract, foreclosing that.

Concretely: `render/layout.Build` resolves `Image.Data` by decoding it
directly (no I/O), and resolves `Asset.Name` via `assets.Store.Get` (I/O,
can fail with `apperr.KindNotFound`). Both end up as the same kind of
already-decoded pixel content on a `Block` by the time `layout.Build`
returns — `canvas.Paint` never distinguishes between them, only `layout`
does.

### JSON representation

Unchanged — discriminated union via `type`, same shape as Slack's Block Kit:

```json
{
  "version": 1,
  "copies": 1,
  "elements": [
    { "type": "heading", "content": "Shopping List" },
    { "type": "divider" },
    { "type": "text", "content": "Milk" },
    { "type": "asset", "name": "logo.png" },
    { "type": "qrcode", "content": "https://example.com/list/42" }
  ]
}
```

### Polymorphism and validation in Go

Same registry-based unmarshal as before (`type` string -> `func() Element`
factory, registered via `init()` in each element's own file — new types
touch exactly one new file). What's new: `Element` now exposes
`Validate() error` instead of `Kind()`. `Receipt.Validate()` calls
`el.Validate()` on every element (recursing into `Columns`), aggregates
failures with `errors.Join`, and wraps the result as
`apperr.Wrap(apperr.KindValidation, "receipt.Validate", err)`.

Each concrete type's `Validate()` checks only its own local invariants —
e.g. `Image.Validate()` checks `Data` decodes as a supported image format;
`Asset.Validate()` checks `Name` is non-empty (it does **not** check the
asset exists — that requires I/O and happens later, in `layout.Build`,
where an `apperr.KindNotFound` is the natural result if it's missing).
Validation is deliberately fast and local; existence-checking is deliberately
deferred to the stage that already does I/O.

---

## 4. Rendering architecture — walking `receipt weather` end to end

1. **CLI**: `receipt weather` calls `POST /api/v1/templates/weather`.
2. **API** → `app.Service.RunTemplate(ctx, "weather", params)`.
3. **app.Service** looks up `"weather"` in the `templates` registry, calls
   `Template.Build(ctx, params)`.
4. **templates/weather** holds an injected `providers/weather.Provider`
   (chosen by config at startup), calls `Provider.Current(ctx, location)`,
   builds a `receipt.Receipt`.
5. **API** returns that Receipt as JSON. Nothing has been rendered or
   printed yet.
6. Depending on which of the three template endpoints was called:
   - `POST /api/v1/templates/weather` → stop here, return the Receipt JSON.
   - `POST /api/v1/templates/weather/preview` → `app.Service` takes the
     Receipt it just built and calls `s.Preview(ctx, receipt)` — the same
     method the raw `/api/v1/preview` endpoint uses — and returns PNG bytes.
   - `POST /api/v1/templates/weather/print` → `app.Service` takes the same
     built Receipt and calls `s.Print(ctx, receipt, printerName)` — the same
     method `/api/v1/print` uses — and returns a job ID.

   All three convenience endpoints are thin composition over
   `RunTemplate` + `Preview`/`Print`; none of them duplicate rendering or
   queueing logic; `Template.Build` still only ever builds.

7. Taking the `/print` path: `app.Service.Print` validates the Receipt,
   resolves `printer.Profile` for the target printer, constructs a
   `queue.Job{Receipt, PrinterName, State: Pending}`, calls
   `Queue.Enqueue`. Returns the job ID immediately — the printer hasn't
   been touched.
8. **Queue worker** (background goroutine, independent of the HTTP request
   that already returned) picks up the job, marks it `Running`, calls
   `app.Service.Process(ctx, job)`:
   a. Resolve `printer.Profile` for `job.PrinterName`.
   b. `layout.Build(ctx, job.Receipt, profile, assetsStore)` — walks
      Elements, measures text via `Font.Measure`, wraps lines, sizes
      columns/table cells, resolves any `Image`/`Asset` elements into
      decoded pixel content, computes vertical positions. Produces a fully
      resolved `Document` — **after this call, nothing downstream ever
      touches `receipt.Receipt`, `assets.Store`, or any provider again.**
      `layout` is the only stage that talks to the outside world.
   c. `canvas.Paint(document)` — paints every Block onto a monochrome
      bitmap using `document.Font.Glyph()` for text, `go-qrcode` /
      `boombuler/barcode`-generated bitmaps for QR/barcode Blocks, decoded
      image pixels for Image/Asset Blocks, lines for dividers.
   d. If the Receipt didn't end with an explicit `cut`, `escpos.Encode`
      appends `profile.DefaultCut` behavior.
   e. `escpos.Encode(canvas, profile)` — init bytes, raster commands
      (chunked only if `profile.MaxImageHeightDots` requires it — see §11
      on not building chunking logic before it's known to be needed),
      feed, cut.
   f. `printer.Send(ctx, bytes)` on the already-constructed `Printer`
      instance for `job.PrinterName` (built once at startup from its
      `Connection` by `cmd/receiptd` — `app.Service` never sees the
      `Connection` itself).
   g. On success: Job → `Done`. On `apperr.KindTransient` (printer offline,
      timeout): retry per policy, then `Failed`. On any other error kind:
      `Failed` immediately, no retry.

---

## 5. Error handling

Idiomatic Go precedent: this is the same shape as Rob Pike's Upspin `errors`
package and the general "typed errors via a small Kind enum + Op + wrapped
cause" pattern — deliberately not inventing something novel.

```go
type Kind int

const (
    KindUnknown Kind = iota
    KindValidation    // bad Receipt / bad request — 400, no retry
    KindNotFound       // asset/template/job not found — 404, no retry
    KindTransient      // printer offline, transport timeout — 502/503, retry
    KindPermanent       // renderer failure, unrecoverable — 500, no retry
    KindUnauthorized     // missing/bad token — 401, no retry
)

type Error struct {
    Kind Kind
    Op   string
    Err  error
}
```

Every package that returns a domain-meaningful error wraps it with
`apperr.Wrap(kind, op, err)` at the point it's first known — a transport
dial failure in `printer/network` is wrapped `KindTransient` right there; a
missing asset in `assets` is wrapped `KindNotFound` right there. Callers
never need to pattern-match on error strings.

This maps cleanly onto every consumer you listed:

- **API**: a small `apperr.Kind -> http.StatusCode` table in `api`
  (`KindValidation→400, KindNotFound→404, KindUnauthorized→401,
  KindTransient→503, KindPermanent→500`).
- **Queue retry**: `apperr.Is(err, apperr.KindTransient)` is the *only*
  condition that consumes retry budget (formalizes the transient-vs-permanent
  distinction from the previous revision).
- **Logs**: `logger.With("kind", err.Kind, "op", err.Op)` via
  `charmbracelet/log` — structured, not string-parsed.
- **Tests**: assert `apperr.Is(err, apperr.KindNotFound)`, never assert on
  `err.Error()` message text — robust to wording changes.

---

## 6. Job system

Unchanged from the previous revision, now stated in terms of `apperr`:
states `pending -> running -> {done|failed}` plus `pending -> cancelled`
(cancelling a `running` job returns an "already in progress" error, it
can't un-print). Retries: bounded attempts (default 3), exponential
backoff (default 5s base), gated strictly on `apperr.KindTransient`.
Persistence: `bbolt` by default (`queue.NewBoltStore`), in-memory available
as an explicit opt-out (`queue.NewMemoryStore`) — both now plain files in
the `queue` package rather than subpackages (§11).

---

## 7. Configuration

```yaml
server:
  address: ":8080"

auth:
  enabled: true
  token_file: /etc/receiptd/token
  # env override: RECEIPTD_AUTH_TOKEN

logging:
  level: info
  format: auto        # auto | text | json

assets:
  path: /var/lib/receiptd/assets

queue:
  store: bbolt         # memory | bbolt
  path: /var/lib/receiptd/queue.db
  max_attempts: 3
  retry_backoff: 5s

printers:
  - name: default
    # -- Connection fields --
    transport: network
    address: 192.168.1.50:9100
    # -- Profile fields --
    width_mm: 80
    dpi: 203
    margins_dots: { left: 0, right: 0 }
    supports_cut: true
    supports_partial_cut: true
    default_cut: partial
    max_image_height_dots: 0   # 0 = no chunking; revisit once tested on real hardware

providers:
  weather:
    driver: openweather
    api_key_env: OPENWEATHER_API_KEY

web:
  enabled: true
```

Each YAML `printers[]` entry stays a single flat block for the user's
convenience — `config` is the one place that splits it into the
`printer.Profile` and `printer.Connection` Go values, matching the "one
config file, two internal types" trade-off described in §1.

`auth.enabled` defaults to `true` if the `auth:` block is omitted from the
document entirely, or present without an `enabled:` key — the API must
never exist unsecured by omission. Only an explicit `enabled: false` opts
out; `config.Config`'s custom `UnmarshalYAML` implements this by
pre-populating the default before decoding, the standard yaml.v3 idiom for
telling "absent" apart from "explicitly zero".

---

## 8. Extension model

All compile-time registration + blank import, never Go's `plugin` package
(Linux-only, fragile across compiler versions, breaks the
single-static-binary/ARM64 story):

- **New Element type**: one new file in `receipt`, self-registers.
- **New Template**: new package under `templates/`, registers via `init()`,
  one blank-import line in `cmd/receiptd`.
- **New Provider**: new package under `providers/<domain>/<name>`,
  implementing that domain's interface, selected by `driver:` in config.
- **New Output format** (PDF, ANSI preview): this is the point where the
  `Output` interface actually gets introduced (deferred in v0.1 — see §11)
  — extract it from the existing `EncodePNG`/`escpos.Encode` call sites
  once a second format is real, not before.
- **New printer transport** (USB/Bluetooth/Serial): implements
  `printer.Printer`; also the point at which splitting `printer` into
  subpackages (per §1) is worth revisiting.
- **New Font implementation**: implements the `Font` interface in
  `render/layout`; swapped in at `cmd/receiptd` wiring.

---

## 9. Testing strategy

Unchanged in shape from the previous revision, with error assertions now
explicit:

| Package             | Approach |
|----------------------|----------|
| `receipt`             | JSON round-trip per Element type; `Validate()` returns `apperr.KindValidation` for bad input. |
| `render/layout`        | Pure data assertions on `Document` (no pixels): wrap counts, column widths, image scale factors, total height. Missing asset returns `apperr.KindNotFound`. |
| `render/canvas`        | Golden image tests (`testdata/`, `-update` flag). |
| `render/escpos`        | Golden byte tests — exact byte sequence for a known Canvas. |
| `queue`               | Fake `Store` + fake `Processor` (scriptable failures via `apperr.Kind`): assert `KindTransient` retries and `KindPermanent`/`KindValidation` fail immediately with no retry; state transitions; cancellation; `-race`. |
| `printer` (network)    | Local `net.Listen` fake server — no hardware in CI. |
| `api`                 | `httptest` against a fake `app.Service`; assert the `apperr.Kind -> HTTP status` table. |
| End-to-end            | One in-process test: real `receipt -> layout -> canvas -> escpos -> fake TCP server`. |
| Hardware (manual)      | `-tags hardware` gated / documented checklist against the real Epson TM-m30II. Not in default CI. |

CI: `go test ./... -race -cover`, `golangci-lint`.

---

## 10. Roadmap

**Milestone 1 — Local render, no server**
`receipt` model + `Validate()`, `apperr`, `render/layout`, `render/canvas`
(with `EncodePNG`). CLI: `receipt render receipt.json --out preview.png`.
Element types: Text, Heading, Divider, Spacer to start.

**Milestone 2 — API + queue + auth, fake printer**
`config`, `auth` (bearer + basic, wired in from the start — the API never
exists unsecured, per your request), `app.Service`, `/api/v1/preview`,
`/api/v1/print`, `bbolt`-backed `queue`, job states,
`GET /api/v1/jobs/{id}`. `Process` writes to a log file instead of a real
printer. CLI switches to calling the API.

**Milestone 3 — Real printer**
`render/escpos`, `printer.Profile`/`Connection`/network transport. Remaining
Element types: Image, Asset, QRCode, Barcode, Columns, Table, Feed, Cut.
First receipt physically prints on the TM-m30II. Chunking logic in
`escpos.Encode` starts as a no-op and only gets built out if hardware
testing here actually shows the printer needs it (§11).

**Milestone 4 — Web UI**
`webui`, asset management endpoints. Auth already exists from Milestone 2;
this just uses it. Printer status, quick actions, preview, text printing,
image upload, asset management, printer settings.

**Milestone 5 — Packaging**
Multi-stage Dockerfile (`CGO_ENABLED=0`), `buildx` multi-arch (amd64/arm64),
release pipeline.

**Milestone 6 — First real Template + Provider**
`templates`, `providers/weather` (+ one concrete implementation, e.g.
OpenWeather), all three convenience endpoints
(`/api/v1/templates/weather`, `/preview`, `/print`) end to end. Validates
the registration/DI extension model in practice before Shopping/Homelab/Plex.

---

## 11. Final architecture review

You asked me to review this as though auditing a mature open-source Go
project, specifically for unnecessary abstraction, unnecessary interfaces,
unnecessary packages, circular dependencies, idiom violations, premature
optimization, and future maintenance concerns.

### Package count: before/after

Previous draft had ~19 packages once every subpackage was counted
(`render/font`, `render/output/{escpos,png,pdf,ansi}`,
`printer/network`, `queue/{boltstore,memstore}`, plus the core set). This
revision has **13 real packages** for everything through Milestone 6:
`apperr`, `receipt`, `printer`, `render/layout`, `render/canvas`,
`render/escpos`, `queue`, `templates`, `assets`, `config`, `auth`, `app`,
`api`, `webui`. Five packages were folded because they had exactly one
implementation and no near-term second one:

- `render/font` → folded into `render/layout` (the interface exists per
  your request, but doesn't need its own package until a second Font
  implementation exists).
- `render/output/png` → folded into `render/canvas` as `EncodePNG()`.
- `render/output/escpos` → simplified to `render/escpos` (dropped the
  `output` grouping level; nothing else lived there).
- `queue/boltstore`, `queue/memstore` → folded into `queue` as plain files.
- `printer/network` → folded into `printer` (see §1's detailed reasoning).

`providers` and `render` as *directories* are not packages at all — they
hold no `.go` files of their own, only subpackages. Worth being explicit
about this since it's easy to mentally count them as packages when they're
really just filesystem grouping.

### Unnecessary abstractions / interfaces

The `Output` interface from the previous draft is gone. It existed to
support multiple preview/print formats polymorphically, but v0.1 has
exactly one of each (PNG preview, ESC/POS print) — `app.Service` calling
`canvas.EncodePNG()` and `escpos.Encode()` directly is simpler and no less
correct. This follows the standard Go guidance of discovering interfaces at
the second real use, not designing them for a use that doesn't exist yet.
The one deliberate exception is `Font` (§2), which I've flagged rather than
silently accepted — it's the one interface in this design with a single
implementation, kept only because you asked for it explicitly and because
its cost is genuinely near zero (three methods, and `layout`/`canvas` would
want a clean struct boundary around "the font" regardless of whether it's
expressed as an interface).

`Queue` staying a concrete struct rather than an interface (§2) is the
converse case: a real seam (`Store`) got an interface because it has two
live implementations today; a seam with no variation (`Queue` itself) did
not get one.

### Circular dependencies

Full dependency order, confirmed acyclic:

```
apperr
  ↑
receipt, printer, assets, templates   (all leaf-ish, depend only on apperr)
  ↑
render/layout   (receipt, printer[Profile], assets)
  ↑
render/canvas   (render/layout)
  ↑
render/escpos   (render/canvas, printer[Profile])
  ↑
queue           (receipt)
  ↑
config          (printer, queue, assets)
  ↑
auth            (config)
  ↑
app             (receipt, printer, render/*, queue, templates, providers/*, assets)
  ↑
api, webui      (app, auth)
  ↑
cmd/receiptd    (everything)
```

No cycles. The one relationship worth double-checking during
implementation: `app` implements `queue.Processor` structurally (Go doesn't
require a declared "implements" relationship), so `app` depends on `queue`
for the `Job`/`Processor` types, and `queue` must never import `app` back —
worth a lint rule or at least a code-review habit, since it'd be an easy
accidental cycle to introduce later (e.g. if someone reaches for a
`queue`-level helper that "just happens" to need `app`-level context).

### Go idioms

- Registry + `init()` self-registration for Elements/Templates mirrors
  `image.RegisterFormat` / `database/sql.Register` — well-precedented, not
  a novel pattern.
- `apperr` mirrors Upspin's `errors` package (Rob Pike) — typed Kind +
  Op + wrapped cause is the standard idiomatic answer to "typed errors that
  map cleanly across layers."
- `errors.Join` for aggregating per-Element validation failures is the
  stdlib-native way to do this since Go 1.20 — no third-party multierror
  package needed.
- Small interfaces throughout (`Element` has one method now; `Printer` has
  three; `Font` has three) — consistent with "the bigger the interface, the
  weaker the abstraction."

### Premature optimization

`Profile.MaxImageHeightDots` stays a config knob, but the chunking logic in
`escpos.Encode` should ship in Milestone 3 as a no-op (single raster block)
until testing against the real TM-m30II shows chunking is actually
necessary — building that logic speculatively before you know the hardware
needs it would be optimizing against a guess. Same reasoning applies to the
`printer.Transport` dispatch in `cmd/receiptd`: a single `case "network"` is
fine through Milestone 5; don't build a general transport-registry
mechanism before a second transport actually exists to justify it.

### Future maintenance concerns (accepted, not blocking)

- Deferring the `Output` interface means introducing it later will touch
  `app.Service.Preview`/`Process` call sites — a small, known, deliberately
  deferred cost, not a surprise.
- As `providers/*` grows past weather (Milestone 6+), each domain's
  interface is independent by design, which means cross-cutting concerns
  like HTTP timeout/retry handling for flaky upstream APIs will likely want
  a small shared helper eventually. Not needed until there are at least two
  provider domains to see the actual duplication — flagging it now so it
  isn't a surprise later, not because it needs solving today.
- `Receipt.Version` is the only thing standing between "the public API
  contract" and silent breakage as Element types accumulate — worth a
  compatibility test (old fixture JSON still unmarshals) once there are a
  few real templates exercising the schema in Milestone 6.

### Verdict

No further changes needed before implementation begins. The architecture is
internally consistent, the dependency graph is a clean DAG, the package
count is proportionate to the problem rather than to speculative future
requirements, and every interface in the design either has a present second
implementation or was kept deliberately narrow enough that having one
anyway costs nothing.

**Recommend starting Milestone 1**: `apperr`, `receipt` (model + `Validate`),
`render/layout`, `render/canvas` (with `EncodePNG`), and the
`receipt render receipt.json --out preview.png` CLI path. This is the
smallest possible vertical slice that proves the core pipeline — Receipt to
pixels — before any networking, queueing, or hardware enters the picture.
