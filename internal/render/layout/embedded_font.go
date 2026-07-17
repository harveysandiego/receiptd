package layout

import (
	"image"

	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// EmbeddedFont is the built-in Font: a fixed-width bitmap face compiled
// into the binary via golang.org/x/image/font/basicfont, so no font file
// is read at runtime. Consistent with docs/adr/0002-raster-rendering.md,
// its glyphs are already pixels, not an outline rasterized on demand.
//
// The zero value is ready to use.
type EmbeddedFont struct{}

// Measure returns the width of s, in dots, as the sum of each rune's
// advance.
func (EmbeddedFont) Measure(s string) int {
	width := 0
	for _, r := range s {
		adv, _ := basicfont.Face7x13.GlyphAdvance(r)
		width += adv.Round()
	}
	return width
}

// LineHeight returns the embedded face's line height, in dots.
func (EmbeddedFont) LineHeight() int {
	return basicfont.Face7x13.Metrics().Height.Round()
}

// Glyph returns r's bitmap and advance. Runes outside the embedded
// face's range fall back to its replacement-character glyph (see
// basicfont.Face.Glyph) rather than an empty bitmap.
func (EmbeddedFont) Glyph(r rune) (GlyphBitmap, int) {
	dr, mask, maskp, adv, _ := basicfont.Face7x13.Glyph(fixed.Point26_6{}, r)
	return GlyphBitmap{
		Width:  dr.Dx(),
		Height: dr.Dy(),
		Bits:   packMask(mask, maskp, dr.Dx(), dr.Dy()),
	}, adv.Round()
}

// packMask reads a Width x Height region of mask starting at origin and
// packs it into GlyphBitmap's 1bpp row format: MSB-first, each row padded
// to a whole byte. A mask pixel is a set bit when its alpha is at least
// half of font.Face's Glyph fully-covered value (0xffff).
func packMask(mask image.Image, origin image.Point, width, height int) []byte {
	rowBytes := (width + 7) / 8
	bits := make([]byte, rowBytes*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			_, _, _, a := mask.At(origin.X+x, origin.Y+y).RGBA()
			if a >= 1<<15 {
				bits[y*rowBytes+x/8] |= 0x80 >> uint(x%8)
			}
		}
	}
	return bits
}
