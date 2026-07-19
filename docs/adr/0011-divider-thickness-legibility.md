# 0011. Raising the default divider thickness for hardware legibility

Status: Superseded by [0012](0012-divider-thickness-default-and-scaling.md)

## Context

`render/layout.DividerThickness` shipped as `1` — the finest line a 1bpp
`Canvas` can represent — a deliberate minimum rather than a considered
visual choice, since no numeric thickness is specified anywhere in
`docs/ARCHITECTURE.md` §3. A physical print on the 203 DPI TM-m30II
(the same hardware ADR-0008 tested against) showed a one-dot rule to be
nearly invisible on paper, despite looking like a normal thin line in the
on-screen PNG preview — the same "renders fine on screen, unreadable on
the actual thermal head" gap ADR-0008 found for the unscaled embedded
font.

## Decision

`DividerThickness` is raised from `1` to `4` dots (about 0.5mm at 203
DPI) — a clearly visible rule without reading as a solid filled bar.
`receipt.Divider.Style` ("solid"/"dashed") is unaffected: it was not read
by thickness before this change and still isn't; this only changes how
tall the (currently always solid-rendered) line is.

## Consequences

- Every `Divider` Block now occupies 4 dots instead of 1, shifting the
  `Y` of everything below it by 3 additional dots per divider. This
  changes rendered output for any already-authored Receipt containing a
  `divider` element, the same category of consequence ADR-0008 already
  accepted for `Text`/`Heading` sizing — no JSON becomes invalid, only
  taller on paper.
- `render/canvas.Paint`'s `blockHeight` and `render/layout.Build`'s `Y`
  advancement for `receipt.Divider` both still read the single shared
  `DividerThickness` constant, so the two stages cannot disagree about a
  Divider's height, unchanged from before this ADR.

## Alternatives considered

- **Render "dashed" as a genuinely dashed/patterned line, keeping solid
  at 1 dot**: rejected as the primary fix — dashed-pattern rendering is
  still unimplemented (`docs/ARCHITECTURE.md` §3), and the legibility
  problem exists for the default ("solid") case a receipt author gets
  without opting into anything.
- **Make thickness configurable via `printer.Profile`**: rejected for the
  same reason `docs/adr/0002-raster-rendering.md` ships
  `MaxImageHeightDots` chunking as a day-one no-op — one printer's worth
  of hardware evidence doesn't justify a new configuration knob; revisit
  if a second printer's testing disagrees.
