# 0014. Lists: a single `list` element for bulleted, numbered, and checkbox items

Status: Accepted

## Context

Receiptd's schema has no way to express a list. A client that wants a
bulleted shopping list, a numbered set of steps, or a checkbox-style task
list today has to fake it with a `Text` element per line and a
hand-typed `"- "`/`"1. "`/`"[ ] "` prefix baked into `Content` — which
works, but pushes marker/indent bookkeeping (what's the next number? how
many spaces for a sub-item?) onto every client instead of Receiptd doing
it once.

This gap surfaced while scoping what a plain-ASCII, non-Markdown-parser
approach to "checkboxes, bulleted/numbered lists" would look like as new
Element shapes rather than a text-conversion feature (`docs/adr/0001-receipt-model.md`
names Markdown-to-Receipt conversion as a planned-but-unbuilt input
source; this ADR is deliberately *not* that — it's a new Element type,
independent of whether a Markdown parser is ever built on top of it
later). Two things constrain the design before any field can be chosen:

1. **The embedded font is ASCII-only.** `render/layout.EmbeddedFont`
   wraps `basicfont.Face7x13` (`docs/adr/0008-embedded-font-legibility.md`);
   any rune outside its native range falls back to a replacement glyph
   rather than rendering correctly. A real Unicode bullet (`•`), checkbox
   (`☐`/`☑`), or emoji marker is not renderable today, full stop — this
   is a font-coverage problem, not something this ADR can solve by
   choosing a different JSON shape. Every marker this ADR defines is
   therefore composed entirely from glyphs the embedded face already has.
2. **`render/layout.Block` has no horizontal coordinate**
   (`docs/ARCHITECTURE.md` §3 "Columns"), by explicit, repeated design
   decision (`docs/adr/0013-text-and-asset-alignment.md` reaffirms this
   rather than adding one). Bullets, numbers, checkboxes, and indentation
   all have to be expressed as literal leading characters composed into
   the line's own content — the same technique `Table`, `Columns`, a
   `Barcode`'s caption, and `Text.Align`/`Asset.Align` already use for
   every other positioning need in this schema — not as a new
   coordinate on `Block`.

## Decision

### One `list` element, not three

