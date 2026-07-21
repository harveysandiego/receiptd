# 0015. A known-model printer catalogue, not a paper-width heuristic

Status: Accepted

## Context

`config`'s `printers[]` schema (`docs/ARCHITECTURE.md` §7) has always
asked the operator for a flat `width_mm` field, converted directly to
`printer.Profile.WidthDots` via `dots = round(width_mm / 25.4 * dpi)`.
Nothing about the field name or the docs distinguished *paper roll
width* from *printable width* — and on real hardware these are
frequently different numbers.

This surfaced concretely during a hardware test of the new `list`
Element (`docs/adr/0014-list-elements.md`) against a real Epson
TM-m30II on 80mm paper. Configuring `width_mm: 80` (the number printed
on the roll packaging — the only number an operator naturally has on
hand) produced `WidthDots = 639`. The TM-m30II's actual printhead only
addresses a 72mm-wide printable area — `576` dots at 203dpi — with
unprintable margin on both edges of the roll that `margins_dots` cannot
correct for (that field trims further *into* the printable area for
layout purposes; it can't undo a `WidthDots` that's wider than the
hardware itself). The print job did not fail: `escpos.Encode` happily
emitted a 639-dot-wide `GS v 0` raster row, and the printer accepted it
but printed it in visibly disjointed bursts instead of one continuous
feed. Reconfiguring to the printer's real printable width (`576` dots)
fixed it completely.

Two things constrain the fix:

1. **The number operators have on hand is the wrong number.** A paper
   roll is sold and labeled by its *width* (58mm, 80mm), not by its
   printhead's printable area. Asking for `width_mm` and silently
   meaning "printable width" sets up exactly the mistake that caused
   this bug — nothing in the schema or the field name warned that the
   number on the box was the wrong one to enter.
2. **Deriving printable width from roll width by a fixed rule is
   unsafe.** A tempting fix is a heuristic — "80mm paper implies 72mm
   printable" — but this is not universally true. Not every 80mm-roll
   thermal printer clips to a 72mm printhead; some genuinely print the
   full width. Baking in such a heuristic would fix this printer while
   silently reproducing the identical failure on different hardware —
   except now the wrong number would be *invented by Receiptd itself*
   rather than entered by a confused operator, which is a strictly
   worse failure mode: nothing about it would be visible or correctable
   in the operator's own config. This directly conflicts with this
   project's stated position that the server should never assert a
   hardware characteristic it has no way to actually verify.

## Decision

### Two mutually exclusive printer configuration modes

Each `printers[]` entry is configured in exactly one of two ways —
**never both, never neither**:

```yaml
printers:
  - name: front-desk
    model: epson-tm-m30ii     # known model (recommended)
    transport: network
    address: 192.168.1.50:9100

  - name: custom-printer
    transport: network         # custom profile (advanced)
    address: 192.168.1.60:9100
    profile:
      printable_width_mm: 72.02
      dpi: 203
      margins_dots: { left: 0, right: 0 }
      supports_cut: true
      supports_partial_cut: true
      default_cut: partial
      max_image_height_dots: 0
```

`config.PrinterConfig.UnmarshalYAML` rejects the entry outright —
`apperr.KindValidation`, same as every other schema violation this
package reports — if both `model` and `profile` are present, or if
neither is. No precedence or override semantics are defined for the
both-present case: if an operator's config sets both, there is no
reliable way to tell whether they meant to override the model's known
values or misunderstood that the two are alternatives, so this project
would rather reject the config than guess at intent. The full
custom-profile escape hatch already covers everything an override would
have been used for, so nothing is lost by refusing to define one.

### `model:` resolves against a built-in catalogue of verified facts

`internal/printer` gains `ModelProfiles map[string]Profile` — a small,
hand-maintained table where every entry is a `Profile` for hardware
whose characteristics have been independently verified — preferably
through real hardware testing — never derived or guessed from a
heuristic. `config` resolves `model:` by
looking the name up in this table; an unrecognized name is rejected at
load time (`apperr.KindValidation`) with the list of known models in
the error, the same "reject clearly, don't silently degrade" posture
this project already takes elsewhere (e.g. `Barcode.Symbology`'s closed
set). The table lives in `internal/printer`, not `internal/config`,
because it's hardware capability knowledge — the same category of fact
`Profile` itself already exists to hold — and keeps `config`'s job
exactly what `docs/ARCHITECTURE.md` §1 already says it is: the one place
that splits a flat YAML block into the frozen `Connection`/`Profile`
types, not a place that itself knows hardware facts.

`ModelProfiles` starts at exactly one entry, `epson-tm-m30ii` (576 dots /
72mm printable width, 203dpi, full+partial cut support) — the one
printer validated against real hardware so far. It grows only as new
entries' characteristics are independently verified — preferably
through real hardware testing — never added speculatively from a spec
sheet alone.

