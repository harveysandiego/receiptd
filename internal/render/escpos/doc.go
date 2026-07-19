// Package escpos encodes a canvas.Canvas into ESC/POS bytes: raster
// print commands plus minimal real ESC/POS (init, feed, cut), tailored
// to a printer.Profile. Text, QR codes, barcodes, and images are never
// sent as native ESC/POS commands — see docs/adr/0002-raster-rendering.md
// for why rendering is raster-first. A tall Canvas is split into
// consecutive raster bands no taller than profile.MaxImageHeightDots
// (docs/ARCHITECTURE.md §4 step 8e); a Profile with no limit set produces
// a single raster command, as before.
package escpos
