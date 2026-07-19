# 0012. Lowering the default divider thickness and adding a Size scale factor

Status: Accepted

Supersedes: [0011](0011-divider-thickness-legibility.md)

## Context

ADR-0011 raised `render/layout.DividerThickness` from `1` to `4` dots
after a physical print on the 203 DPI TM-m30II showed a one-dot rule to be
nearly invisible on paper. A further physical print at `4` dots, reviewed
after the `table` element landed, showed the opposite problem: `4` reads
as noticeably heavier than intended for a default rule, closer to a bar
than a line. Unlike ADR-0011's finding, this isn't something the on-screen
PNG preview hides — it was visible in both preview and print — it's a
judgment call about the right default weight, which ADR-0011 made without
the benefit of comparing it against real printed output at a few
different thicknesses.

Separately, nothing in the schema let a receipt author ask for a
deliberately heavier rule (e.g. to set off a total line) without
generating multiple adjacent `divider` elements — an awkward workaround
that also multiplies `Y` advancement rather than producing one clean
thicker line.

## Decision

`DividerThickness` is lowered from `4` to `2` dots (about 0.25mm at 203
DPI) — thin but legible, splitting the difference between ADR-0011's two
data points rather than picking either extreme.

`receipt.Divider` gains a `Size int` field, following the exact
"0 or omitted means unscaled" convention `Text.Size` already established
(`docs/adr/0007-bitmap-text-styling.md`): a rendered `Divider` occupies
`DividerThickness * Size` dots, so `Size: 2` reproduces roughly ADR-0011's
old default weight for a receipt author who wants it, without a second
scaling mechanism alongside `Text.Size`'s.

Unlike `Text.Size`, a `Divider`'s `Size` does not flow through
`Block.Style` — `render/layout.Build` and `render/canvas.Paint` both read
`receipt.Divider.Size` directly off the `Element`, resolved via the same
`ResolveSize` helper `Text.Size` uses (now exported from `render/layout`
for this second use). `Block.Style` is documented as text-rendering hints
(`docs/ARCHITECTURE.md` §3 "Text styling"); a `Divider`'s line thickness
isn't a text-styling concept, so folding it into `Style.Size` would
conflate two different scale factors that happen to share the same "0/1
means unscaled" arithmetic. This mirrors how `receipt.Spacer.Height` is
already read directly rather than through `Style`.

## Consequences

- Every already-authored `divider` element (no `size` field) now renders
  at `2` dots instead of `4` — half as tall as ADR-0011 shipped, though
  still twice ADR-0011's original `1`. No JSON becomes invalid; only
  shorter on paper.
- A receipt author can opt into a heavier rule per-`divider` via `size`,
  without a new element type or a second styling mechanism.
- `render/layout.ResolveSize` (renamed and exported from the former
  unexported `resolveSize`) is now the single place both `Text.Size` and
  `Divider.Size` resolve a `0`-or-omitted value to `1` — `render/canvas`
  calls it directly rather than duplicating the same three-line floor.

## Alternatives considered

- **Keep `4` as the default, add only `Size` for callers who want
  thinner**: rejected — `4` was the wrong default on its own evidence
  (this ADR's Context), not just missing a way to opt out; a receipt
  author who never sets `size` should get a good default, not a value
  this project's own physical testing found too heavy.
- **A `style` value ("thick") instead of a numeric `Size`**: rejected —
  this project already has exactly one integer-scale-factor idiom
  (`Text.Size`), and inventing a second, string-based one for `Divider`
  for the same "make this bigger" need would be a second parallel
  mechanism for no benefit over reusing the first.
- **Fold `Divider.Size` into `Block.Style.Size`**: rejected — `Style` is
  specifically the resolved *text*-styling struct
  (`docs/ARCHITECTURE.md` §3); a `Divider` Block already deliberately
  does not read `Style` for anything (ADR-0011), and starting to read it
  only for `Size` would make that boundary inconsistent for no reduction
  in code, since `render/canvas.Paint` still has to type-assert
  `receipt.Divider` to reach `paintHLine` either way.
