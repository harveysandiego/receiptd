# 0007. Integer bitmap scaling as the public text-styling API

Status: Accepted

## Context

`receipt.Text.Size` was frozen in the original architecture review as an
unconstrained `string`, with its own doc comment stating plainly that
"`docs/ARCHITECTURE.md` does not define a fixed set of valid values for
`Align` or `Size`, so `Validate` does not restrict them." That was a
reasonable placeholder while no renderer read the field at all. It stopped
being reasonable the moment an implementation slice ("bitmap text
styling", Milestone 3) tried to make `Size` actually affect rendering: any
mapping from string to a rendered effect — `"large"`? `"1x"`/`"2x"`/`"3"`?
something else? — would have been invented by that slice, not read from an
existing specification. The slice correctly stopped rather than invent a
public API surface.

This is concrete evidence discovered while implementing, in the same
spirit as `docs/adr/0006-preview-requires-printer-profile.md`: a documented
gap became a blocking one only once real implementation work reached it.

Three things needed deciding before implementation could proceed, all
interconnected:

1. What does `Size` actually mean, expressed as what Go type and what
   valid values?
2. How does `Heading`'s already-documented "bold + large" styling
   (§3) relate to `Text`'s own styling fields, without `Heading` becoming
   a second, parallel rendering path?
3. How do future bitmap transformations (italic, underline,
   strikethrough) fit into the same pipeline as `Size` and `Bold`, without
   requiring a redesign when each one is eventually implemented?

## Decision

**`Size` becomes an integer bitmap scaling factor**, not a point size and
not printer/DPI-dependent:

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

- `0` (omitted) and `1` both mean "unscaled" — the same zero-as-default
  convention `printer.Profile.WidthDots` and `MaxImageHeightDots` already
  use elsewhere in this document, chosen for consistency rather than
  introducing a second "unset" convention.
- `2` means every glyph pixel is painted as an exact `2x2` block, `3` a
  `3x3` block, and so on — nearest-neighbour integer scaling, no
  interpolation.
- Negative values are the only invalid input; `Text.Validate()` rejects
  them as `apperr.KindValidation`, consistent with every other
  `Validate()` in this schema.

`Heading` gains no fields. It is defined as presentation sugar over
`Text`: rendering a `Heading` is equivalent to rendering a `Text` with
`Bold: true, Size: 2` and every other style field at its zero value.
`render/layout.Build` resolves this equivalence once, the same place it
resolves a `Text`'s own style fields — `Heading` never reaches
`render/canvas.Paint` as a distinct styling case.

The renderer gets a single, element-type-agnostic carrier for resolved
styling, `render/layout.Style`, attached to each `Block`:

```go
type Style struct {
    Bold          bool
    Italic        bool
    Underline     bool
    Strikethrough bool
    Size          int // >= 1, always resolved by Build
}
```

`render/canvas.Paint` reads `Block.Style` only — never `receipt.Text` or
`receipt.Heading` fields directly. Styling is layered on top of the base
glyph bitmap `Font.Glyph` returns, as a fixed pipeline: **scale first**
(nearest-neighbour integer, preserving sharp edges), **then bold**
(a deterministic raster technique, e.g. neighbouring-pixel overdraw),
with underline/strikethrough/italic reserved as further steps appended to
the same pipeline when implemented, each operating on the already-scaled
bitmap. `Font` itself stays exactly what it already was — the sole source
of a glyph's unscaled, unstyled base pixels — so this decision touches
nothing in `Font`'s contract.

Measurement is not duplicated for styled text: `Font.Measure` keeps
measuring at native size, and because integer nearest-neighbour scaling is
exact and uniform in both axes, the effective width of a `Size: N` string
is precisely `Font.Measure(s) * N`. Wrapping computes candidate line
widths this way.

`Italic`, `Underline`, and `Strikethrough` join the public schema now,
ahead of their implementation. Only `Bold` renders in the next slice; the
other three validate and round-trip like any other field but currently
have no visible rendering effect — the same position `Align` has already
held since Milestone 1.

## Consequences

- Exactly one text rendering pipeline: `Heading` is genuinely sugar, with
  no divergent painting code path, and adding a future style (underline,
  say) means adding one step to one pipeline, not one case per element
  type.
- Breaking change to `Size`'s JSON type (`string` → `int`). Receiptd is
  pre-alpha with no release (`README.md` "Current status"), the same
  justification `docs/adr/0006-preview-requires-printer-profile.md` used
  for its own breaking change — no installed base to migrate.
- Clients may set `Italic`/`Underline`/`Strikethrough` today and see no
  effect until a later milestone implements them, exactly as already true
  of `Align`. A client integrating now does not need to change its
  request shape again when that milestone lands.
- `Font`'s interface and its one implementation (`EmbeddedFont`) are
  untouched — this decision adds a layer above `Font`, not a parameter to
  it.

## Alternatives considered

- **Named string sizes** (`"normal"`/`"large"`/`"xlarge"`): rejected. This
  is the same undefined-string-values problem being fixed here, just with
  a fixed vocabulary instead of a free one — each new tier still requires
  a schema/documentation change to say what it maps to, and it implies a
  size *concept* (why not `"huge"`?) rather than the exact scaling
  operation the bitmap renderer actually performs.
- **Point size** (e.g. `14`, meaning 14pt): rejected. The embedded font is
  a fixed bitmap face (`docs/adr/0002-raster-rendering.md`), not an
  outline; an arbitrary point size would require either printer/DPI-aware
  conversion (explicitly ruled out — "not printer-dependent") or
  non-integer scaling, which reintroduces interpolation/anti-aliasing this
  decision explicitly avoids.
- **Float scale factor** (e.g. `1.5`): rejected. Nearest-neighbour scaling
  is only pixel-exact for integer factors; a fractional factor would force
  a choice between blurring edges (interpolation) or inconsistent
  per-glyph rounding — both contradict "preserve sharp pixel edges."
- **Per-element-type styling dispatch** (`Paint` switching on
  `receipt.Text` vs. `receipt.Heading` for style, rather than a shared
  `Style` value): rejected. This is exactly the second rendering path
  `docs/adr/0001-receipt-model.md` already rejects on a different axis,
  and it was already flagged as an accepted trade-off worth revisiting in
  the text-wrapping slice that preceded this one.
- **Styling as a `Font` concern** (e.g. `Font.Glyph(r rune, s Style)`):
  rejected. It would make every future style variant part of `Font`'s
  interface contract, contradicts "the bitmap font remains responsible
  only for producing the base glyph bitmap," and would force a second
  `Font` implementation to duplicate scaling/bold logic that has nothing
  to do with glyph sourcing.
