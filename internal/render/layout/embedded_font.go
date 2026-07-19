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
// EmbeddedFont's own native resolution is basicfont.Face7x13's 7x13
// glyphs upscaled by the fixed nativeScale below, baked in here rather
// than left at basicfont's raw size: real 203 DPI thermal hardware
// testing found 7x13 dots too small to read reliably
// (docs/adr/0008-embedded-font-legibility.md). This is purely an
// internal resolution change — Style.Size (docs/ARCHITECTURE.md §3)
// keeps its exact documented meaning, an integer multiple of
// EmbeddedFont's own native glyph, with 1 (or omitted) still
// "unscaled"; it's just that "unscaled" now means 14x26, not 7x13.
//
// The zero value is ready to use.
type EmbeddedFont struct{}

// nativeScale is the fixed nearest-neighbour upscale baked into every
// glyph, advance, and line height EmbeddedFont reports, applied before
// Style.Size scaling ever sees them — see EmbeddedFont's own doc
// comment and docs/adr/0008-embedded-font-legibility.md.
const nativeScale = 2

// Measure returns the width of s, in dots, as the sum of each rune's
// advance.
func (EmbeddedFont) Measure(s string) int {
	width := 0
	for _, r := range s {
		adv, _ := basicfont.Face7x13.GlyphAdvance(r)
		width += adv.Round() * nativeScale
	}
	return width
}

// LineHeight returns the embedded face's line height, in dots.
func (EmbeddedFont) LineHeight() int {
	return basicfont.Face7x13.Metrics().Height.Round() * nativeScale
}

// Glyph returns r's bitmap and advance. Runes outside the embedded
// face's range fall back to its replacement-character glyph (see
// basicfont.Face.Glyph) rather than an empty bitmap.
func (EmbeddedFont) Glyph(r rune) (GlyphBitmap, int) {
	dr, mask, maskp, adv, _ := basicfont.Face7x13.Glyph(fixed.Point26_6{}, r)
	raw := GlyphBitmap{
		Width:  dr.Dx(),
		Height: dr.Dy(),
		Bits:   packMask(mask, maskp, dr.Dx(), dr.Dy()),
	}
	return upscale(raw, nativeScale), adv.Round() * nativeScale
}

// upscale returns bmp scaled by an exact integer factor via
// nearest-neighbour replication (each source pixel becomes a
// factor x factor block) — the same scaling contract
// render/canvas.scaleGlyph applies for Style.Size, duplicated here in
// miniature rather than imported: render/layout sits below
// render/canvas in the package dependency order (docs/ARCHITECTURE.md
// §11) and must not import it. This bakes EmbeddedFont's own native
// resolution (nativeScale); it has nothing to do with Style.Size, which
// render/canvas applies separately, afterwards.
func upscale(bmp GlyphBitmap, factor int) GlyphBitmap {
	if factor <= 1 {
		return bmp
	}

	width, height := bmp.Width*factor, bmp.Height*factor
	srcRowBytes := (bmp.Width + 7) / 8
	dstRowBytes := (width + 7) / 8
	out := GlyphBitmap{Width: width, Height: height, Bits: make([]byte, dstRowBytes*height)}

	for y := 0; y < bmp.Height; y++ {
		for x := 0; x < bmp.Width; x++ {
			if bmp.Bits[y*srcRowBytes+x/8]&(0x80>>uint(x%8)) == 0 {
				continue
			}
			for dy := 0; dy < factor; dy++ {
				for dx := 0; dx < factor; dx++ {
					px, py := x*factor+dx, y*factor+dy
					out.Bits[py*dstRowBytes+px/8] |= 0x80 >> uint(px%8)
				}
			}
		}
	}
	return out
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