Unordered, ordered, and checkbox lists are one `receipt.List` element
with a closed-enum `Kind`, not three separate Element types
(`BulletList`/`NumberedList`/`CheckboxList`). All three need the same
`Items`/`Content`/`Indent` shape, the same word-wrapping, and the same
hanging-indent continuation-line behavior — the only thing that varies is
which marker string gets generated per item, a difference of one
generation function's output, not of data shape. This is the same
"closed vocabulary on one field" pattern `Divider.Style`,
`Barcode.Symbology`, and `Text`/`Asset.Align` already establish, applied
here instead of introducing three near-duplicate registry entries, three
render/layout wiring paths, and three golden-test suites for what is
structurally one element (`CLAUDE.md`: a package or type earns a split
by holding code that genuinely varies independently — marker-glyph choice
doesn't clear that bar).

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

`Kind` accepts exactly `""`, `"bullet"`, `"number"`, or `"checkbox"` —
`""` and `"bullet"` are equivalent (omitted defaults to bulleted, the
common case), the identical "empty string is an explicit synonym for the
first named value" pattern `Divider.Style`'s `""`/`"solid"` already uses.
Any other value is rejected by `List.Validate()` with `apperr.KindValidation`,
same as every other closed-enum field in this schema.

`List` carries no `Bold`/`Italic`/`Size`/`Align` fields of its own —
consistent with `Table` and `Columns`, the schema's other flat structured
elements, which also render as plain, unstyled text rather than exposing
`Text`'s per-run styling surface. An author wanting styled list text
composes it from `Text` today, the same answer already given for a
styled/aligned heading (§3 "Text styling").

### Why `Kind` instead of an `Ordered bool` + a `Checked` flag

An earlier sketch used `Ordered bool` on `List` plus an optional
per-item `Checked` flag, treating "numbered vs. bulleted" and
"checkbox vs. plain" as two independent axes. Rejected: it makes
`Ordered: true` with checked items representable — a numbered checkbox
list — which has no real-world rendering convention (GFM task-list items
are always unordered) and would need its own validation rule just to
reject a combination a closed enum never lets exist in the first place.
`Kind` as one closed-vocabulary field is strictly simpler and matches
every other either/or/or field already in this schema.

`ListItem.Checked` is a plain `bool`, not `*bool`: every other boolean
flag in this schema (`Text.Bold`, `Barcode.ShowText`, etc.) is a plain
bool, and there's no nil/false ambiguity to resolve here — `Checked` is
only ever meaningful when `Kind == "checkbox"`, and that combination is
enforced directly by `Validate()` (see "Validation" below), not by
distinguishing "unset" from "false."

### Checkbox representation: ASCII brackets, not Unicode glyphs

A checkbox item renders as `"[x] "` (checked) or `"[ ] "` (unchecked) —
plain ASCII, every character already in the embedded face, and
deliberately the same literal notation GFM task lists themselves use in
raw Markdown (`- [ ] task`, `- [x] done`). This was chosen over a real
Unicode checkbox glyph (`☐`/`☑`) specifically because of the font
constraint in "Context" above: a Unicode glyph would silently render as
a replacement-character box on real hardware, which is worse than an
ASCII approximation that's at least correct. No new drawing primitive is
needed in `render/canvas` — `"[x] "`/`"[ ] "` paint through the exact
same glyph-by-glyph text path every other character in this schema
already uses.

### Bullet and number generation

An unordered (`""`/`"bullet"`) item's marker renders as a hyphen (`"-"`).
A numbered (`"number"`) item's marker is its 1-based position, in order,
across the *whole* `Items` slice — **flat and sequential, independent of
`Indent`.** An indented item in a numbered list still receives the next
sequential number rather than restarting a per-level count, because
`Indent` is a nesting-level marker only, not a hierarchy (see "Nesting"
below) — there is no "siblings at this level" concept to number relative
to. This is a deliberate simplification, not an oversight: the classic
nested `1, 2, 2a, 2b, 3` numbering convention needs a real parent/child
structure, which is exactly the "second real use" this ADR declines to
build ahead of a demonstrated need (see "Nesting").

Both markers are resolved entirely during layout, before painting —
never by `render/canvas` — the same division of responsibility every
other positioning decision in this schema already follows (§4:
`render/canvas.Paint` only paints an already-resolved layout).

### Indentation is a semantic nesting level, not a space count

`ListItem.Indent` (0 = top level, the common case) is a **semantic
nesting depth**, not a literal count of characters: `Indent: 2` means
"two levels deep," full stop. How much visual space one level occupies
is a rendering choice, not part of the schema's contract — an
implementation is free to pick (and later revisit) the exact offset, as
long as it's applied consistently across a `List`. This ADR fixes only
the invariant that matters architecturally: since `Block` has no
horizontal coordinate (see "Context"), whatever visual offset a level
represents has to be expressed as leading content on the line itself —
composed during layout, using the existing `Font` interface to measure
however much of the available width it consumes — not as a new
coordinate on `Block`, and not as a value carried on `Block` for
`render/canvas` to interpret.

The word-wrap budget available for an item's own content is correspondingly
narrower the more deeply an item is indented, measured the same way this
schema already measures every other width-constrained line (`Table`
columns, `Columns`). A `List` nested past the point where any content
width remains degrades to the narrowest wrapping this schema already
falls back to elsewhere, rather than failing — the same "a narrow
printable width still constrains rather than errors" behavior already
established for `Table`/`Columns`.

`List.Validate()` rejects a negative `Indent` (the same "negative is the
only invalid input" rule `Text.Size`/`Divider.Size`/`Column.Weight`
already use) and enforces a maximum nesting depth as a defensive bound
against pathological input — the same boundary-validation posture
`Barcode.Symbology`'s closed set and `Columns`' own nesting-depth guard
already take: reject a clearly-pathological value explicitly at
`Validate()` rather than letting it silently degrade into an unreadable
receipt. The specific bound is an implementation detail, not part of the
public contract this ADR is freezing, and may be tuned without being a
schema-visible or architectural change.

### Line wrapping and hanging indent

Each item's `Content` word-wraps against its own available width (marker
and indentation already accounted for, per above) using the same
greedy word-wrap this schema already applies to `Text`, `Table` cells,
and `Columns` content. The marker and indentation appear only on an
item's first wrapped line; every continuation line is prefixed with
equivalent blank space instead, so wrapped continuation lines align
under the item's *content*, not under its marker — the standard "hanging
indent" list convention, and a straightforward extension of the same
content-measurement technique this schema already uses elsewhere for
positioning (e.g. alignment, column layout).

