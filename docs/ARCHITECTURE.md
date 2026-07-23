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
                         the network transport implementation, and
                         ModelProfiles, a small built-in catalogue of
                         verified Profiles config's "model:" field
                         resolves against (docs/adr/0015-printer-model-catalogue.md).
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
                         printer/ (Profile only), receipt/ (to distinguish
                         a positioned Feed from a Cut — see
                         docs/adr/0010-printer-control-elements-via-canvas-controls.md),
                         apperr/.

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
                         filesystem implementation (cmd/receiptd's real
                         Store) and an in-memory one (cmd/receipt's offline
                         path, tests) — the same "both implementations live
                         here as plain files" pattern queue/ already uses.
                         Depends on apperr/.

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
    Copies   int       `json:"copies"` // decoded, not yet acted on — see §3
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

// Style is the resolved, element-type-agnostic styling for one Block. It
// carries no reference back to receipt.Text/receipt.Heading — Build
// resolves either element's fields into a Style once, so render/canvas
// never needs to know which element type produced a given Block. See
// §3 "Text styling" and docs/adr/0007-bitmap-text-styling.md.
type Style struct {
    Bold          bool
    Italic        bool
    Underline     bool
    Strikethrough bool
    Size          int // integer bitmap scale factor, always >= 1 once resolved by Build
}

