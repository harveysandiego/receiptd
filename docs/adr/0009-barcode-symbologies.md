# 0009. Fixed set of barcode symbologies for v1

Status: Accepted

## Context

`docs/ARCHITECTURE.md` §3's element table has always listed `barcode`'s
fields as `content`, `symbology`, `height`, `show_text` (Milestone 3's
roadmap entry, §10), but never defined what values `symbology` accepts.
`docs/adr/0002-raster-rendering.md` names `boombuler/barcode` as the
library backing both QR and barcode generation, but only in the context of
the raster-vs-native decision — it says nothing about which of that
library's symbologies (it supports ten: 2 of 5, Aztec Code, Codabar, Code
128, Code 39, Code 93, Datamatrix, EAN-13, EAN-8, PDF417, QR Code) the
`barcode` element should expose.

The architecture defined a `barcode` element but left its `symbology`
field's accepted values unspecified, leaving the public API incomplete:
`symbology` is a wire-format string a client sends and
`receipt.Barcode.Validate()` checks against, so its valid values are public
API, not an implementation detail — the same category of gap
`docs/adr/0007-bitmap-text-styling.md` already closed for `Text.Size`
("any mapping ... would have been invented by that slice, not read from an
existing specification"). This ADR closes the equivalent gap for
`barcode.symbology`, settled once and deliberately, the same way 0007
settled `Size` before implementation.

## Decision

Receiptd v1 supports exactly six barcode symbologies — the ones a
receipt/retail use case actually needs, not the full set `boombuler/barcode`
happens to implement:

| Symbology              | `symbology` value |
|-------------------------|-------------------|
| Code 128                | `code128`         |
| EAN-13                  | `ean13`           |
| EAN-8                   | `ean8`            |
| UPC-A                   | `upca`            |
| Code 39                 | `code39`          |
| Interleaved 2 of 5 (ITF)| `itf`             |

These six lowercase strings are `receipt.Barcode.Symbology`'s complete,
stable set of valid values for v1. No other value — including any other
symbology `boombuler/barcode` itself supports (Codabar, Code 93, Data
Matrix, PDF417, Aztec Code, 2 of 5) — is accepted. `receipt.Barcode.Validate()`
rejects both unrecognized strings and an empty/omitted `Symbology` as
`apperr.KindValidation`, the same pattern `QRCode.Validate()` already uses
for `ErrorCorrection` (`docs/ARCHITECTURE.md` §3, `receipt/qrcode.go`).

Barcode generation stays raster-first, unchanged from
`docs/adr/0002-raster-rendering.md`: whichever symbology is selected is
rendered by `boombuler/barcode` to a bitmap during layout, then flows
through the exact same image-rendering path `QRCode` already established
(`render/layout` generates a `GlyphBitmap`, `render/canvas.Paint` paints it
via the same `paintGlyph` primitive every raster element uses — no
symbology-specific drawing logic in `canvas`). No printer-native ESC/POS
barcode command is used for any symbology; this ADR does not reopen 0002,
it only narrows *which* symbologies reach that same raster path.

## Consequences

- `symbology` has a closed, documented vocabulary a client can validate
  against before ever sending a request, the same guarantee
  `QRCodeErrorCorrectionLevels` already gives `error_correction`.
- The six symbologies cover the common retail/receipt cases (product
  barcodes: EAN-13/EAN-8/UPC-A; logistics and general-purpose: Code
  128/Code 39/ITF) without carrying maintenance surface for symbologies
  this project has no evidence anyone needs (Codabar, Code 93, Data
  Matrix, PDF417, Aztec Code).
- Adding a symbology later is additive (a new string value, a new
  `boombuler/barcode` encoder call) and does not require touching
  `render/canvas` or the raster pipeline at all — the same reason `Barcode`
  can reuse `QRCode`'s established pipeline unchanged.
- A client that needs a symbology outside this set (e.g. Data Matrix) has
  no way to get one from Receiptd v1. Not a goal this decision was trying
  to satisfy — the six chosen symbologies are believed sufficient for the
  project's actual (receipt/retail) use case — but worth naming as a
  capability this decision forecloses rather than one it neutrally leaves
  open, the same honesty `docs/adr/0006-preview-requires-printer-profile.md`
  applies to its own scope narrowing.

## Alternatives considered

- **Expose every symbology `boombuler/barcode` supports**: rejected. This
  would make Receiptd's public API a direct reflection of one dependency's
  implementation surface rather than a deliberate choice — the same
  "inventing public API" problem this ADR exists to avoid, just resolved by
  defaulting to "everything" instead of picking nothing. It also means a
  future library swap (as already happened once for QR generation) could
  silently change what `symbology` values are valid.
- **Free-form `symbology` string, validated only by whether generation
  succeeds**: rejected. This defers the "what's valid" question to runtime
  library behavior instead of documenting it, the same "undefined string
  values" problem `docs/adr/0007-bitmap-text-styling.md` rejected for
  `Text.Size`'s named-size alternative.
- **Auto-detect symbology from `content`'s shape** (e.g. 13 digits implies
  EAN-13): rejected. Several supported symbologies overlap in what content
  they can encode (a numeric string is valid Code 128, Code 39, and ITF
  alike), so detection would be ambiguous or surprising; an explicit
  `symbology` field matches how `ErrorCorrection` and every other
  configuration field in this schema already works — the client states
  intent, Receiptd does not guess it.
