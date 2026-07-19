# 0002. Raster-first rendering over native ESC/POS commands

Status: Accepted

## Context

ESC/POS is not one protocol but a family of vendor dialects. Native text
printing depends on the printer's configured codepage, which varies by
model and firmware and complicates anything beyond basic ASCII. Native QR
code and barcode commands vary meaningfully between vendors (and even
between printer families from the same vendor). Supporting native ESC/POS
text, QR, and barcode commands robustly across "any ESC/POS-compatible
printer" would mean maintaining a matrix of vendor-specific command
variants and codepage tables — real, ongoing complexity directly opposed
to the project's goal of a small, maintainable codebase.

The one printer available for testing during design and early
implementation is a single Epson TM-m30II (203 DPI, 58/80mm switchable,
Ethernet/USB/Bluetooth/Wi-Fi capable). This is supporting evidence for,
not merely neutral to, the raster-first decision: with only one physical
printer to validate against, a rendering strategy that minimizes
printer-specific native command usage also minimizes the number of things
that can only be verified on hardware that isn't available yet (a second
vendor's printer). Nearly all ESC/POS-compatible printers, across vendors,
support *some* form of raster image printing (`GS v 0`-style commands) —
this is a much safer cross-vendor assumption than native text/QR/barcode
command compatibility.

## Decision

Everything that ends up on paper — text, headings, dividers, QR codes,
barcodes, images, tables — is painted onto a monochrome bitmap
(`render/canvas.Canvas`) using one embedded font
(`render/layout.Font`), then sent to the printer as a raster image via
ESC/POS raster commands. The only genuine ESC/POS *commands* used are
initialization, feed, and cut — everything else is pixels.

Text rendering therefore never depends on the printer's codepage: glyphs
are painted from the embedded font, not sent as codepage-dependent bytes.
QR codes and barcodes are generated as bitmaps (via `boombuler/barcode`,
see `docs/adr/0009-barcode-symbologies.md` for the symbologies `barcode`
supports) and painted onto the canvas like any other graphic, rather than
emitted via vendor-specific native QR/barcode ESC/POS
commands.

`render/escpos.Encode(canvas, profile)` is the one place printer-specific
byte sequences are produced, and it is Profile-aware (paper width,
DPI-driven scaling, chunking for `Profile.MaxImageHeightDots` if a given
printer needs it — see the note on premature optimization below).

## Consequences

- One rendering path handles every Element type and every printer that
  supports raster printing — no per-vendor branch for QR/barcode/text
  commands.
- Internationalization and font rendering are solved once, in
  `render/layout`/`render/canvas`, rather than depending on each printer's
  configured codepage.
- Print jobs are somewhat larger over the wire/USB/network than the
  equivalent native-text ESC/POS bytes would be, since a rasterized line
  of text is pixels, not a handful of character bytes. This is judged
  acceptable given target use cases (short receipts, local network or USB
  connection, not high-volume commercial printing).
- `render/canvas` and `render/escpos` need real hardware to validate
  against at least once (Milestone 3) — the golden-byte and golden-image
  tests approximate this in CI, but nothing replaces an actual printed
  receipt from the TM-m30II before considering this pipeline proven.
- `Profile.MaxImageHeightDots`-driven chunking in `escpos.Encode` is a
  configuration knob today but should ship as a no-op (single raster
  block) until testing against the real TM-m30II demonstrates chunking is
  actually necessary — building that logic speculatively would be
  optimizing against a guess rather than an observed hardware constraint.

## Alternatives considered

- **Native ESC/POS text/QR/barcode commands, raster only as a fallback**:
  rejected — this is the more "efficient" path bandwidth-wise, but it
  reintroduces exactly the codepage and vendor-command matrix problem this
  decision exists to avoid, for a marginal efficiency gain that doesn't
  matter at the scale of a home/homelab receipt printer.
- **A third-party ESC/POS library**: considered and rejected in favor of a
  hand-rolled encoder. Existing Go ESC/POS libraries generally assume the
  native-command model this decision deliberately avoids, and a
  raster-first encoder is small enough (init/feed/cut + one raster command
  family) that hand-rolling it with a golden-byte test suite is less
  overall complexity than adapting a general-purpose library to a
  raster-only usage pattern.
- **Requiring a specific printer model rather than "any ESC/POS-compatible
  printer"**: rejected as too narrow for the project's stated goals; the
  TM-m30II is the *test* hardware, not the *target* hardware.
