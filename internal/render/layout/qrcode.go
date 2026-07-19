package layout

import (
	"github.com/boombuler/barcode/qr"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

// qrCodeSize returns the width and height, in dots, q renders at (a QR
// code is always square) — q.Size, or receipt.DefaultQRCodeSize when
// unset, scaled to fit maxWidth per scaledImageSize, the same "shrink to
// fit, never enlarge" rule Image already follows. Pure arithmetic: unlike
// an image, a QR code's rendered size never depends on Content, only on
// Size and maxWidth.
func qrCodeSize(q receipt.QRCode, maxWidth int) int {
	size := q.Size
	if size <= 0 {
		size = receipt.DefaultQRCodeSize
	}
	size, _ = scaledImageSize(size, size, maxWidth)
	return size
}

// qrCodeDimensions returns q's rendered width and height (see
// qrCodeSize), additionally attempting q's real encode (qr.Encode)
// purely to surface a capacity error (content too large for the QR
// standard to represent at q's error-correction level) at Build time
// rather than only discovering it later in GenerateQRCodeBitmap — the
// same "Build fails fast on content it can't lay out" precedent
// imageDimensions already sets for undecodable Image data, even though
// unlike an image header read this has no cheaper partial form: the
// encode is redone in GenerateQRCodeBitmap when the actual bitmap is
// needed, mirroring how DecodeImageBitmap independently redecodes Data
// after imageDimensions already read its header once — each of the two
// call sites (Build, Paint) encodes at most once, never twice within the
// same call.
func qrCodeDimensions(q receipt.QRCode, maxWidth int) (width, height int, err error) {
	if _, err := qr.Encode(q.Content, q.RecoveryLevel(), qr.Auto); err != nil {
		return 0, 0, err
	}
	size := qrCodeSize(q, maxWidth)
	return size, size, nil
}

// GenerateQRCodeBitmap generates q's QR code as a GlyphBitmap — the same
// 1bpp pixel representation DecodeImageBitmap already produces for
// receipt.Image, so render/canvas.Paint paints a QRCode Block with the
// exact same paintGlyph primitive it already paints Image and glyph
// Blocks with (docs/ARCHITECTURE.md §4: exactly one bitmap-painting
// path). Generation happens here, at layout time, mirroring how an
// Image's pixels are resolved by DecodeImageBitmap rather than by
// render/canvas — canvas never needs to know a QRCode Block's bitmap
// came from an encoder rather than a decoder.
//
// qr.Encode returns the QR code at its native resolution — one pixel per
// module, e.g. 21x21 for a small payload — with no scaling of its own;
// rasterizeImage (the exact same helper image.go uses to turn any
// image.Image into a GlyphBitmap) both up- and down-samples that native
// image to qrCodeSize's target dimensions via nearest-neighbour, so
// there is no QR-specific pixel-plotting or scaling logic anywhere in
// this package.
func GenerateQRCodeBitmap(q receipt.QRCode, maxWidth int) (GlyphBitmap, error) {
	code, err := qr.Encode(q.Content, q.RecoveryLevel(), qr.Auto)
	if err != nil {
		return GlyphBitmap{}, err
	}
	size := qrCodeSize(q, maxWidth)
	return rasterizeImage(code, size, size), nil
}
