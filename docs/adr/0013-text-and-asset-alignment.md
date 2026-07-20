# 0013. Closing the `Text.Align`/`Asset.Align`/`Asset.Width` gap with pixel- and space-padding

Status: Accepted

## Context

`Text.Align`, `Asset.Align`, and `Asset.Width` have been in the schema
since Milestone 1, accepted, validated (`Width` only — `Align` has no
validation at all today), and round-tripped through JSON, but never
interpreted by `render/layout.Build`. `docs/adr/0007-bitmap-text-styling.md`
already flagged the same problem for `Text.Size` and fixed it; its own
text notes the identical gap was left open for `Align`: "`Align` does not
have a fixed set of valid values, so `Validate` does not restrict it."
Milestone 3's Divider.Style and Barcode.ShowText work
(`docs/ARCHITECTURE.md` §3) closed every other "accepted but not
rendered" field except these three — this ADR is the same kind of
concrete-evidence-driven decision `docs/adr/0007` describes, reached by
attempting the implementation and stopping at the first genuinely
undefined piece of API surface, per this project's "if the API is
missing, stop and refine before implementing" convention.

Three things need deciding, all interconnected:

1. What values does `Align` accept, for `Text` and `Asset` alike, and
   what does `Validate()` do with an invalid one?
2. Given `render/layout.Block` is `{Y, Element, Style}` — no horizontal
   coordinate, by design (`docs/ARCHITECTURE.md` §3 "Columns": "there is
   no horizontal-position primitive anywhere in `Document`/`Block`/
   `Canvas`") — how can alignment have any visible effect at all?
3. `Asset.Width` has a materially different shape of problem than the
   other two: `render/layout.Build` today lowers a `receipt.Asset` into a
   plain `receipt.Image{Data: ...}` `Block` (see `Build`'s own doc
   comment on the `receipt.Asset` case), which has no `Width` field to
   carry, and `render/canvas.Paint` independently re-decodes that `Data`
   from scratch via `layout.DecodeImageBitmap(e.Data, doc.WidthDots)`
   with no per-`Block` parameter besides the page width. There is
   currently no path for a per-`Asset` width request to reach the stage
   that actually rasterizes pixels.

## Decision

### Align: a closed four-value enum, shared by `Text` and `Asset`

`Text.Align` and `Asset.Align` both accept exactly:

| Value | Meaning |
|---|---|
| `""` (omitted) | left-aligned — today's implicit behavior, unchanged |
| `"left"` | left-aligned, explicit |
| `"center"` | centered |
| `"right"` | right-aligned |

Any other value is rejected by `Validate()` with a plain error (wrapped
`apperr.KindValidation` at the `Receipt.Validate()` aggregation point, the
same as every other field-level `Validate()` check in this schema) — the
same closed-vocabulary pattern `Barcode.Symbology`
(`docs/adr/0009-barcode-symbologies.md`) and
`QRCode.ErrorCorrection`/`QRCodeErrorCorrectionLevels` already establish,
now applied to `Align` instead of leaving it a free string. This is a
breaking change in the narrow sense that a previously-accepted arbitrary
string (e.g. `"middle"` or `"justify"`) now fails validation — acceptable
under the same "pre-alpha, no installed base" reasoning
`docs/adr/0006-preview-requires-printer-profile.md` and
`docs/adr/0007-bitmap-text-styling.md` already used for their own
breaking changes to this schema.

`Heading` gains no `Align` field. This is a restatement, not a new
decision: `docs/adr/0007-bitmap-text-styling.md` already fixed `Heading`
as presentation sugar over `Text` with no fields of its own, precisely so
`Heading` never becomes a second, parallel place styling/positioning
concepts accumulate. An author who wants a centered heading-like line
today reaches for `Text` with `Bold: true, Size: 2, Align: "center"` —
the same "compose it from `Text`'s own fields" answer already implied by
`Heading`'s existing design.

### Where horizontal positioning state lives: nowhere new

`Block` stays exactly `{Y, Element, Style}`. No `X` field is added to
`Block`, `Document`, or `Canvas`, and `render/canvas.Paint`'s core loop —
which paints every glyph and every raster bitmap starting at `x = 0`
(`c.paintGlyph(0, b.Y, ...)` for raster Blocks, `x := 0` at the top of the
text-glyph loop) — is unchanged by this decision. This is a deliberate
continuation of the technique this codebase already uses three times over
for exactly this "something needs to look positioned, but `Block` has no
coordinate for it" problem:

- `render/layout.tableRowLines`/`padToWidth` right-pads non-last table
  columns with trailing spaces so the next column starts at a consistent
  offset.
- `render/layout.columnsLines` does the same for `Columns`.
- `render/layout.centerBarcodeCaption` left-pads a barcode's caption with
  leading spaces so it reads as roughly centered under the barcode.

In every case, the *content itself* — not a coordinate — carries the
positioning, because the content is always painted from `x = 0` through
the one shared glyph-by-glyph (or, for bitmaps, one shared blit)
primitive. `Text.Align` generalizes this from trailing-only (`Table`/
`Columns`) and leading-only (`Barcode` caption) padding into both
directions; `Asset.Align` extends the same idea from glyph columns to
pixel columns. Nothing about `docs/ARCHITECTURE.md` §3 "Columns"'s
existing "there is no horizontal-position primitive" statement is
reversed by this decision — it is reaffirmed and reused, not
contradicted. Adding a real `Block.X`/coordinate primitive was considered
and rejected — see "Alternatives considered".

### `Text.Align`: generalizing the existing space-padding helper

`render/layout.centerBarcodeCaption` is generalized into one shared
helper (name subject to the implementing slice, e.g. `alignPad(content
string, align string, width int, f Font, size int) string`) that all
leading-space alignment in this package goes through:

- `align` is `""`/`"left"`: return `content` unchanged (no padding) — the
  fast path, and today's implicit behavior for every existing Receipt.
- `align` is `"center"`: left-pad with as many leading spaces as fit
  within half of `width - f.Measure(content)*size` — exactly
  `centerBarcodeCaption`'s existing arithmetic, unchanged.
- `align` is `"right"`: left-pad with as many leading spaces as fit
  within the *full* `width - f.Measure(content)*size`.
- `width <= 0` (Build's documented "no printer configured" sentinel) or
  `content` already as wide as or wider than `width`: return unchanged —
  the same fallback `centerBarcodeCaption`/`padToWidth` already apply.

`centerBarcodeCaption`'s own call site becomes `alignPad(content,
"center", width, f, 1)` — one fewer near-duplicate implementation of the
same padding arithmetic, discovered exactly because this is the second
real use of "pad text so it reads as positioned," the same "abstract at
the second use" principle (`CLAUDE.md`) that already governs when this
codebase introduces a shared helper versus leaving two copies.

`Build`'s `receipt.Text` case applies `alignPad` to each *wrapped* line
independently, using the same `p.WidthDots` reference `wrapText` already
wrapped that line against and the same resolved `Style.Size`:

```go
for _, line := range wrapText(e.Content, p.WidthDots, f, style.Size) {
    e.Content = alignPad(line, e.Align, p.WidthDots, f, style.Size)
    blocks = append(blocks, Block{Y: y, Element: e, Style: style})
    y += f.LineHeight() * style.Size
}
```

Aligning per wrapped line (rather than, say, once across the whole
`Text` element's bounding box) matches how `TableLine`/`ColumnsLine`
already treat each composed line independently, and is the only
definition that makes sense once a single `Text` element can wrap into
several lines of different natural widths — a right-aligned paragraph
should have every line's *own* right edge line up, not be padded as a
block.

No new `Block`-carrying type is needed for `Text`: unlike `Table`,
`Columns`, and `Barcode`'s caption, a wrapped `Text` line's content
already lives directly on the per-line `receipt.Text` copy `Build`
already mutates (`e.Content = line`) before emitting the `Block`. Padding
that same field before emitting the `Block` means `render/canvas.Paint`
needs zero changes for `Text.Align` — the padded string flows through the
existing `textContent`/glyph-painting path completely unchanged, the same
way `centerBarcodeCaption`'s output already does for `BarcodeCaption`.

### `Asset.Align` and `Asset.Width`: padding pixels instead of glyphs

`Asset`'s resolved content is a bitmap, not text, so `alignPad`'s
glyph-padding technique doesn't literally apply — but its generalization
does: pad the *bitmap* with blank (unset) leading pixel columns so the
image sits at the correct horizontal offset once painted at `x = 0`,
mirroring leading *space* padding with leading *blank-column* padding.

This does require touching the one thing `Text.Align` didn't: how
`receipt.Asset` reaches `render/canvas.Paint`. `Build` currently lowers
`receipt.Asset` into `receipt.Image{Data: data}`, discarding `Name`,
`Width`, and `Align`. A `render/layout`-local carrier type, produced only
by `Build`, never part of a client-supplied `receipt.Receipt`, is the
answer — but, importantly, **for a narrower and more specific reason than
"`TableLine`/`ColumnsLine`/`BarcodeCaption` already establish this
pattern, so reuse it."** That framing was examined and rejected during
review of this ADR — see "Why a new type, and why it's not the same
move as `TableLine`/`ColumnsLine`/`BarcodeCaption`" immediately below for
the actual justification, which the type's own doc comment reflects:

```go
// AlignedAsset is a resolved receipt.Asset's pixel data plus its own
// already-declared Width/Align request. Unlike TableLine/ColumnsLine/
// BarcodeCaption, this is not layout-synthesized content with no 1:1
// element counterpart — it is the same one Asset's own fields, carried
// forward past the one resolution step (assets.Store.Get) only Build is
// positioned to perform. See docs/adr/0013-text-and-asset-alignment.md
// for why that resolution step, not "new types carry extra fields," is
// what actually forces this to be a distinct type from receipt.Asset or
// receipt.Image.
type AlignedAsset struct {
    Data  []byte
    Width int    // 0 = no explicit width requested (today's shrink-to-fit-only behavior)
    Align string // "" (left, default) | "left" | "center" | "right"
}

func (AlignedAsset) Validate() error { return nil }
```

`Build`'s `receipt.Asset` case emits `Block{Y: y, Element:
AlignedAsset{Data: data, Width: e.Width, Align: e.Align}, Style:
normalStyle}` instead of `receipt.Image{Data: data}`. An ordinary
`receipt.Image` element is untouched — it has no `Width`/`Align` fields
in the schema and gains none; `AlignedAsset` exists specifically because
`Asset` (and only `Asset`) has fields `Image` doesn't.

### Why a new type, and why it's not the same move as `TableLine`/`ColumnsLine`/`BarcodeCaption`

This point was raised directly in review of this ADR, and it's correct as
stated: `TableLine`, `ColumnsLine`, and `BarcodeCaption` are
layout-*synthesized* content — a `Table` with five rows produces five (or
more, once wrapped) `TableLine` Blocks that never existed as discrete
elements in the Receipt; a `Barcode`'s caption is a wholly new line of
text derived from, but structurally unlike, the barcode itself. Those
three types earn their existence because the *content* is new.
`AlignedAsset` is not that: it is the same one `Asset`, one Block in, one
Block out, carrying fields (`Width`, `Align`) that were already sitting on
`receipt.Asset` before this ADR — nothing new is being synthesized. The
original draft of this ADR leaned on the `TableLine`/`ColumnsLine`/
`BarcodeCaption` precedent to justify `AlignedAsset`, and that was the
wrong justification: it doesn't distinguish "this needs a new type" from
"this project has used new types before," which is exactly the "a
dedicated type for every future render-time option won't scale"
objection this section exists to answer properly.

Two concrete alternatives were weighed instead, both aimed at avoiding a
new type entirely:

**Could `Width`/`Align` travel as metadata on `Block` itself, keeping the
`Element` a plain `receipt.Image`?** This was seriously considered — it
would in fact eliminate the need for `AlignedAsset` completely, since
`Data` alone (already carried by the existing `receipt.Image`) is the
only reason a distinct `Element` type is needed at all. It was rejected
for two reasons, not one:

1. `render/layout.Block` being frozen at exactly `{Y, Element, Style}` is
   referenced, verbatim, in half a dozen places across this codebase as a
   load-bearing invariant (`Build`'s own doc comment; `DecodeImageBitmap`'s
   doc comment explicitly ties it to `Block` staying comparable;
   `docs/ARCHITECTURE.md` §2's core-interfaces listing). Adding `Width`/
   `Align` fields to `Block` is not a Build-local implementation
   convenience the way a new `Element` type is — it changes a type this
   project has repeatedly treated as a bigger, more consequential
   commitment than adding a small internal type, and §11's "Future
   maintenance concerns" already earmarks exactly this move ("design an
   actual `X` concept onto `Block`/`Canvas`") as something to do
   deliberately, in full, "if a real use case needs" it — not
   incrementally, two fields at a time, under a different name, for one
   element type. Doing it narrowly here would be building half of that
   larger, deliberately-deferred primitive while calling it something
   else.
2. It generalizes before a second real use exists — the same principle
   (`CLAUDE.md`: "discover interfaces at the second real use, not
   before") that already governs every other design choice in this
   document, just pointed at `Block` instead of at a helper function.
   Every other `Block` variant — `Text`, `Heading`, `Divider`, `Spacer`,
   `TableLine`, `ColumnsLine`, `BarcodeCaption`, `QRCode`, `Barcode`,
   `Feed`, `Cut` — would carry two fields it never reads, for the sole
   benefit of the one variant (`Asset`) that does. That is the mirror
   image of the scaling problem being raised: instead of many
   narrow-but-clear types, `Block` becomes one wide struct most of whose
   instances leave most of its fields meaningless — which is worse for
   readability, not better, once a third or fourth element type wants its
   *own* unrelated render-time knob (a future per-element rotation,
   opacity, whatever it turns out to be) and `Block` keeps growing to fit
   all of them.

**Could `Build` simply not resolve the `Asset` at all — keep the
`Block.Element` as (something like) `receipt.Asset`, and let
`render/canvas.Paint` read `Width`/`Align` off it directly at paint
time**, the same way `render/canvas.Paint` already reads `receipt.Divider`'s
own `Style`/`Size` fields directly, with no wrapper type whatsoever
(`paintDivider`/`blockHeight` type-assert `receipt.Divider` and read its
fields straight off)? `Divider` is genuine, working precedent in this
codebase for "don't invent a carrier type, read the original element's
fields in `Paint`" — and it's the right answer *for `Divider`*, so it
deserves a real answer for why it isn't the right answer here too.

The difference is `docs/ARCHITECTURE.md` §4's rendering-architecture
invariant: "after this call [`layout.Build`], nothing downstream ever
touches `receipt.Receipt`, `assets.Store`, or any provider again... layout
is the only stage that talks to the outside world." `Divider` needs no
resolution step at all — `receipt.Divider{Style, Size}` is already
exactly the representation `Paint` needs, so passing it through unchanged
costs nothing and crosses no boundary. `Asset` is fundamentally
different: turning `Name` into pixel bytes is I/O (`assets.Store.Get`),
and only `Build` is allowed to perform it. `receipt.Asset` itself has no
field to hold resolved bytes (deliberately — "look this up by name" is
the entire point of `Asset` existing as a separate type from `Image`, see
"Image vs. Asset" in `docs/ARCHITECTURE.md` §3), and it must not gain one
just to satisfy this: `Data` is not part of the client-facing `asset`
JSON schema and adding it would blur exactly the "Image means bytes,
Asset means a name to resolve" distinction that section exists to
preserve. So *some* type distinct from `receipt.Asset` is structurally
required to carry `Data` forward from `Build` to `Paint` — this part is
not a stylistic choice, it is forced by the §4 I/O-boundary invariant,
independent of where `Width`/`Align` end up living. Once that much is
already true, carrying `Width`/`Align` on the same small type costs
nothing further and avoids the `Block`-widening problem above.

**The dividing line going forward**, which directly answers "won't scale
to every future render-time option": an element that needs new
render-time metadata but *no* `Build`-side resolution follows `Divider`'s
path — read the concrete `receipt.*` type's own fields directly in
`Paint`, no wrapper, no `Block` change, nothing added here. An element
that also needs `Build`-side resolution (today, only `Asset`, because only
`Asset` crosses the `assets.Store` I/O boundary) needs a small,
purpose-built resolved-content carrier — and only then, only for that
element, not as a preemptive general mechanism every raster element gets
whether it needs one or not. If a second element ever needs the same
shape of carrier (a hypothetical future per-`Barcode` or per-`QRCode`
placement option, say), *that* second real use is the point to consider
factoring out something shared between it and `AlignedAsset` — not
before, and not by generalizing `Block` speculatively today.

**Resolving a target width** (shared by `Build`, to advance `Y`, and
`render/canvas.Paint`, to actually rasterize, so the two stages can never
disagree — the same "one resolution function, two call sites" precedent
`ResolveSize` already establishes for `Text.Size`/`Divider.Size`):

```go
// resolveTargetWidth returns the width, in dots, an Asset with an
// explicitly requested width (0 = none) renders at, given the page's
// printable width maxWidth (0/negative = Build's "no printer configured"
// sentinel).
func resolveTargetWidth(requestedWidth, maxWidth int) int {
    if requestedWidth <= 0 {
        return 0 // caller falls back to today's scaledImageSize shrink-to-fit-only cap
    }
    if maxWidth > 0 && requestedWidth > maxWidth {
        return maxWidth
    }
    return requestedWidth
}
```

An explicit `Width` may request *either* a smaller or a larger rendered
size than the image's native pixel dimensions — unlike the *implicit*
`maxWidth` cap (`scaledImageSize`), which only ever shrinks. This mirrors
`Barcode.Height`, which already lets a receipt author request an
arbitrary explicit dimension independent of the barcode's native size;
`render/layout.rasterizeImage` already samples in both directions
(nearest-neighbour, nothing new to add) — see barcode.go's own doc
comment making the identical point for `Barcode`'s width scaling. `Width`
is always clamped to the printable page width when one is known, the same
hard "never wider than the printable area" rule every other raster
element (`Image`, `QRCode`, `Barcode`) already enforces.

`Build`'s height calculation for a `receipt.Asset` (currently `imageHeight`)
resolves the target width via `resolveTargetWidth(e.Width, p.WidthDots)`
first, falling back to `scaledImageSize`'s existing shrink-only cap only
when no explicit `Width` was requested — so a Receipt that never sets
`width` advances `Y` by exactly what it does today, unchanged.

**Painting**, in `render/canvas`: `rasterBitmap` gains a case for
`layout.AlignedAsset`, calling a new exported `layout.
DecodeAlignedAssetBitmap(a layout.AlignedAsset, maxWidth int)
(GlyphBitmap, error)` (the `AlignedAsset` analogue of
`DecodeImageBitmap`) that:

1. Decodes `a.Data` and resolves the target width via the *same*
   `resolveTargetWidth(a.Width, maxWidth)` `Build` used, then
   `rasterizeImage`s to that width — identical output to today's
   `DecodeImageBitmap` when `a.Width == 0`.
2. If `a.Align` is `"center"` or `"right"` and the resolved bitmap is
   narrower than `maxWidth` (there is horizontal room to move it at all):
   left-pads the bitmap with blank pixel columns — `maxWidth -
   resolvedWidth` of them for `"right"`, half that for `"center"` — via a
   small new helper that allocates a `maxWidth`-wide `GlyphBitmap` and
   copies the narrower bitmap's set bits in at the computed column
   offset, leaving every other bit unset (blank/white). This is a
   pixel-domain sibling of `alignPad`, not a new alignment *concept*: the
   "leading blank space" idea, expressed as pixel columns instead of
   glyph advances.
3. `a.Align` empty/`"left"`, or `maxWidth <= 0` (no printer configured —
   the same sentinel every other alignment path here already respects):
   no padding, bitmap returned exactly as `rasterizeImage` produced it.

`render/canvas.Paint`'s own painting loop needs exactly one new line — a
`case layout.AlignedAsset` alongside `receipt.Image`/`receipt.QRCode`/
`receipt.Barcode` in `rasterBitmap` and `isRasterElement` — everything
downstream of bitmap resolution (the `c.paintGlyph(0, b.Y, bitmaps[i])`
call in `Paint`'s painting loop) is unchanged, because the alignment is
already baked into the bitmap `rasterBitmap` returns, the same way a
right-aligned `Text` line's padding is already baked into the string
`textContent` returns.

**Alignment reference width is the page, not the image's own footprint.**
This is worth calling out explicitly because it differs from
`centerBarcodeCaption`, which centers a caption under *its own barcode's*
rendered width, not the page — an intentional difference, not an
inconsistency: a barcode caption's only job is to sit under its own
barcode, while `Asset.Align` answers "where on the receipt does this
picture sit," which only makes sense relative to the full printable
width (`doc.WidthDots`/`maxWidth`), the same reference `Text.Align`
already aligns against. Margins
(`printer.Profile.MarginLeftDots`/`MarginRightDots`) are not subtracted
from that reference width, for the same reason `wrapText`'s own doc
comment already gives for text wrapping: "Profile's own doc comment
defers usable-width arithmetic to 'a later layout slice'" — this decision
does not reopen that deferral, it stays consistent with it.

### Validation

- `Text.Validate()` and `Asset.Validate()` both gain the same closed-enum
  check `Divider.Validate()` already performs for `Style`: reject
  anything other than `""`/`"left"`/`"center"`/`"right"` with a plain
  error.
- `Asset.Width`'s existing "must not be negative" check
  (`Asset.Validate()`) is unchanged — `0` continues to mean "no explicit
  width," now with an actual rendering effect once implemented, rather
  than always meaning "unused field."
- No new I/O-backed validation is introduced. Clamping `Width` to the
  printable page width happens in `Build` (`resolveTargetWidth`), not in
  `Validate()`, for the same reason existence-checking an `Asset.Name`
  already happens in `Build` and not `Validate()`
  (`docs/ARCHITECTURE.md` §3): `Validate()` has no `printer.Profile` to
  clamp against, and is documented to stay fast, local, and
  I/O/context-free.

## Consequences

- Closes the last open gap `docs/adr/0007-bitmap-text-styling.md` left
  for `Align`, and the `Asset.Width` gap flagged when Milestone 3's
  Divider/Barcode work reported it as blocked. `docs/ARCHITECTURE.md` §3's
  element table, "Text styling" section, and "Image vs. Asset" section
  all need updating in the implementing slice to describe the new,
  concrete behavior instead of "accepted but not yet rendered."
- Every existing Receipt that never sets `align` or `width` renders
  byte-for-byte identically to today: `Text.Align` defaults to `""` (no
  padding applied — `alignPad` is a no-op), and `Asset.Width` defaults to
  `0` (`AlignedAsset` behaves exactly like today's `receipt.Image`
  lowering). No breaking change to rendered output for any Receipt that
  doesn't use these fields, only to Receipts that previously set an
  arbitrary/invalid `align` string, which now fails validation instead of
  silently doing nothing.
- `receipt.Asset` no longer lowers to `receipt.Image` — a small new
  `render/layout.AlignedAsset` type is introduced. Unlike `TableLine`/
  `ColumnsLine`/`BarcodeCaption` (layout-synthesized content with no 1:1
  element counterpart), `AlignedAsset` is a *resolved-content carrier*:
  the same one `Asset`'s own already-declared fields, forced to be a
  distinct type only by the `assets.Store` I/O-boundary invariant (§4),
  not by "carrier types are this project's default answer for extra
  fields." See "Why a new type" above for the dividing line this draws
  for future render-time metadata on other element types.
- `centerBarcodeCaption` is refactored into a call to the new shared
  `alignPad`, so `render/layout` ends up with one leading-space-padding
  implementation instead of two. This refactor is part of implementing
  this decision, not a separate drive-by cleanup — the whole point of
  generalizing `Align` is reusing the technique `centerBarcodeCaption`
  already proved out.
- `render/canvas.Paint`'s core painting loop, `Block`, `Canvas`, and
  `Document` are all unchanged. The only new code in `render/canvas` is
  one more `rasterBitmap`/`isRasterElement` case, following the exact
  shape the `receipt.Image`/`receipt.QRCode`/`receipt.Barcode` cases
  already have.
- `Heading` deliberately has no `Align` field. Authors requiring aligned
  heading-style text should compose it from `Text` using `Bold: true`,
  `Size: 2`, and the desired `Align` value. This preserves ADR-0007's
  "Heading is presentation sugar over Text" design and avoids creating a
  second styling surface.
- A future proportional-font `Font` implementation (`docs/ARCHITECTURE.md`
  §8 "New Font implementation") would need `alignPad`'s space-counting
  loop revisited — the same caveat `centerBarcodeCaption`'s own doc
  comment already carries today, now shared by one function instead of
  needing to be repeated for a second one.

## Alternatives considered

- **Add a real horizontal coordinate to `Block`** (e.g. `Block.X int`,
  a general pixel-offset primitive `render/canvas.Paint` would apply when
  blitting any Block): rejected. This directly contradicts the explicit,
  deliberate "no horizontal-position primitive" stance already recorded
  for `Columns` in `docs/ARCHITECTURE.md` §3 and flagged in §11 as a
  materially bigger, deliberately deferred change ("Supporting arbitrary
  nested content side by side would require a real horizontal-positioning
  primitive on `Block`/`Canvas`"). Padding achieves the identical visible
  result without touching the frozen `Block`/`Canvas` shape, and is
  already proven three times over (`Table`, `Columns`, `Barcode` caption)
  rather than being a new mechanism introduced just for this. (A
  narrower, `Asset`-specific version of this idea — `Width`/`Align` as
  fields on `Block` rather than a general `X` coordinate — was also
  considered separately; see below.)
- **Let `render/canvas.Paint` read `receipt.Text` fields directly**
  instead of pre-resolving alignment in `Build`: rejected for `Text` — this
  is exactly the "`Paint` never inspects `receipt.Text` or
  `receipt.Heading` fields to decide how to style a Block — only
  `Block.Style`" invariant `docs/ARCHITECTURE.md` §3 already establishes.
  `Text.Align` needs no I/O resolution, so in principle `Paint` reading
  `Text.Align` directly (the way it already reads `Divider.Style`/`Size`)
  was a live option — rejected here specifically because alignment is
  expressed as *content* padding (leading spaces baked into the string),
  and only `Build` has the wrapped-line boundaries to pad correctly; there
  is no equivalent of `Divider`'s "no resolution needed at all" shortcut
  once wrapping is involved.
- **Preserve `receipt.Asset` unresolved through to `Paint`**, reading
  `Width`/`Align` off it directly at paint time (the same way `Paint`
  already reads `receipt.Divider.Style`/`.Size` with no wrapper type):
  examined in detail — see "Why a new type" above — and rejected
  specifically because `Asset`, unlike `Divider`, requires a `Build`-side
  I/O resolution step (`Name` → pixel bytes via `assets.Store.Get`) that
  `docs/ARCHITECTURE.md` §4 documents as exclusive to `Build` ("layout is
  the only stage that talks to the outside world"). `receipt.Asset` has
  no field for resolved bytes and must not gain one without blurring the
  "Image means bytes, Asset means a name to resolve" distinction
  `docs/ARCHITECTURE.md` §3 "Image vs. Asset" already establishes as the
  reason the two types are kept separate. Some distinct carrier for the
  resolved bytes is therefore unavoidable regardless of where `Width`/
  `Align` end up living — this is the actual reason `AlignedAsset` (or
  something shaped like it) is required, not "carrier types are this
  project's default answer for extra fields" (see "Why a new type"
  above for the fuller argument, including why this reasoning does *not*
  generalize into "every element should get a wrapper type").
- **Carry `Width`/`Align` as generic metadata fields on `Block` itself**
  (e.g. `Block.Width int`, `Block.Align string`), keeping the `Element` a
  plain `receipt.Image`: this was the strongest alternative considered —
  it would remove the need for `AlignedAsset` entirely, since `Data`
  alone (already on `receipt.Image`) is the only reason a distinct
  `Element` type is otherwise needed. Rejected for two independent
  reasons: (1) it changes `Block`'s frozen `{Y, Element, Style}` shape,
  referenced verbatim as a load-bearing invariant in multiple places in
  this codebase, and is exactly the "design an actual `X` concept onto
  `Block`/`Canvas`" move `docs/ARCHITECTURE.md` §11 already earmarks as a
  deliberate, complete, future decision — not something to do
  incrementally and narrowly under a different name; (2) it generalizes
  `Block` before a second real use of per-element render-time metadata
  exists, leaving every non-`Asset` `Block` variant carrying two fields it
  never reads — worse for clarity than a small type used only where its
  fields are always meaningful, and the wrong direction to resolve the
  "won't scale" concern this alternative was meant to address.
- **Give `receipt.Image` a `Width`/`Align` too, instead of a new
  `layout.AlignedAsset`**: rejected. The element schema table
  (`docs/ARCHITECTURE.md` §3) deliberately gives these fields only to
  `Asset`, not `Image`; growing `Image`'s public JSON schema to carry
  fields no `image` element has would be an unrelated, unrequested public
  API change, and would force every ordinary `Image` Block through the
  same "was this really an Image, or an Asset wearing an Image's face"
  ambiguity `TableLine` et al. already exist specifically to avoid.
- **Percentage-based `Width`** (e.g. `"50%"`): rejected for this slice.
  Every dimension field in this schema today (`Text.Size`,
  `Divider.Size`, `Barcode.Height`, `printer.Profile.WidthDots`) is
  either an integer scale factor or dots; a percentage would be the first
  non-integer, non-dots sizing concept in the schema, and immediately
  reopens "percentage of what — the full printable width, or a
  margins-adjusted width?" — a bigger question this slice doesn't need to
  answer to close the `Width`/`Align` gap. Dots keeps `Asset.Width`
  consistent with every existing dimension field.
- **`Asset.Width` limited to shrinking only**, matching the implicit
  `maxWidth` cap's existing behavior: considered, then rejected in favor
  of allowing explicit upscaling (clamped to the page width) — an
  explicit `Width` is a deliberate request, the same reasoning
  `Barcode.Height` already established for its own explicit dimension
  field, and `rasterizeImage` already supports both directions with no
  new sampling code required.
- **Aligning a whole multi-line `Text` element as one block** (padding
  computed once across all wrapped lines, e.g. against the longest line's
  width) instead of per wrapped line: rejected — it produces a visibly
  wrong result the moment lines have different natural widths (a
  right-aligned paragraph's shorter lines would hang left of the longest
  line's right edge instead of lining up their own right edges), and
  `TableLine`/`ColumnsLine` already establish "each composed line is
  independently positioned" as this codebase's convention.
