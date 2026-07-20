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

// BarcodeCaption is one already-space-padded line of human-readable text
// printed beneath a receipt.Barcode's bars when Barcode.ShowText is true —
// the Barcode analogue of TableLine/ColumnsLine (see TableLine's own doc
// comment for why Build produces a distinct Block-carrying type here
// rather than reusing receipt.Text): a Barcode-derived caption Block keeps
// its own identity through layout, the same as every other element type.
// render/canvas.Paint paints a BarcodeCaption's Content through the exact
// same glyph-by-glyph path a receipt.Text Block already uses (see
// canvas.textContent) — this is not a second text-rendering primitive,
// just one more case recognized by the existing one. "Centered" here means
// centerBarcodeCaption's leading-space padding, not a geometric,
// font-independent alignment — see that function's own doc comment.
type BarcodeCaption struct {
	Content string
}

// Validate always succeeds — see TableLine.Validate's identical doc
// comment: BarcodeCaption is never part of a client-supplied
// receipt.Receipt, it exists only as a Block.Element Build itself
// produces.
func (BarcodeCaption) Validate() error { return nil }

// centerBarcodeCaption approximates centering content under width dots of
// bars by left-padding it with as many leading spaces as fit within half
// of (width - f.Measure(content)) — the same "pad with spaces so plain
// text paints as positioned once painted by the ordinary text-glyph path
// starting at x=0" technique tableRowLines/columnsLines already use for
// their own (trailing) padding via padToWidth, applied here to leading
// padding instead.
//
// This is space-padded centering against the embedded font's own fixed
// glyph advance (every rune measures the same width — see
// docs/adr/0008-embedded-font-legibility.md), not a general,
// font-independent text-alignment primitive: the padding is only ever a
// whole number of space-glyph widths, so the result can be off by up to
// half a glyph's advance from true geometric center, and a proportional
// font would need a different implementation entirely (padding by a
// fractional-width space isn't possible with this technique). That
// imprecision is acceptable for a caption whose only job is to read as
// "roughly under the barcode," the same tolerance TableLine/ColumnsLine's
// own space-padding already accepts for column alignment.
//
// content already as wide as or wider than width, or width <= 0 (Build's
// documented "no printer configured" sentinel, see wrapText), is returned
// unchanged — the same "no width to constrain to" fallback padToWidth
// itself applies.
func centerBarcodeCaption(content string, width int, f Font) string {
	contentWidth := f.Measure(content)
	if width <= 0 || contentWidth >= width {
		return content
	}
	pad := (width - contentWidth) / 2
	prefix := ""
	for f.Measure(prefix+" ") <= pad {
		prefix += " "
	}
	return prefix + content
}
