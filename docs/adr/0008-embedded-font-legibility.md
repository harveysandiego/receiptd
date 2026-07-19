# 0008. Doubling the embedded font's native resolution for legibility

Status: Accepted

## Context

An end-to-end hardware test (Milestone 3, printing a receipt exercising
every `Text` style and `Size` combination on a real 80mm/203 DPI thermal
printer) showed `Style.Size: 1` (or omitted) — `render/layout.EmbeddedFont`'s
raw, unscaled output — to be almost unreadable on the physical printout,
even though the same content looked fine in an on-screen PNG preview.
`EmbeddedFont` wraps `golang.org/x/image/font/basicfont.Face7x13`
directly: 7x13 dots at 203 DPI is about 0.87mm x 1.6mm per glyph, too
small to read reliably on paper.

The fix needed to raise legibility of the *default* case — most
`Text` content has no `Size` set at all — without breaking the
documented contract of `Size` itself
(`docs/adr/0007-bitmap-text-styling.md`): `0`/omitted and `1` both mean
"unscaled," `2` is exactly double, and so on, computed as an integer
multiple of `EmbeddedFont`'s own native glyph. A client that already
sends `Size: 2` expecting "twice whatever Size 1 looks like" must keep
getting exactly that.

## Decision

**`render/layout.EmbeddedFont` bakes a fixed 2x nearest-neighbour
upscale into its own native glyphs**, applied inside `Measure`,
`LineHeight`, and `Glyph` before `render/canvas`'s `Style.Size` scaling
ever sees the result. `EmbeddedFont`'s native glyph is now 14x26 dots,
not `basicfont.Face7x13`'s raw 7x13.

This is a private implementation detail of `EmbeddedFont`, not a change
to any documented interface:

- `render/layout.Font`'s contract (`Measure`/`LineHeight`/`Glyph`) is
  unchanged — it never promised a specific resolution, only that it
  answers those three questions consistently.
- `Style.Size`'s meaning is unchanged: still an exact integer multiple
  of `EmbeddedFont`'s own native glyph, still `0`/`1` = unscaled. Only
  what "unscaled" now looks like in dots has changed.
- `receipt.Text.Size`'s public JSON schema, validation, and every
  existing valid value are untouched — a client sending `Size: 2` today
  gets a visually bigger result than before, but the *ratio* between
  `Size: 1` and `Size: 2` content is identical to before this change.

The upscale itself reuses the exact nearest-neighbour algorithm
`render/canvas.scaleGlyph` uses for `Style.Size` — each source pixel
becomes an exact `2x2` block, no interpolation — duplicated locally in
`render/layout` (as `upscale`) rather than imported from
`render/canvas`, because `render/layout` sits below `render/canvas` in
the package dependency order (`docs/ARCHITECTURE.md` §11) and must not
import it.

## Consequences

- Every `Text`/`Heading` block prints roughly twice as large, in dots,
  as it did before this change, at every `Size` value — including the
  common case of `Size` omitted entirely. This changes line-wrapping
  points (`Font.Measure` values doubled) and vertical space consumed
  (`Font.LineHeight` doubled) for any already-authored `Receipt`, though
  no existing JSON becomes invalid or needs to change.
- `docs/ARCHITECTURE.md` §3's description of the embedded face's native
  resolution needed updating to describe this two-step relationship
  (raw `basicfont.Face7x13` → `EmbeddedFont`'s baked-in native glyph →
  `Style.Size`'s integer multiple of that), rather than treating "the
  embedded face" as a single fixed 7x13 fact.
- If a future hardware test finds even 14x26 too small (or too large),
  the fix is the same shape: adjust `nativeScale` in
  `render/layout/embedded_font.go`, not `Style.Size`'s semantics.

## Alternatives considered

- **Renumber what each `Size` value maps to** (e.g. `Size: 1` internally
  resolves to today's 2x scale, `Size: 2` to today's 3x, and so on):
  rejected. This breaks the frozen `docs/adr/0007-bitmap-text-styling.md`
  contract that `Size: 1`/omitted means literally unscaled — a client
  reasoning about `Size` as "an exact integer multiple of the base
  glyph" would get a different answer than the schema documents, for no
  benefit over changing the base glyph itself.
- **Leave the renderer unchanged; document that `Size: 1` is small and
  receipt authors should default to `Size: 2`**: rejected as the
  primary fix (though still good practice) — it pushes a hardware
  legibility problem onto every client instead of fixing it once, and
  the *default* case (`Size` omitted) would stay hard to read for any
  client that doesn't know to compensate.
- **Switch to a different, larger embedded bitmap face** instead of
  upscaling `Face7x13`: rejected as unnecessary complexity for this
  problem — nearest-neighbour upscaling `Face7x13` produces the same
  "exact integer multiple, no interpolation" pixel character this
  renderer already commits to (`docs/adr/0002-raster-rendering.md`),
  without sourcing or maintaining a second bitmap font asset.