type Block struct {
    Y       int
    Element receipt.Element
    Style   Style
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

func Build(ctx context.Context, r receipt.Receipt, p printer.Profile, f Font, a assets.Store) (Document, error)
```

`f Font` is an explicit parameter, not folded into `p` or dropped in favour of
always using the one real `Font` implementation internally: Milestone 3's
bitmap-styling and Table work depend on injecting a fake `Font` in
`render/layout` tests to get exact, controlled glyph measurements, and losing
that would be a real regression to that test suite for no design benefit —
see "Font" below for why the interface itself exists at all.

`Font` is the one interface in this design with a single implementation and
no immediate second one planned — a deliberate exception to "don't design an
interface before a second implementation exists," made because you asked for
it explicitly as future-proofing and because it's genuinely narrow (three
methods) and cheap either way: `layout` and `canvas` need a clean boundary
around "the font" regardless, so the interface costs essentially nothing
over a concrete struct while keeping a future proportional/TTF font swap
from touching either package's internals. `Style` is deliberately kept
separate from `Font` rather than added as a parameter to `Measure`/`Glyph`:
`Font` stays the sole source of a glyph's unscaled, unstyled base pixels,
and styling — scaling, bold, and later underline/strikethrough/italic — is
a `render/canvas` concern layered on top of what `Font` returns. See §3
"Text styling" for the full styling pipeline and
docs/adr/0007-bitmap-text-styling.md for why.

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
    ID             string
    PrinterName    string
    Receipt        receipt.Receipt
    State          JobState
    Attempts       int
    LastError      string
    CreatedAt      time.Time
    UpdatedAt      time.Time
    IdempotencyKey string // optional; "" means no key supplied — see docs/adr/0020
}

type Store interface {
    Save(ctx context.Context, j *Job) error
    Get(ctx context.Context, id string) (*Job, error)
    List(ctx context.Context, f Filter) ([]*Job, error)
    NextPending(ctx context.Context) (*Job, error)

    // EnqueueIdempotent atomically looks up newJob.IdempotencyKey, decides
    // whether to create, return an existing match, reject a mismatch, or
    // reactivate a Failed match, and persists that decision — one
    // operation with respect to any other concurrent call for the same
    // key. See docs/adr/0020-idempotent-print-requests.md.
    EnqueueIdempotent(ctx context.Context, newJob *Job, now time.Time) (job *Job, created bool, err error)
}

func NewBoltStore(path string) (Store, error) // store_bolt.go
func NewMemoryStore() Store                    // store_memory.go

type Processor interface {
    Process(ctx context.Context, j *Job) error
}

type Queue struct { /* Store + Processor, retry/backoff (apperr.KindTransient only), cancellation */ }

func (q *Queue) Enqueue(ctx context.Context, j *Job) error
func (q *Queue) EnqueueIdempotent(ctx context.Context, j *Job, key string) (jobID string, err error)
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

func NewFilesystemStore(root string) Store // filesystem_store.go — cmd/receiptd's
                                            // real Store, configured via
                                            // config.AssetsConfig.Path
func NewMemoryStore() Store                // memory_store.go — the same
                                            // "also ship an in-memory Store"
                                            // pattern queue.Store already
                                            // uses; used by cmd/receipt's
                                            // offline render path (which has
                                            // no configured asset backend at
                                            // all) and by tests
```

Both implementations reject a name containing a path separator or a bare
`"."`/`".."` before doing anything else — the same rule
`receipt.Asset.Validate()` already applies to a `Name` arriving via a
Receipt, reused here so a name arriving directly at a `Store` method (e.g.
a future asset-management API handler, which never goes through
`receipt.Asset.Validate()` at all) gets the same protection against
`FilesystemStore` escaping its root via `".."`.

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

func (s *Service) Print(ctx context.Context, r receipt.Receipt, printerName, idempotencyKey string) (jobID string, err error)
func (s *Service) Preview(ctx context.Context, r receipt.Receipt, printerName string) ([]byte, error) // PNG only for now
func (s *Service) RunTemplate(ctx context.Context, name string, p templates.Params) (receipt.Receipt, error)
func (s *Service) JobStatus(ctx context.Context, id string) (*queue.Job, error)
func (s *Service) Process(ctx context.Context, j *queue.Job) error // satisfies queue.Processor
```

`Preview` takes the same `printerName` string `Print` does — see
`docs/adr/0006-preview-requires-printer-profile.md`. Once rendering is
`printer.Profile`-aware (canvas width sized to `profile.WidthDots`, §4
step 8b), a preview is only meaningful relative to a specific printer's
width; there is no "default printer" concept anywhere else in this
schema, and inventing one for `Preview` alone would mean either a second,
non-deterministic rendering path or a fabricated fallback `Profile` with
no basis in configuration. An unknown or missing `printerName` is
`apperr.KindNotFound`, the same behavior `Process` already has for an
unconfigured `Job.PrinterName`.

---

## 3. Receipt model

### Element types (v0.1)

| Type       | Key fields                                                        |
|------------|--------------------------------------------------------------------|
| `text`     | `content`, `align` (`""`/`left`/`center`/`right`, leading-space padded per wrapped line — see "Text styling" below), `bold`, `italic`, `underline`, `strikethrough`, `size` |
| `heading`  | `content` (implies `bold: true, size: 2`, see "Text styling" below) |
| `divider`  | `style` (solid/dashed, optional, default solid), `size` (integer thickness scale factor, optional, default 1 — see "Divider thickness" below) |
| `spacer`   | `height` (dots)                                                     |
| `image`    | `data` (inline base64) — always bytes the client already has        |
| `asset`    | `name`, `width` (dots, clamped to the printable width, may upscale — see "Image vs. Asset" below), `align` (`""`/`left`/`center`/`right`, leading blank-pixel-column padded) — resolved by name via `assets.Store` at layout time |
| `qrcode`   | `content`, `size`, `error_correction`                                |
| `barcode`  | `content`, `symbology` (see "Barcode symbologies" below), `height`, `show_text` (prints `content` as a caption beneath the bars, space-padded to sit roughly centered — see `render/layout.alignPad`) |
| `columns`  | `columns: []{ weight int, elements: []Element }` — recursive        |
| `table`    | `headers: []string`, `rows: [][]string` — flat, no nested Elements  |
| `list`     | `kind` (`""`/`bullet`/`number`/`checkbox`, optional, default bullet), `items: []{ content string, checked bool, indent int }` — flat, no nested Elements — see "Lists" below |
| `feed`     | `lines`                                                              |
| `cut`      | `mode` (full/partial) — optional; renderer supplies `Profile.DefaultCut` if absent |

### Text styling

`Text`'s styling fields — `Bold`, `Italic`, `Underline`, `Strikethrough`,
`Size` — are rendering hints for the bitmap renderer described below, not
a second rendering path per element type. `Size` was previously an
unconstrained `string` with no defined values; per
`docs/adr/0007-bitmap-text-styling.md`, it is now:

```go
type Text struct {
    Content       string `json:"content"`
    Align         string `json:"align,omitempty"`
    Bold          bool   `json:"bold,omitempty"`
    Italic        bool   `json:"italic,omitempty"`
    Underline     bool   `json:"underline,omitempty"`
    Strikethrough bool   `json:"strikethrough,omitempty"`
    Size          int    `json:"size,omitempty"`
}
```

`Size` is an **integer bitmap scaling factor**, not a point size and not
printer/DPI-dependent — the same embedded bitmap face is scaled by an
exact integer multiple, never substituted for a different, larger face.
That embedded face's own native resolution is `basicfont.Face7x13`'s
7x13 glyphs upscaled by a fixed 2x baked into `render/layout.EmbeddedFont`
itself (14x26 native), not `Size`: real 203 DPI thermal hardware testing
found the raw 7x13 glyphs too small to read reliably — see
`docs/adr/0008-embedded-font-legibility.md`. `Size` still means exactly
what it always has, an integer multiple of whatever `EmbeddedFont`
reports as its own native glyph:

- `0` (the field omitted) means "unscaled" and is treated identically to
  `1` — the same zero-as-default convention `printer.Profile.WidthDots`
  and `MaxImageHeightDots` already use elsewhere in this document.
- `1` is normal size (no scaling).
- `2` is double size (every glyph pixel becomes a 2x2 block), `3` is
  triple size, and so on.
- Negative values are invalid: `Text.Validate()` rejects a negative
  `Size` as `apperr.KindValidation`, consistent with every other
  `Validate()` in this schema. `Validate()` stays fast and local for this
  check — no rendering or Font access required to reject a negative
  integer.
- `Size` above `receipt.maxTextSize` (100) is also invalid, for the same
  reason `Spacer.Height` and `Feed.Lines` are bounded: left unbounded, a
  single glyph's scaled bitmap allocation (`render/canvas.scaleGlyph`,
  sized off `Width*factor, Height*factor`) and the layout arithmetic that
  positions it (`Font.LineHeight() * Style.Size`) would be open to an
  arbitrarily large allocation or integer overflow from one oversized or
  malicious value.

`Heading` gains no fields of its own. It remains exactly:

```go
type Heading struct {
    Content string `json:"content"`
}
```

and is defined as presentation sugar over `Text`'s own styling: rendering
a `Heading` is equivalent to rendering a `Text` with `Bold: true, Size: 2`
and every other style field at its zero value. `render/layout.Build`
resolves this equivalence once, at the same point it resolves a `Text`'s
own style fields into a `Style` (§2) — there is no second,
`Heading`-specific styling or painting path anywhere downstream.

`Bold`, `Italic`, `Underline`, `Strikethrough`, and `Align` are all
implemented — `docs/adr/0007-bitmap-text-styling.md` records why the
first four joined the schema together, ahead of actually rendering, and
that gap has since closed (see "Rendering model" below for the pipeline
that paints all four); `Align` closed the same way per
`docs/adr/0013-text-and-asset-alignment.md`. `Align` has a closed,
validated set of values (`""`/`"left"`/`"center"`/`"right"`, `Validate()`
rejects anything else) and is applied by `render/layout.Build` as
leading-space padding on each wrapped line (`render/layout.alignPad`,
called once per line `wrapText` produces, using the same resolved
`Style.Size`), generalizing the same technique `Table`/`Columns`/
`Barcode`'s caption already use — not a new horizontal-position primitive
on `Block` (see "Columns" below). `Asset.Align` closed alongside it, the
same padding idea expressed as blank pixel columns instead of glyphs (see
"Image vs. Asset" below). `Heading` gains no `Align` field, by the same
"no fields of its own" design ADR-0007 already fixed; an aligned
heading-like line is composed from `Text` directly (`Bold: true, Size: 2,
Align: "center"`, for example).

#### Rendering model

The embedded bitmap font (`render/layout.EmbeddedFont`) remains the
**only** source of a glyph's base pixels — its `Font` interface
(`Measure`/`LineHeight`/`Glyph`, §2) is unchanged by this decision and
knows nothing about `Style`. Styling is a separate concern, layered on top
of the bitmap `Font.Glyph` returns, applied by `render/canvas.Paint` in a
fixed pipeline:

1. **Scale** — nearest-neighbour integer scaling of the glyph bitmap by
   `Style.Size` (no interpolation, no anti-aliasing: every source pixel
   becomes an exact `Size x Size` block of identical pixels, preserving
   sharp edges). Always first, because every later step is defined in
   terms of the already-scaled bitmap's dimensions, not the font's native
   ones.
2. **Bold** — a deterministic raster technique (neighbouring-pixel
   overdraw: every set pixel is also set one pixel to its right) applied
   to the scaled bitmap.
3. **Italic** — a deterministic synthetic-italic shear: each row is
   shifted right by an amount that grows moving up the glyph, dropping
   any pixel sheared past the right edge rather than growing the
   bitmap. Applied last. (Reordering this before Bold was tried during a
   real-hardware print review, on the theory that shearing a narrower,
   unbolded glyph first would clip less — it didn't change the output at
   all: both are simple per-row translations under the same final
   bounds check, and clipped translations compose identically regardless
   of order.)

Underline and strikethrough are *not* part of this glyph-transform
pipeline: unlike scale/italic/bold, they don't change a glyph's own
bitmap at all. They're decorations `render/canvas.Paint` draws directly
onto the `Canvas` after every glyph on a line is painted — a horizontal
band positioned and sized (thickness) from the line's own resolved
height and `Style.Size`, so they stay correctly placed and scale
naturally as `Style.Size` grows, without `Font` needing to expose a
baseline or x-height concept it doesn't have.

`render/canvas.Paint` reads each `Block`'s `Style` (§2) only — never
`receipt.Text` or `receipt.Heading` fields directly. This is what keeps
`Heading` from needing a second painting path, and what "exactly one
rendering pipeline" (`docs/adr/0001-receipt-model.md`) continues to mean
once styling exists.

Measurement is not duplicated for styled text: `Font.Measure` keeps
measuring at native size, and because integer nearest-neighbour scaling is
exact and uniform in both axes, the effective width of a `Size: N` string
is precisely `Font.Measure(s) * N`. `render/layout`'s wrapping (`Build`'s
`wrapText`) computes candidate line widths this way, so measurement and
rendering can never disagree about how wide a scaled string is.

### Divider thickness

`render/layout.DividerThickness` (2 dots at `Size: 1`, about 0.25mm at 203
DPI) is the base height a `divider` renders at — visible on real thermal
hardware without reading as a solid filled bar. `Divider.Size` is an
integer thickness scale factor over that base, the same "0 or omitted
means unscaled" convention `Text.Size` uses: the rendered line is
`DividerThickness * Size` dots. Unlike `Text.Size`, this does not go
through `Block.Style` — a `Divider`'s own `Size` field is read directly by
both `render/layout.Build` (`Y` advancement) and `render/canvas.Paint`
(line thickness), the same way a `Spacer`'s own `Height` already is,
since `Style` is documented as text-rendering hints and a divider's
thickness isn't one. See
`docs/adr/0012-divider-thickness-default-and-scaling.md`.
`Divider.Validate()` rejects a negative `Size`, and — the same bound
`Text.Size` uses, for the same reason — a `Size` above
`receipt.maxDividerSize` (100), as `apperr.KindValidation`.

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
returns — `canvas.Paint` never distinguishes between them for the actual
pixels, only `layout` does.

`Asset.Width` and `Asset.Align` are implemented per
`docs/adr/0013-text-and-asset-alignment.md`. `Build` lowers a
`receipt.Asset` into a `layout.AlignedAsset` Block (`Data` plus the
Asset's own already-declared `Width`/`Align`) rather than a plain
`receipt.Image`, and `render/canvas.Paint` resolves it via one more
`rasterBitmap` case (`layout.DecodeAlignedAssetBitmap`), which scales to
`Width` (clamped to the printable width, may upscale — see
`resolveTargetWidth`) and, for `"center"`/`"right"`, left-pads the
rasterized bitmap with blank pixel columns (`alignBitmap`) before it
reaches the same `paintGlyph` primitive every other raster element uses.
Alignment is computed against the full printable width
(`doc.WidthDots`), not the asset's own footprint — unlike
`Barcode`'s caption, which centers under its own barcode's rendered
width, since `Asset.Align` answers "where on the receipt does this
picture sit."

`AlignedAsset` is *not* the same kind of type as `TableLine`/
`ColumnsLine`/`BarcodeCaption` — those synthesize new composed content
with no 1:1 element counterpart, while `AlignedAsset` just carries one
resolved `Asset`'s own already-declared fields past the one I/O
resolution step (`assets.Store.Get`) only `Build` may perform (§4) — see
ADR-0013's "Why a new type" section for the full reasoning, including why
this does not generalize into a wrapper type for every element. An
`AlignedAsset` with no explicit `Width` or `Align` set renders
pixel-identically to the plain `receipt.Image` lowering it replaced —
this was an additive change, not a behavior change for any Receipt that
doesn't use those two fields. An ordinary `receipt.Image` element is
untouched by this: it has no `Width`/`Align` fields in the schema and
gains none.

### Barcode symbologies

`barcode.symbology` accepts exactly six values — the complete, stable set
for v1, defined in `docs/adr/0009-barcode-symbologies.md`:

| Symbology                | `symbology` value |
|----------------------------|-------------------|
| Code 128                   | `code128`         |
| EAN-13                      | `ean13`           |
| EAN-8                       | `ean8`            |
| UPC-A                       | `upca`            |
| Code 39                     | `code39`          |
| Interleaved 2 of 5 (ITF)    | `itf`             |

Any other value — including a symbology `github.com/boombuler/barcode`
itself supports but this list omits (Codabar, Code 93, Data Matrix,
PDF417, Aztec Code, 2 of 5) — fails `receipt.Barcode.Validate()` with
`apperr.KindValidation`, the same closed-vocabulary pattern
`QRCode.ErrorCorrection`/`QRCodeErrorCorrectionLevels` already establishes
for `qrcode.error_correction`. An empty/omitted `symbology` is invalid too;
unlike `qrcode.error_correction` (which defaults to `"medium"`), there is
no default symbology.

`Barcode` generation follows `QRCode`'s established pipeline exactly, per
`docs/adr/0002-raster-rendering.md` (unchanged by this decision):
`render/layout` generates the selected symbology as a bitmap via
`github.com/boombuler/barcode`, producing the same `GlyphBitmap`
`DecodeImageBitmap`/`GenerateQRCodeBitmap` already produce, which
`render/canvas.Paint` paints via the one shared `paintGlyph` primitive
every raster element uses. No printer-native ESC/POS barcode command is
emitted for any symbology — `render/escpos` gains no barcode-specific
encoding of its own.

### Columns

`render/layout.Block` carries only a vertical position (`Y` — see §2's
`Block` definition); there is no horizontal-position primitive anywhere in
`Document`/`Block`/`Canvas`. `Columns` therefore lays its columns out side
by side using exactly the technique `Table` already established for its
own row/column alignment (see `tableColumnWidths`/`tableRowLines`):
`render/layout.Build` word-wraps each column's own content to its share of
`p.WidthDots` — divided proportionally by `Column.Weight` (0 or omitted
floors to 1, `render/layout.ResolveSize`) rather than evenly as `Table`'s
columns are — then composes the wrapped lines side by side into full-width
text, right-padding every non-last column to its column budget. Each
composed line becomes one `ColumnsLine` Block, the `Columns` analogue of
`TableLine`: a `Block` derived from a `Columns` element keeps its own
identity through layout rather than being lowered into `receipt.Text`, and
`render/canvas.Paint` paints its `Content` through the exact same
glyph-by-glyph path `Text`/`Heading`/`TableLine` already use — no new
painting primitive.

Because this technique only has a defined meaning for plain text, only
`receipt.Text` is currently supported inside a `Column`.
`receipt.Columns.Validate()` still recursively validates a column's
`Elements` against the full frozen schema (any `Element` type is accepted
and validated, per §3's "Element types" table) — but `render/layout.Build`
reports any other element type nested in a column (an `Image`, `Divider`,
`QRCode`, `Barcode`, a nested `Table`/`Columns`, or a `Heading`) as
`apperr.KindPermanent` — an unsupported element type nested in a column is
rejected outright, not silently ignored or given placeholder positioning.
Supporting arbitrary nested content side by side would require a real
horizontal-positioning primitive on `Block`/`Canvas`, which is a
materially bigger change than this slice's scope — see §11's "Future
maintenance concerns". `docs/adr/0013-text-and-asset-alignment.md`
(`Text.Align`/`Asset.Align`) deliberately does not introduce that
primitive either: alignment is padding baked into already-positioned
content, the same technique this section's own `Table`/`Columns` column
alignment already uses, not a new coordinate on `Block`.

`receipt.Heading` is called out specifically because it is a case that
might look supportable at first glance — `Build` already knows how to lay
a `Heading` out everywhere else — but isn't, for a different reason than
the other rejected types: a `ColumnsLine` Block carries exactly one
`Style` for its whole composed line, and that line is assembled from one
line of *every* column in the row, each of which may come from a
different, independently-authored `Column.Elements`. A `Heading`'s
`Bold`/`Size: 2` styling (`headingStyle`, §3 "Text styling") cannot be
applied to only its own column's slice of a row shared with a sibling
column's plain `Text` without per-run styling inside a `Block` — another
new rendering primitive, not just a missing type-switch case. Painting the
whole row at `headingStyle` instead would incorrectly style the sibling
columns' `Text` too; painting it at `normalStyle` would silently drop the
`Heading`'s styling. Neither is "supporting" `Heading`, so `Build` rejects
it outright rather than picking one of those two wrong answers.

### Lists

`list` covers bulleted, numbered, and checkbox lists as one Element type
with a closed-enum `Kind`, per `docs/adr/0014-list-elements.md`:

```go
type List struct {
    Kind  string     `json:"kind,omitempty"`
    Items []ListItem `json:"items"`
}

type ListItem struct {
    Content string `json:"content"`
    Checked bool   `json:"checked,omitempty"`
    Indent  int    `json:"indent,omitempty"`
}
```

`Kind` accepts `""`/`"bullet"` (equivalent, the default), `"number"`, or
`"checkbox"` — the same closed-vocabulary pattern `Divider.Style` and
`Barcode.Symbology` already establish. `List` carries no styling fields
of its own, rendering as plain unstyled text like `Table` and `Columns`;
an author wanting styled list text composes it from `Text`.

Marker generation happens during layout, entirely from characters the
embedded ASCII font can render (`docs/adr/0008-embedded-font-legibility.md`):
a hyphen for a bullet item, a 1-based sequential number (independent of
`Indent`) for a numbered item, and `"[x]"`/`"[ ]"` for a
checked/unchecked checkbox item — deliberately ASCII brackets rather
than a Unicode checkbox glyph, which the embedded font cannot render.
`ListItem.Checked` is only valid when the parent `List.Kind ==
"checkbox"`; `Validate()` rejects the combination otherwise rather than
silently ignoring it.

`ListItem.Indent` (0 = top level) is a **semantic nesting level, not a
count of characters**: `Indent: 2` means two levels deep, and how much
visual space one level occupies is a rendering choice, not part of the
schema's contract — expressed as leading content composed onto the line
itself, the same way every other positioning decision in this schema is
(see "Columns" above). `ListItem` has no nested `Items`/`Elements` field —
a flat level integer, not a recursive tree — the same "flat, no nested
Elements" design `Table`'s `Headers`/`Rows` already use. `Validate()`
rejects a negative `Indent` and enforces a defensive maximum nesting depth
(an implementation detail, not part of the public contract), guarding
against pathological width-consuming input the same way `Columns`'
`maxElementDepth` guards against pathological nesting depth.

Each item's `Content` word-wraps to its own available width (narrower
the more deeply an item is indented, degrading to the narrowest
wrapping this schema already falls back to elsewhere rather than
failing); wrapped continuation lines are prefixed with blank space
equivalent to the marker's own width instead of the marker itself, so
they hang-indent under the item's content rather than under its bullet
or number, correctly even when markers vary in width (`"1."` vs.
`"10."`).

Rendering follows `Table`/`Columns`/`Barcode`'s established pattern:
`layout.Build` performs all the semantic expansion — markers, indentation,
and wrapping resolved into fully positioned per-line text content, one
`ListLine` Block per output line, the `List` analogue of `TableLine` and
`ColumnsLine` — and `render/canvas.Paint` paints it through the existing
text-content path, unchanged. No new drawing primitive, no change to
`Block`/`Canvas`/`Document`.

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

`copies` is decoded and round-tripped (`receipt.Receipt.Copies`), but no
stage of the pipeline — `layout`, `canvas`, `escpos`, `printer` — reads it
yet; every Job prints exactly once regardless of its value. Multi-copy
printing has no milestone assigned; treat the field as reserved, not
functional.

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
     Receipt it just built and calls `s.Preview(ctx, receipt, printerName)`
     — the same method the raw `/api/v1/preview` endpoint uses, and the
     same `printerName` the `/print` variant below already needs — and
     returns PNG bytes.
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
      Elements, resolves each `Text`/`Heading`'s styling fields into a
      `Style` (`Heading` resolving to `Bold: true, Size: 2` — §3 "Text
      styling"), measures text via `Font.Measure(s) * Style.Size`, wraps
      lines, sizes columns/table cells, resolves any `Image`/`Asset`
      elements into decoded pixel content, computes vertical positions.
      Produces a fully resolved `Document` — **after this call, nothing
      downstream ever touches `receipt.Receipt`, `assets.Store`, or any
      provider again.** `layout` is the only stage that talks to the
      outside world.
   c. `canvas.Paint(document)` — paints every Block onto a monochrome
      bitmap using `document.Font.Glyph()` for text, scaled and bolded per
      `Block.Style` (§3's rendering model pipeline),
      `boombuler/barcode`-generated bitmaps for QR/barcode Blocks, decoded
      image pixels for Image/Asset Blocks, lines for dividers.
   d. If the Receipt didn't end with an explicit `cut`, `escpos.Encode`
      appends `profile.DefaultCut` behavior.
   e. `escpos.Encode(canvas, profile)` — init bytes, raster commands
      (split into bands of at most `profile.MaxImageHeightDots` rows; a
      Profile with no limit set produces a single raster command — see
      §11), feed, cut.
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
  KindTransient→503, KindPermanent→500`). The API is the trust boundary: a
  4xx response body returns `err.Error()` (actionable — the request itself
  was the problem), but a 5xx response body is always the fixed generic
  `"internal server error"` message, never the wrapped error, a
  filesystem/database path, a network error, or the `Op` — the real `err`
  is logged server-side instead (`api.writeError`).
