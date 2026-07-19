package layout

import "github.com/harveysandiego/receiptd/internal/receipt"

// barcodeModuleWidth is the width, in dots, each encoded module (the
// narrowest bar or space unit) renders at before any printable-width
// scaling — chosen so even a short symbology (e.g. an EAN-8's ~67
// modules) is wide enough to be legible at typical thermal-printer DPI,
// the same "legible by default" reasoning
// docs/adr/0008-embedded-font-legibility.md applies to the embedded font.
const barcodeModuleWidth = 2

// barcodeWidth returns the width, in dots, a barcode with nativeWidth
// encoded modules renders at: nativeWidth scaled by barcodeModuleWidth,
// capped to maxWidth when positive (Build's documented "no printer
// configured" sentinel otherwise) — the same "shrink to fit the
// printable width, never enlarge" rule scaledImageSize already applies
// to Image and QRCode, but width-only. Unlike a QRCode's uniform square
// scale, a Barcode's height (its configured bar thickness, see
// barcodeHeight) is independent of how many modules its content encodes
// into, so capping width here must never also shrink height — this is
// why Barcode gets its own sizing helper rather than reusing
// scaledImageSize directly.
func barcodeWidth(nativeWidth, maxWidth int) int {
	width := nativeWidth * barcodeModuleWidth
	if maxWidth > 0 && width > maxWidth {
		return maxWidth
	}
	return width
}

// barcodeHeight returns b's rendered height in dots: b.Height, or
// receipt.DefaultBarcodeHeight when unset.
func barcodeHeight(b receipt.Barcode) int {
	if b.Height > 0 {
		return b.Height
	}
	return receipt.DefaultBarcodeHeight
}

// barcodeDimensions returns b's rendered width and height (see
// barcodeWidth, barcodeHeight), additionally attempting b's real encode
// (b.Encode) purely to surface an encoding error at Build time rather
// than only discovering it later in GenerateBarcodeBitmap — the same
// "Build fails fast on content it can't lay out" precedent
// qrCodeDimensions already sets for QRCode.
func barcodeDimensions(b receipt.Barcode, maxWidth int) (width, height int, err error) {
	bc, err := b.Encode()
	if err != nil {
		return 0, 0, err
	}
	return barcodeWidth(bc.Bounds().Dx(), maxWidth), barcodeHeight(b), nil
}

// GenerateBarcodeBitmap generates b's barcode as a GlyphBitmap — the same
// 1bpp pixel representation DecodeImageBitmap and GenerateQRCodeBitmap
// already produce, so render/canvas.Paint paints a Barcode Block with the
// exact same paintGlyph primitive it already paints Image and QRCode
// Blocks with (docs/ARCHITECTURE.md §4: exactly one bitmap-painting
// path). Generation happens here, at layout time, mirroring
// GenerateQRCodeBitmap.
//
// b.Encode returns the barcode at its native resolution — one pixel per
// module, one pixel tall (github.com/boombuler/barcode's own
// one-dimensional-code convention) — with no scaling of its own; the
// exact same rasterizeImage helper GenerateQRCodeBitmap and
// DecodeImageBitmap already use both up- and down-samples that native
// image to barcodeWidth(...) x barcodeHeight(b) via nearest-neighbour —
// stretching its single source row down the full target height paints
// each module as a solid vertical bar — so there is no barcode-specific
// pixel-plotting or scaling logic anywhere in this package.
func GenerateBarcodeBitmap(b receipt.Barcode, maxWidth int) (GlyphBitmap, error) {
	bc, err := b.Encode()
	if err != nil {
		return GlyphBitmap{}, err
	}
	width := barcodeWidth(bc.Bounds().Dx(), maxWidth)
	height := barcodeHeight(b)
	return rasterizeImage(bc, width, height), nil
}
