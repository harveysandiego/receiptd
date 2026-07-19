package canvas

import "github.com/harveysandiego/receiptd/internal/receipt"

// Canvas is a 1-bit monochrome bitmap: Width x Height dots, each row
// packed MSB-first into whole bytes, padded with unset bits — the same
// layout as layout.GlyphBitmap, so painting a glyph is a direct bit
// copy. Row length is (Width+7)/8 bytes, and len(Bits) is Height times
// that.
//
// Controls carries any printer-control elements (receipt.Feed,
// receipt.Cut) Paint found, in Document order — see Control and
// docs/adr/0010-printer-control-elements-via-canvas-controls.md. It never
// affects Width, Height, or Bits; render/escpos.Encode is its only
// consumer, EncodePNG ignores it.
type Canvas struct {
	Width, Height int
	Bits          []byte
	Controls      []Control
}

// Control is a positioned receipt.Feed or receipt.Cut: Y is the dot row
// (Canvas's own coordinate space) everything above it must be sent to the
// printer before its command bytes. Terminal is true exactly when this
// was the last Block in the source Document — see
// docs/adr/0010-printer-control-elements-via-canvas-controls.md for why.
type Control struct {
	Y        int
	Element  receipt.Element
	Terminal bool
}
