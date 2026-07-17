# 0001. Receipt as a printer-agnostic document model

Status: Accepted

## Context

Receiptd needs a single representation of "what to print" that can be
produced from several input sources — raw JSON from an API client,
Markdown, and server-side templates (weather, shopping lists, homelab
alerts) — and consumed by a single rendering pipeline regardless of which
printer it eventually reaches.

The naive approach — let each client describe printer-level concerns
directly (paper width, cut behavior, raw ESC/POS bytes) — was rejected
early: it would mean every client needs printer-specific knowledge, which
directly contradicts the project's core goal (see `README.md`
"Philosophy": no client should know it's talking to an ESC/POS printer).
An earlier draft of the spec even had `Receipt.PaperWidth` as a
client-supplied field — an internal contradiction with that same stated
goal, since the client shouldn't know the paper width either.

## Decision

`Receipt` is a printer-agnostic document: an ordered list of typed
`Element`s (`text`, `heading`, `divider`, `spacer`, `image`, `asset`,
`qrcode`, `barcode`, `columns`, `table`, `feed`, `cut`), deliberately
similar in spirit to Slack's Block Kit. It carries no paper width, DPI, or
printer identity — those live server-side in `printer.Profile`, resolved
by printer name at render/print time, not supplied by the client.

Every input source (JSON, Markdown, templates) converts into this same
`Receipt` value before rendering. There is exactly one rendering pipeline
(`receipt.Receipt` → `render/layout.Document` → `render/canvas.Canvas` →
`render/escpos` bytes); no input source gets its own parallel path.

Polymorphic JSON unmarshaling uses a `type` string discriminator resolved
through a factory registry populated by each Element's own `init()` — the
same pattern as `image.RegisterFormat` / `database/sql.Register` — rather
than a large type-switch maintained in one place, so adding an Element
type touches exactly one new file.

`Element` exposes only `Validate() error` — no `Kind()` identity method.
The JSON `type` discriminator is a serialization concern already handled
by the registry during unmarshal; rendering dispatches on the Go concrete
type via a type switch, using information the compiler already has,
without a hand-rolled identity method duplicating that.

`Image` and `Asset` are kept as two distinct Element types rather than
merged into one, even though they render identically today (both become a
decoded bitmap in the `Document`). `Image` means "here are the bytes";
`Asset` means "look this name up." The resolution step — literal decode
for `Image`, a `assets.Store.Get` lookup for `Asset` — is exactly where
future flexibility belongs: an `Asset` could later resolve through an SVG
rasterizer, a generated or templated image, or a theme-aware logo variant,
none of which requires the Receipt/Element JSON schema to change. Merging
them would have baked "images are always literal bytes" into the public
API contract.

## Consequences

- Clients (CLI, API callers, the Web UI) only ever construct or receive
  plain `Receipt` JSON — never printer-specific bytes or dimensions.
- Adding a new Element type is a single new file with a `Validate()`
  method and an `init()` registration — no changes needed to the JSON
  unmarshaling machinery itself.
- `Asset` resolution can evolve (caching, SVG, generated content) without
  a schema version bump, because the schema only ever said "look up this
  name," not "here are literal bytes."
- `Validate()` is deliberately fast and local (no I/O) — it cannot itself
  tell you whether a named `Asset` actually exists. That check is deferred
  to `render/layout.Build`, which already performs I/O and returns
  `apperr.KindNotFound` if the asset is missing. This is a conscious
  layering decision, not an oversight: validation answers "is this
  document well-formed," resolution answers "does everything it points to
  actually exist."

## Alternatives considered

- **Client-supplied paper width / printer parameters**: rejected as
  directly contradicting the project's core goal; also brittle, since a
  client would need to be updated whenever printer hardware changed.
- **`Kind() string` on the `Element` interface**: considered and dropped —
  redundant with both the JSON `type` discriminator (unmarshal-time) and
  the Go type switch (render-time); it would be a second identity
  mechanism doing a job the language and the registry already do.
- **Merging `Image` and `Asset` into one Element type**: considered
  (an earlier draft of this decision took this direction) and reversed
  after concluding it foreclosed future Asset resolution flexibility for
  no present benefit — the two concepts ("here are bytes" vs. "look this
  up") are genuinely different even though they render identically today.
- **A single large type-switch for JSON unmarshaling**, maintained in one
  file: rejected in favor of the registry pattern specifically so adding
  an Element type never requires editing a shared file, reducing merge
  conflicts and keeping each Element type self-contained.
