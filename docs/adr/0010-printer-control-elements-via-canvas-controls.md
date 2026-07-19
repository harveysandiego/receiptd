# 0010. Positioned printer-control elements carried via Canvas.Controls

Status: Accepted

## Context

`docs/ARCHITECTURE.md` §3's element table always listed `feed` and `cut`
as element types, positioned anywhere in a Receipt's `elements` array
like any other element — not restricted to appearing only at the very
end. `escpos.Encode` (ADR-0002) already emitted one automatic feed+cut
pair at the end of its output, driven entirely by `printer.Profile`. §4
step 8d further specifies that an explicit trailing `cut` should suppress
that automatic one — implying `Encode` needs *some* signal about the
source Receipt's structure, not just the painted `Canvas`.

Implementing `receipt.Feed`/`receipt.Cut` therefore needed a way to carry
"an ESC/POS command belongs at this position in the output" from
`render/layout.Build` (which knows the Receipt's element order) through
to `escpos.Encode` (the one place, per ADR-0002, that produces
printer-specific bytes) — without a `Canvas`, a pure 2D bitmap, gaining
any pixels for something that paints nothing.

## Decision

`render/canvas.Canvas` gains a `Controls []Control` field alongside its
existing `Width`/`Height`/`Bits`:

```go
type Control struct {
    Y        int
    Element  receipt.Element // always a receipt.Feed or receipt.Cut
    Terminal bool
}
```

`render/canvas.Paint` appends one `Control` per `receipt.Feed`/
`receipt.Cut` `Block` it walks, in `Document` order, at that Block's own
`Y` (the same dot-row coordinate space `Canvas` rows already use) — the
same pass that positions every other element, so no second traversal is
needed. `Terminal` is set when that Block was the very last one in the
`Document`: the exact fact step 8d's "the Receipt didn't end with an
explicit cut" needs, resolved once here rather than re-derived from Y
alone (a trailing `Feed` and a trailing `Cut` can share the same Y, since
neither advances it).

`escpos.Encode`'s signature is unchanged
(`Encode(c *canvas.Canvas, profile printer.Profile)`); it now reads
`c.Controls` to split its raster-band output at each entry's Y and emit
that entry's command bytes there, and to decide whether the automatic
trailing feed+cut is redundant. `canvas.EncodePNG` ignores `Controls`
entirely — a printer-control command has no pixels to preview.

## Consequences

- `render/escpos` gains a dependency on `receipt` (to distinguish `Feed`
  from `Cut`), which it didn't need before — legal under the dependency
  order in `docs/ARCHITECTURE.md` §1 (`receipt` sits above `escpos`), and
  that section's package list and dependency graph are updated in this
  same change to reflect it.
- A `Control` never affects `Canvas.Width`/`Height`: `render/layout.Build`
  advances `Y` by zero for `Feed`/`Cut`, matching their element-table
  meaning of "no raster footprint."
- A `Feed` or `Cut` positioned mid-document forces a raster-band boundary
  at its `Y` regardless of `profile.MaxImageHeightDots` — a raster
  command's data can't have a command byte sequence spliced into the
  middle of it, so every `Control` ends whatever band was in progress.
- The suppression decision (skip the automatic trailing feed+cut when the
  Receipt already ends with an explicit `Cut`) stays entirely inside
  `escpos.Encode`, not the caller (`app.Service.Process`) — keeping
  ADR-0002's "the one place printer-specific byte sequences are produced"
  true for this decision too.

## Alternatives considered

- **Pass `layout.Document` to `escpos.Encode` directly**, instead of
  adding `Canvas.Controls`: rejected. It couples `Encode` to `Document`'s
  full shape (`Font`, `WidthDots`, `Style`) for a need that only ever
  reads a `Block`'s `Y` and `Element` — no more than `Control` already
  captures — and doesn't reduce anything `Control` doesn't already do:
  `Encode` still has to walk and filter for `Feed`/`Cut`.
- **Resolve suppression in `app.Service.Process`**, passing a bool into
  `Encode`: rejected — splits one ESC/POS-byte-sequence decision (does
  this job get a trailing cut) across two packages for no benefit, and
  `app` has no other reason to know anything about cut semantics.
- **Split `Canvas` into multiple `Canvas` segments at each control
  boundary**, returned as a slice from `Paint`, instead of one `Canvas`
  plus a `Controls` list: rejected. Breaks the one-`Canvas`-per-`Document`
  artifact `EncodePNG`/preview already depends on, and would duplicate
  `escpos`'s own `MaxImageHeightDots` chunking at the `canvas` layer for
  no benefit.
