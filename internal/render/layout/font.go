package layout

// Font answers the two questions render/layout.Build needs to lay out
// text and render/canvas.Paint needs to draw it: how wide is this string,
// and what does this glyph look like. See docs/ARCHITECTURE.md §2.
type Font interface {
	// Measure returns the width of s, in dots, if painted with this Font.
	Measure(s string) int

	// LineHeight returns this Font's line height, in dots.
	LineHeight() int

	// Glyph returns the bitmap for r and the horizontal distance, in
	// dots, to advance before painting the next glyph. Fonts that lack a
	// glyph for r return a fallback bitmap rather than a zero value, so
	// callers never need to special-case missing glyphs.
	Glyph(r rune) (bitmap GlyphBitmap, advance int)
}

// GlyphBitmap is a single glyph's pixels: a Width x Height grid, one bit
// per pixel, set bits painted. Each row is packed MSB-first into whole
// bytes, padded with unset bits — row length is
// (Width+7)/8 bytes, and len(Bits) is Height times that.
type GlyphBitmap struct {
	Width, Height int
	Bits          []byte
}
