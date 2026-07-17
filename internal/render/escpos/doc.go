// Package escpos encodes a canvas.Canvas into ESC/POS bytes: raster
// print commands plus minimal real ESC/POS (init, feed, cut), tailored
// to a printer.Profile. Text, QR codes, barcodes, and images are never
// sent as native ESC/POS commands — see docs/adr/0002-raster-rendering.md
// for why rendering is raster-first, and docs/ARCHITECTURE.md §11 on why
// Profile-driven image chunking should ship as a no-op until real
// hardware testing proves it necessary.
package escpos