### `profile:` is the explicit, unambiguous manual path

For any printer not in `ModelProfiles` (or modified from stock), the
`profile:` block is the full manual escape hatch, structurally the same
fields the old flat schema had — with one deliberate rename:
`width_mm` becomes **`printable_width_mm`**. The old name is retired
entirely, not kept as a deprecated alias (`CLAUDE.md`: this project
doesn't carry backwards-compatibility shims when it can just change the
code, and it's pre-1.0 — see "Consequences"). The new name states the
requirement in the field itself, rather than relying on documentation
an operator may not read before their first print: there is no reading
of "printable width in mm" that can be confused with "the paper roll's
width," which is exactly the confusion that caused the bug this ADR
fixes.

### No heuristic anywhere derives printable width from roll width

Nothing in `config` or `printer` ever computes a printable width from a
paper/roll size. Every `Profile` value in the system is either a fact
transcribed once into `ModelProfiles` from validated hardware, or a value
an operator states explicitly in `profile:` — there is no third,
inferred path. This is the specific thing this ADR is turning down (see
"Alternatives considered").

## Consequences

- Configuring a catalogued printer requires zero hardware-specific
  knowledge from the operator — a model name they can read off the
  printer itself or its box, not a millimeter value they'd otherwise
  have to reverse-engineer the way this bug was first found.
- Unlisted or modified hardware still requires the operator to supply
  real facts, but the field name (`printable_width_mm`) now states
  exactly what's needed, closing the specific mistake (roll width
  instead of printable width) this ADR grew out of.
- This is a **breaking change** to the `printers[]` config schema: the
  old flat `width_mm`/`dpi`/`margins_dots`/etc. fields directly on a
  printer entry are no longer valid — they must move under `profile:`
  (with `width_mm` renamed), or be replaced by `model:`. Acceptable
  under `VERSIONING.md`'s 0.x policy, which already treats a breaking
  change as a MINOR bump, not something reserved for a 1.0 boundary. No
  migration shim is provided; every existing deployment's config must
  be updated when upgrading past this change.
- `docs/ARCHITECTURE.md` §7 needs its config schema example and prose
  rewritten to show both modes and explain the mutual-exclusion
  rule, and §1's `printer` package row should mention it now owns
  `ModelProfiles`.
- `ModelProfiles` is a small, flat data table — a map literal, not a new
  registration mechanism — so it introduces no second extension
  mechanism alongside this project's one documented one (compile-time
  registration + blank import, `docs/adr/0004-extension-model.md`); it
  is asserted directly in `internal/printer`, not registered via
  `init()` from elsewhere, and there is no present need for it to be.
- A one-entry catalogue means most real-world printers remain on the
  `profile:` path today. That's an accepted starting point, not a gap
  this ADR needs to close — the catalogue is meant to grow by the same
  discipline it started with — entries added only once their
  characteristics are independently verified, preferably through real
  hardware testing — not by transcribing spec sheets sight-unseen.

## Alternatives considered

- **A fixed paper-width → printable-width heuristic** (e.g. "80mm paper
  implies 72mm printable"): rejected — not universally true across
  vendors, and wrong in the failure direction that matters: it would
  have `receipt` silently asserting a hardware characteristic it cannot
  verify, reproducing this exact bug on different hardware with no
  operator mistake to point to. This is the core alternative this ADR
  exists to rule out.
- **`model:` and `profile:` together, with explicit `profile:` fields
  always overriding the model's defaults**: rejected — introduces
  precedence rules whose only purpose is resolving an ambiguity (did
  the operator mean to override, or misunderstand the schema?) that
  simply doesn't need to exist. Mutual exclusion gets the same
  practical coverage (anyone who needs different values from a known
  model already has the full `profile:` path available) without ever
  having to guess intent.
- **Runtime auto-detection via an ESC/POS status/ID query**: not
  seriously pursued — there is no ESC/POS command for "report your
  printable width in dots" that's reliable and consistent enough across
  vendors for this project's single-transport, network-only v0.x
  `printer` package to depend on. Meaningfully more complexity and more
  failure modes (timeouts, vendor divergence, printers that don't
  implement any such query) than is justified before a second transport
  or a demonstrated need exists — the same "don't build the general
  mechanism before a second real case shows up" restraint already
  applied to `printer.Transport` dispatch and `escpos.Encode`'s
  chunking logic.
- **Keep `width_mm`'s name, just document the printable-width meaning
  better**: an earlier, narrower attempt at this same fix, rejected as
  insufficient — the field name itself invites exactly the mistake that
  caused this bug (an operator reaching for the number on the paper
  roll's packaging), and no amount of documentation guarantees it's
  read before a first print. Renaming the field removes the mistake at
  its source instead of warning against it after the fact.
