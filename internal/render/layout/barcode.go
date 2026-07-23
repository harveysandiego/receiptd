package layout

import "github.com/harveysandiego/receiptd/internal/receipt"

// barcodeModuleWidth is the width, in dots, each module (narrowest bar or
// space) renders at before printable-width scaling — chosen so even a short
// symbology (e.g. EAN-8's ~67 modules) is legible at typical thermal DPI,
// the "legible by default" reasoning of
// docs/adr/0008-embedded-font-legibility.md.
const barcodeModuleWidth = 2

// barcodeWidth returns the width, in dots, a barcode of nativeWidth modules
// renders at: nativeWidth scaled by barcodeModuleWidth, capped to maxWidth
// when positive (Build's "no printer configured" sentinel otherwise) — the
// shrink-to-fit-never-enlarge rule scaledImageSize applies, but width-only.
// A Barcode's height (its bar thickness, see barcodeHeight) is independent
// of module count, so capping width must not shrink height — which is why
// Barcode gets its own helper rather than reusing scaledImageSize.
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

// barcodeDimensions returns b's rendered width and height (see barcodeWidth,
// barcodeHeight), also running b.Encode purely to surface an encoding error
// at Build time rather than later in GenerateBarcodeBitmap — the fail-fast
// precedent qrCodeDimensions sets.
func barcodeDimensions(b receipt.Barcode, maxWidth int) (width, height int, err error) {
	bc, err := b.Encode()
	if err != nil {
		return 0, 0, err
	}
	return barcodeWidth(bc.Bounds().Dx(), maxWidth), barcodeHeight(b), nil
}

// GenerateBarcodeBitmap generates b's barcode as a GlyphBitmap — the same
// 1bpp representation DecodeImageBitmap and GenerateQRCodeBitmap produce, so
// render/canvas.Paint paints a Barcode Block with the one paintGlyph
// primitive (docs/ARCHITECTURE.md §4). Generation happens here at layout
// time, mirroring GenerateQRCodeBitmap.
//
// b.Encode returns the barcode at native resolution (one pixel per module,
// one tall, boombuler/barcode's convention). The shared rasterizeImage
// helper up-/down-samples it to barcodeWidth x barcodeHeight via
// nearest-neighbour, stretching the single source row down the full height
// so each module paints as a solid bar — no barcode-specific scaling logic.
func GenerateBarcodeBitmap(b receipt.Barcode, maxWidth int) (GlyphBitmap, error) {
	bc, err := b.Encode()
	if err != nil {
		return GlyphBitmap{}, err
	}
	width := barcodeWidth(bc.Bounds().Dx(), maxWidth)
	height := barcodeHeight(b)
	return rasterizeImage(bc, width, height), nil
}

// BarcodeCaption is one already-space-padded line of human-readable text
// printed beneath a receipt.Barcode when Barcode.ShowText is true — see
// TableLine for why Build produces a distinct Block-carrying type.
// render/canvas.Paint paints its Content through the same glyph-by-glyph
// path as receipt.Text (canvas.textContent). "Centered" here means
// alignPad's leading-space padding, not geometric alignment — see alignPad.
type BarcodeCaption struct {
	Content string
}

// Validate always succeeds — see TableLine.Validate's identical doc
// comment: BarcodeCaption is never part of a client-supplied
// receipt.Receipt, it exists only as a Block.Element Build itself
// produces.
func (BarcodeCaption) Validate() error { return nil }