This must hold regardless of a marker's own width — a numbered list's
markers aren't all the same width (`"1."` vs. `"10."`) — so the
hanging-indent behavior is defined per item, against that item's own
marker, not a fixed assumed width across the whole list.

### Nesting: a flat `Indent int`, deliberately not a tree

`ListItem` has no `Items []ListItem` (or `Elements []receipt.Element`)
field of its own — no recursive nesting. A flat, semantic nesting-level
integer per item covers the common case a plain-ASCII list author
actually writes (one level of sub-bullets under a parent item) at a
fraction of the complexity a real tree would need: genuine recursive
nesting reopens indent/wrap arithmetic that has to compose correctly to
unbounded depth, much closer in shape to `Columns`' own recursive
`Column.Elements` nesting (and its own nesting-depth guard) than to
anything a flat list needs. There is no demonstrated second use for
arbitrary-depth nesting today — per
`CLAUDE.md`'s "discover interfaces [and fields] at the second real use,
not before," a flat `Indent` is deliberately the answer for v1. This
doesn't foreclose a future recursive design; it just declines to build
one speculatively (see "Alternatives considered").

### List items hold plain text only, not arbitrary Elements

`ListItem.Content` is a plain string, the same "flat, no nested
Elements" design `Table`'s `Headers`/`Rows` already use
(`docs/ARCHITECTURE.md` §3) — not `Elements []receipt.Element`. This
mirrors the reasoning `docs/ARCHITECTURE.md` §3 "Columns" already gives
for rejecting arbitrary nested content in a `Column`: a rendered list
line is one composed line of text, so mixed raster/text content per item
(an `Image` inside a bullet, say) would need a rendering primitive this
codebase deliberately doesn't have. `Table` already proves flat,
string-only content is sufficient for this project's structured-data
elements; `List` is the same category.

### Rendering requires no new Block-level primitive

Rendering a `List` follows the same architectural pattern already
established for `Table`, `Columns`, and a `Barcode`'s caption: `layout.Build`
performs all the semantic expansion — resolving markers, indentation, and
word-wrapping into fully positioned per-line text content — and
`render/canvas.Paint` paints that content through the existing
text-rendering path, unchanged. This requires no new drawing primitive
in `canvas` and no change to `Block`, `Canvas`, or `Document`: positioning
is, as throughout this schema, expressed entirely as already-composed
content rather than a coordinate `Paint` would need to interpret. The
specific internal type(s) layout uses to carry that content are an
implementation choice for the implementing change, not fixed by this
ADR — the architectural guarantee this ADR freezes is the *absence* of
any new rendering primitive, not the shape of the plumbing that achieves
it.

### Validation

- `Kind` — closed enum `""`/`"bullet"`/`"number"`/`"checkbox"`; anything
  else is `apperr.KindValidation`.
- `Items` — at least one, same as `Table.Rows`'s "at least one row"
  requirement.
- `ListItem.Content` — required (non-empty), valid UTF-8. This is
  intentionally *not* restricted to ASCII: exactly like `Text.Content`
  and `Table`'s cells today, any valid UTF-8 string is accepted, and an
  unsupported rune renders via the embedded font's existing
  replacement-glyph fallback rather than failing validation — `List`
  introduces no new restriction here, it inherits the same permissive
  accept / graceful-render-degradation behavior already established
  everywhere else text is accepted into this schema.
- `ListItem.Indent` — must be `>= 0` and no deeper than a defensive
  maximum nesting depth (an implementation detail, not fixed here).
- `ListItem.Checked` — `true` is only valid when the parent `List.Kind ==
  "checkbox"`; `Checked: true` on a bullet or numbered item is rejected
  outright rather than silently ignored, consistent with how this schema
  always rejects an invalid combination explicitly (e.g. an unrecognized
  `Barcode.Symbology`) instead of accepting and quietly discarding it.
- No I/O-backed validation is introduced — `List.Validate()` stays fast
  and local, per `docs/ARCHITECTURE.md` §3's existing rule for every
  `Validate()` method in this schema.

### Extending this later without breaking compatibility

- **A new marker style** (lettered `a. b. c.`, Roman numerals, etc.) is
  purely additive: one new accepted `Kind` string, with the corresponding
  rendering behavior added independently of the schema. `ListItem`
  doesn't change, `render/canvas` doesn't change — the same "closed enum,
  one more accepted value" extension path `docs/adr/0009-barcode-symbologies.md`
  already establishes for `Barcode.Symbology`.
