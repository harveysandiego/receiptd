package canvas

// Canvas is a 1-bit monochrome bitmap: Width x Height dots, each row
// packed MSB-first into whole bytes, padded with unset bits — the same
// layout as layout.GlyphBitmap, so painting a glyph is a direct bit
// copy. Row length is (Width+7)/8 bytes, and len(Bits) is Height times
// that.
type Canvas struct {
	Width, Height int
	Bits          []byte
}
