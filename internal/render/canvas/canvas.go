package canvas

import "github.com/harveysandiego/receiptd/internal/receipt"

// Canvas is a 1-bit monochrome bitmap: Width x Height dots, each row
// packed MSB-first into whole bytes, padded with unset bits — the same
// layout as layout.GlyphBitmap, so painting a glyph is a direct bit
// copy. Row length is (Width+7)/8 bytes, and len(Bits) is Height times
// that.
//
// Controls carries the printer-control elements (receipt.Feed,
// receipt.Cut) Paint found while painting, in Document order — see
// Control. It is unrelated to Bits: a Control has no pixels of its own
// and never affects Width, Height, or what's painted. render/escpos.Encode
// is Controls' only consumer; EncodePNG ignores it entirely, since a
// printer-control command has no visual representation to preview.
type Canvas struct {
	Width, Height int
	Bits          []byte
	Controls      []Control
}

// Control is a printer-control instruction — an explicit receipt.Feed or
// receipt.Cut element — positioned at Y, the dot row (in Canvas's own
// coordinate space) everything painted above it must be sent to the
// printer before Control's own command bytes. Element is always a
// receipt.Feed or receipt.Cut; render/escpos.Encode is what turns it into
// actual ESC/POS bytes, per docs/adr/0002-raster-rendering.md's "the only
// genuine ESC/POS commands used are initialization, feed, and cut."
//
// Terminal is true exactly when this Control's Block was the very last
// Block in the source Document — the fact render/escpos.Encode needs to
// decide whether an explicit trailing receipt.Cut should suppress the
// automatic end-of-receipt cut (docs/ARCHITECTURE.md §4 step 8d), without
// needing the whole Document itself. A receipt.Feed positioned last is
// still marked Terminal, but Encode only acts on it for a receipt.Cut.
type Control struct {
	Y        int
	Element  receipt.Element
	Terminal bool
}
