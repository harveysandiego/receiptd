package canvas

import "github.com/harveysandiego/receiptd/internal/render/layout"

// styleGlyph applies style to bmp, the base bitmap layout.Font.Glyph
// returned, per docs/ARCHITECTURE.md §3 "Text styling"'s fixed pipeline:
// scale first, then bold. Font itself never sees style — it remains the
// sole source of a glyph's unscaled, unstyled pixels; everything style
// changes about those pixels happens here, in the one place Blocks are
// turned into painted pixels. Future styles (underline, strikethrough,
// italic) are additional steps appended to this same pipeline when
// implemented, not a reason to redesign it.
func styleGlyph(bmp layout.GlyphBitmap, style layout.Style) layout.GlyphBitmap {
	bmp = scaleGlyph(bmp, style.Size)
	if style.Bold {
		bmp = boldGlyph(bmp)
	}
	return bmp
}

// scaleGlyph returns bmp scaled by an exact integer factor: each source
// pixel becomes a factor x factor block of identical pixels (nearest
// neighbour), so edges stay sharp with no interpolation or
// anti-aliasing. factor <= 1 returns bmp unchanged — by the time Paint
// sees a Style, render/layout.Build has already resolved its Size to
// >= 1 (docs/ARCHITECTURE.md §3), so this is the ordinary "no scaling"
// case, not a defensive check against invalid input.
func scaleGlyph(bmp layout.GlyphBitmap, factor int) layout.GlyphBitmap {
	if factor <= 1 {
		return bmp
	}

	width, height := bmp.Width*factor, bmp.Height*factor
	srcRowBytes := (bmp.Width + 7) / 8
	dstRowBytes := (width + 7) / 8
	out := layout.GlyphBitmap{Width: width, Height: height, Bits: make([]byte, dstRowBytes*height)}

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

// boldGlyph returns bmp with a deterministic bitmap-bold effect: every set
// pixel is ORed with its immediate right-hand neighbour, a one-pixel
// overdraw that thickens strokes without anti-aliasing. bmp's dimensions
// are unchanged — bold never affects a glyph's measured width or advance,
// only which pixels within it are set.
func boldGlyph(bmp layout.GlyphBitmap) layout.GlyphBitmap {
	rowBytes := (bmp.Width + 7) / 8
	out := layout.GlyphBitmap{Width: bmp.Width, Height: bmp.Height, Bits: make([]byte, len(bmp.Bits))}
	copy(out.Bits, bmp.Bits)

	for y := 0; y < bmp.Height; y++ {
		for x := 0; x < bmp.Width; x++ {
			if bmp.Bits[y*rowBytes+x/8]&(0x80>>uint(x%8)) == 0 {
				continue
			}
			if x+1 < bmp.Width {
				out.Bits[y*rowBytes+(x+1)/8] |= 0x80 >> uint((x+1)%8)
			}
		}
	}
	return out
}