- **Queue retry**: `apperr.Is(err, apperr.KindTransient)` is the *only*
  condition that consumes retry budget (formalizes the transient-vs-permanent
  distinction from the previous revision).
- **Logs**: `logger.With("kind", err.Kind, "op", err.Op)` via
  `charmbracelet/log` — structured, not string-parsed.
- **Tests**: assert `apperr.Is(err, apperr.KindNotFound)`, never assert on
  `err.Error()` message text — robust to wording changes.

---

## 6. Job system

States `pending -> running -> {done|failed}` plus `pending -> cancelled`
(cancelling a `running` job returns an "already in progress" error, it
can't un-print). Retries: bounded attempts (default 3), exponential
backoff (default 5s base), gated strictly on `apperr.KindTransient`.
Persistence: `bbolt` by default (`queue.NewBoltStore`), in-memory available
as an explicit opt-out (`queue.NewMemoryStore`) — both now plain files in
the `queue` package rather than subpackages (§11).

**Worker topology: one worker per configured printer**
(`docs/adr/0016-queue-concurrency-per-printer-workers.md`). `cmd/receiptd`
starts one background goroutine per entry in the configured printer set
(`buildPrinters`'s own key set), each running `runWorker`, which loops
calling `queue.Queue.ProcessNextForPrinter(ctx, printerName)` and sleeping
`pollInterval` between calls — scoped to exactly one printer's Jobs, never
more than one worker per printer name. With today's typical single-printer
deployment this is exactly one worker, doing exactly what the daemon did
before this decision. The difference only appears once a second printer is
configured: a slow or offline printer's worker can retry with backoff
indefinitely without blocking any other printer's worker from claiming and
sending its own Jobs — the problem a single global worker had, since every
printer funneled through the same goroutine and the same retry/backoff
state machine.

**Atomic per-printer claim.** `queue.Store` gained
`ClaimNextPending(ctx, printerName) (*Job, error)`, which atomically finds
the lowest-ID `Pending` Job for `printerName`, transitions it to `Running`,
and persists that transition in one step — `boltStore` inside one
`db.Update` call, `memoryStore` inside one `s.mu.Lock()` critical section.
No two concurrent callers, for the same printer name or a different one,
can ever be handed the same Job. This is what makes one worker per printer
safe: same-printer concurrency is still structurally impossible (exactly
one worker ever drains one printer's lane), and the atomic claim is
defense in depth for that guarantee rather than its primary source.
`queue.Queue.ProcessNextForPrinter` is the entry point built on top of it,
sharing its retry/backoff/error-handling logic with `ProcessNext` via a
private helper; `ProcessNext` and `Store.NextPending` (the old,
non-atomic, printer-unaware lookup) are both unchanged and remain
available for a caller that genuinely wants the old global, single-Job-
at-a-time behavior — a worker just doesn't use them anymore.

**What this does not promise.** Job IDs are random hex, so even within one
printer's lane, claim order is arbitrary with respect to enqueue order —
true before this decision and unchanged by it. What *is* new: there is no
longer a single global claim order across all printers either. A Job for
printer B can now complete before an earlier-enqueued Job for printer A,
if A's worker happens to be busy retrying — by design, since that is
exactly the head-of-line blocking this decision exists to remove. Nothing
in this design promises ordering within or across printers; a caller that
needs it must serialize its own requests.

**Shutdown** (`cmd/receiptd`, `docs/adr/0018-graceful-shutdown.md`):
`SIGTERM`/`SIGINT` stop the HTTP server accepting new requests and stop
every per-printer worker claiming a new `Job` immediately (all at the same
instant — none waits on another), but let a `Job` already inside `Process`
(in particular, an in-progress `Printer.Send`) finish naturally rather than
cutting it off mid-transmission — there is no defined "resume a partial
raster image" command (`docs/adr/0002-raster-rendering.md`), so a stream
cut short there is worse than simply waiting for it. A worker instead
asleep in a retry's backoff wait (nothing physical in flight) *is*
interrupted immediately; the `Job` it was retrying is left `running`, not
`failed` — synthesizing a permanent failure for a retry the shutdown chose
not to wait out would misrepresent what happened, and a `Job` found
`running` at the next startup is exactly what
`docs/adr/0017-queue-lifecycle-crash-recovery.md`'s reconciliation pass
already has to handle for a crash, so shutdown reuses that one mechanism
rather than a second one. The whole drain (every in-flight HTTP request
and every printer's worker together, not per-request or per-worker) is
bounded by one fixed internal deadline (`shutdownDeadline`, initially
30s); reaching it, or a second shutdown signal while already draining,
forces an immediate exit. See the ADR for the full sequence and the
operator-facing grace-period guidance in the README — there is
deliberately no config field for the deadline (§7's schema stays frozen).

**Crash recovery** (`cmd/receiptd`, `docs/adr/0017-queue-lifecycle-crash-recovery.md`):
`(*daemon).run` calls `Queue.Reconcile` once, synchronously, before
`startWorker` — every Job it finds `JobRunning` is unconditionally orphaned,
since no worker has claimed anything yet and this project never runs more
than one `receiptd` process against one `Store`. The interrupted attempt
counts as one consumed try against `Attempts`/`max_attempts`: still under
budget, the Job returns to `JobPending` with a fixed, greppable
`LastError`; budget exhausted, it goes to `JobFailed` instead — either way,
visible, never stuck in `JobRunning` forever. This is also the backstop for
a Job the Shutdown paragraph above leaves non-terminal mid-backoff. Making
this correct required `runClaimedJob` to persist `Attempts` after every
attempt, not only at the retry loop's start and end, so a crash mid-loop
no longer undercounts what Reconcile checks against `max_attempts`.

**Idempotent print requests** (`docs/adr/0020-idempotent-print-requests.md`):
`POST /api/v1/print` accepts an optional `Idempotency-Key` header, threaded
through to `Service.Print`'s `idempotencyKey` parameter and, if non-empty,
stamped onto the `Job` as `Job.IdempotencyKey`. An empty key (the header
omitted, today's every-existing-client behavior) is always a new `Job` —
identical to calling `Queue.Enqueue` directly, which `Service.Print` in fact
does in that case. A non-empty key instead calls the new
`Queue.EnqueueIdempotent`, which delegates atomically to
`Store.EnqueueIdempotent`:

- No non-expired `Job` recorded under the key: a fresh `Job` is created
  and persisted, keeping the key, exactly like `Enqueue` otherwise would.
- A non-expired match exists but its `Receipt` or `PrinterName` differ from
  the incoming request: `apperr.KindValidation`, nothing persisted or
  mutated — this is a client key-reuse bug, not a legitimate retry.
- A non-expired match exists and agrees on `Receipt`/`PrinterName`: if its
  `State` is `Pending`, `Running`, or `Done`, nothing is created and that
  Job's ID is returned unchanged. If its `State` is `Failed`, it is
  reactivated **in place** — the one `failed -> pending` transition this
  ADR adds beyond the state machine above, gated specifically on a
  same-key client retry — with `Attempts` reset to 0 and `LastError`
  cleared, rather than being replaced by a second `Job`.

"Atomically" means the whole "look up by key, decide, persist" sequence
runs as one operation with respect to any other concurrent call for the
same key (one `bbolt` `db.Update` in `boltStore`, one `memoryStore.mu`
critical section) — the same check-then-act discipline
`docs/adr/0016-queue-concurrency-per-printer-workers.md` requires for
atomic Job claiming, reused here rather than re-derived. A key is valid
for `queue.IdempotencyKeyTTL` (24 hours) from the `Job`'s `CreatedAt`;
expiry is enforced lazily, only at lookup time inside
`EnqueueIdempotent` — there is no active sweep or deletion of expired
keys, and an expired match is simply treated as if no `Job` had ever been
recorded under that key.

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
  - name: front-desk
    # -- Connection fields --
    transport: network
    address: 192.168.1.50:9100
    # -- known model (recommended) --
    model: epson-tm-m30ii

  - name: custom-printer
    # -- Connection fields --
    transport: network
    address: 192.168.1.60:9100
    # -- custom profile (advanced) --
    profile:
      printable_width_mm: 72.02
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

Each YAML `printers[]` entry stays a single block for the user's
convenience — `config` is the one place that splits it into the
`printer.Profile` and `printer.Connection` Go values, matching the "one
config file, two internal types" trade-off described in §1.

A printer's `Profile` is resolved from exactly one of two mutually
exclusive sources (`docs/adr/0015-printer-model-catalogue.md`) — an
entry giving both or neither is rejected outright, with no precedence
rule between them:

- **`model:`** (recommended) — a name looked up in
  `printer.ModelProfiles`, a small built-in catalogue of `Profile`
  values whose characteristics have been independently verified,
  preferably through real hardware testing. This needs no
  hardware-specific knowledge from the operator beyond the printer's own
  model name.
- **`profile:`** (advanced) — every `Profile` field supplied explicitly,
  for hardware not yet in the catalogue. `printable_width_mm` must be
  the printhead's actual printable width, not the paper roll width
  printed on the packaging — these are frequently different numbers
  (most 80mm-roll thermal printers, including the Epson TM-m30II behind
  the one built-in `model:` entry, only address a 72mm-wide printhead).
  Configuring the roll width here produces a `WidthDots` wider than the
  hardware can actually raster, which doesn't necessarily fail cleanly —
  see the ADR for the real-hardware symptom this caused.

Nothing in `config` or `printer` ever derives a printable width from a
paper/roll size — every `Profile` in the system is either a verified
`model:` catalogue entry or a value the operator states explicitly in
`profile:`, never a guess.

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
First receipt physically prints on the TM-m30II. `escpos.Encode` splits a
tall Canvas into raster bands no taller than `profile.MaxImageHeightDots`
(§11); a Profile with no limit set still produces a single raster command.

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
render/escpos   (render/canvas, printer[Profile], receipt)
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

`escpos.Encode` splits a Canvas into raster bands of at most
`profile.MaxImageHeightDots` rows — a real printer-compatibility need, not
a speculative one, since some ESC/POS printers reject a single raster
command above a certain height. The chunking logic itself does no more
than that split: no retry, no backoff, no adaptive band sizing based on
observed printer behavior — those would be optimizing against a guess
until real hardware testing shows they're needed. Same reasoning applies
to the `printer.Transport` dispatch in `cmd/receiptd`: a single `case
"network"` is fine through Milestone 5; don't build a general
transport-registry mechanism before a second transport actually exists to
justify it.

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
- `Columns` currently only renders `Text` content inside a column (see §3
  "Columns") because `Block` has no horizontal-position primitive to place
  anything else with, and no per-run styling primitive to give a `Heading`
  its own styling within a row shared with other columns. If a real use
  case needs, say, an `Image` inside a column, that's the point to design
  an actual `X` concept onto `Block`/`Canvas`; if it needs a `Heading`
  inside a column, that's the point to design per-run `Block` styling —
  not before, per "discover interfaces at the second real use"
  (`CLAUDE.md`).
- Per-`Element` validation (e.g. `maxElementDepth` bounding `Columns`/`list`
  nesting) bounds any single Element's own pathological shape, but not a
  `Receipt`'s aggregate size — nothing today bounds total Element count,
  overall rendered height, or final `Canvas`/PNG size for a `Receipt` built
  from many individually-valid Elements. A request could still drive
  `render/canvas.Paint` into producing an extremely large bitmap. Worth a
  future document-level limit (max Element count, max rendered height, or
  similar) once there's a concrete reason to add one — not a gap this
  needs to close today, since per-Element validation already closes the
  more common pathological cases.

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