- **A client-chosen bullet character** (e.g. `"•"` or `"*"` instead of
  the fixed `"-"`) is deliberately *not* added now — it would be an
  optional field (e.g. `List.Bullet string`, meaningful only for
  `Kind == "bullet"`) with an empty default preserving today's `"-"`, so
  it's additive whenever a real second use for it shows up. Not building
  it speculatively is the same "second real use" restraint the rest of
  this ADR applies throughout.
- **Real Unicode/icon markers are not a schema question at all.** Nothing
  about `List`'s JSON shape needs to change for a Unicode bullet or
  checkbox to become renderable — that's entirely gated on a future
  proportional/Unicode-capable `Font` implementation
  (`docs/ARCHITECTURE.md` §8, already a named, deferred concern), which
  would only change what layout resolves a marker to, not `List`'s or
  `ListItem`'s fields.

## Consequences

- Closes the "checkboxes, bulleted/numbered lists" half of the original
  question this ADR grew out of, as a first-class Element rather than a
  client-side text-formatting convention. The "emoji" half remains
  explicitly unsolved — by design, it's a `Font` limitation, not
  something `List`'s schema could fix.
- `List` follows the standard Element-addition recipe (a new type in
  `receipt/`, a new `layout.Build` case, rendering that reuses the
  existing text path). No change to `Block`, `Canvas`, `Document`, the
  registry mechanism, or any existing Element type's schema — the same
  architectural guarantee `Table`, `Columns`, and `Barcode`'s caption
  already demonstrate for structurally similar additions.
- `docs/ARCHITECTURE.md` §3 gains a `list` row in the Element types table
  and a new "Lists" subsection, modeled on the existing "Barcode
  symbologies"/"Columns" subsections.
- A `List` with `Kind == "number"` and any indented items numbers them
  sequentially regardless of indent, not per-level — a deliberate
  limitation stated openly, and the natural trigger for reconsidering
  recursive nesting if a real need for hierarchical numbering shows up
  later.
- Nesting depth is defensively bounded to guard against pathological
  input consuming the printable width; there is no mechanism to nest
  *content*, only a visual offset, so this bound is about input hygiene,
  not about limiting real structure. The exact bound is left to the
  implementing change.

## Alternatives considered

- **Three separate Element types** (`BulletList`, `NumberedList`,
  `CheckboxList`) instead of one `List` with a `Kind` enum: rejected —
  near-total overlap in fields, wrapping, and hanging-indent behavior;
  the only real difference is marker-string generation, which doesn't
  clear this project's bar for a type split (`CLAUDE.md`: types earn
  separation by varying independently, not by sharing a JSON
  discriminator with different string values).
- **`Ordered bool` + independent per-item `Checked`** instead of one
  `Kind` enum: rejected — makes nonsensical combinations (numbered
  checkboxes) representable, needing extra validation to reject what a
  closed enum simply never allows to exist.
- **Recursive nested lists** (`ListItem.Items []ListItem`, or generic
  `[]receipt.Element`): rejected for v1 — no demonstrated second use,
  and it's a materially bigger feature (real hierarchical wrap/indent
  arithmetic, closer to `Columns`' recursion and its depth guard than to
  anything a flat list needs) than a fixed per-level space shift. Left
  open as a real future option if a concrete need for arbitrary depth or
  per-level-restarted numbering appears.
- **Arbitrary `receipt.Element` content per item** instead of a plain
  `Content` string: rejected for the same reason `docs/ARCHITECTURE.md`
  §3 already rejects arbitrary nested content in a `Column` — a rendered
  list line carries one composed line of text; mixed raster/text content
  per item needs a rendering primitive this codebase doesn't have and
  isn't building here.
- **Real Unicode marker glyphs** (`☐`/`☑`/`•`) instead of ASCII
  `"[ ]"`/`"[x]"`/`"-"`: rejected for v1 — the embedded font can't render
  them; they'd fall back to a replacement glyph on real hardware. ASCII
  bracket/hyphen notation renders correctly today and matches the raw
  Markdown convention it approximates.
- **A dedicated indent/position field on `Block`** instead of expressing
  indentation as composed leading content: rejected for the same reason
  `docs/adr/0013-text-and-asset-alignment.md` already rejected a general
  `Block.X` coordinate — a bigger, deliberately deferred primitive
  (`docs/ARCHITECTURE.md` §11), where content-composed positioning is
  already proven three times over and costs nothing further to reuse a
  fourth time.
