package layout

import (
	"github.com/boombuler/barcode/qr"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

// qrCodeSize returns the width and height, in dots, q renders at (always
// square) — q.Size or receipt.DefaultQRCodeSize, scaled to fit maxWidth per
// scaledImageSize's shrink-to-fit rule. Pure arithmetic: unlike an image, a
// QR code's size depends only on Size and maxWidth, not Content.
func qrCodeSize(q receipt.QRCode, maxWidth int) int {
	size := q.Size
	if size <= 0 {
		size = receipt.DefaultQRCodeSize
	}
	size, _ = scaledImageSize(size, size, maxWidth)
	return size
}

// qrCodeDimensions returns q's rendered width and height (see qrCodeSize),
// also running qr.Encode purely to surface a capacity error (content too
// large for q's error-correction level) at Build time rather than later in
// GenerateQRCodeBitmap — the fail-fast precedent imageHeight sets. Unlike an
// image-header read this has no cheaper partial form, so the encode is
// redone in GenerateQRCodeBitmap; each call site encodes at most once.
func qrCodeDimensions(q receipt.QRCode, maxWidth int) (width, height int, err error) {
	if _, err := qr.Encode(q.Content, q.RecoveryLevel(), qr.Auto); err != nil {
		return 0, 0, err
	}
	size := qrCodeSize(q, maxWidth)
	return size, size, nil
}

// GenerateQRCodeBitmap generates q's QR code as a GlyphBitmap — the same
// 1bpp representation DecodeImageBitmap produces, so render/canvas.Paint
// paints a QRCode Block with the one paintGlyph primitive
// (docs/ARCHITECTURE.md §4). Generation happens here at layout time, so
// canvas never needs to know the bitmap came from an encoder.
//
// qr.Encode returns the code at native resolution (one pixel per module,
// e.g. 21x21). The shared rasterizeImage helper up-/down-samples it to
// qrCodeSize via nearest-neighbour — no QR-specific scaling logic.
func GenerateQRCodeBitmap(q receipt.QRCode, maxWidth int) (GlyphBitmap, error) {
	code, err := qr.Encode(q.Content, q.RecoveryLevel(), qr.Auto)
	if err != nil {
		return GlyphBitmap{}, err
	}
	size := qrCodeSize(q, maxWidth)
	return rasterizeImage(code, size, size), nil
}
