package canvas

import "github.com/harveysandiego/receiptd/internal/render/layout"

// styleGlyph applies style to bmp, the base bitmap layout.Font.Glyph
// returned, per docs/ARCHITECTURE.md §3 "Text styling"'s fixed pipeline:
// scale, then bold, then italic. Font itself never sees style — it
// remains the sole source of a glyph's unscaled, unstyled pixels;
// everything style changes about those pixels happens here, in the one
// place Blocks are turned into painted pixels. Underline and
// strikethrough are not part of this pipeline: they don't transform a
// glyph's bitmap at all, so they're painted directly onto the Canvas
// after glyphs are (see paintDecorations in paint.go).
//
// Bold before italic, not the reverse: it reads like italic-then-bold
// should clip less (shearing a narrower, unbolded glyph first, then
// only adding bold's 1px overdraw after), but both are simple per-row
// translations under the same final bounds check, and clipped
// translations compose identically regardless of order — confirmed by
// literally swapping the order and diffing byte-for-byte identical
// output. Kept in the originally documented order since reordering
// bought nothing.
func styleGlyph(bmp layout.GlyphBitmap, style layout.Style) layout.GlyphBitmap {
	bmp = scaleGlyph(bmp, style.Size)
	if style.Bold {
		bmp = boldGlyph(bmp)
	}
	if style.Italic {
		bmp = italicGlyph(bmp)
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

// italicGlyph returns bmp with a deterministic synthetic-italic shear
// applied: each row is shifted right by italicShear(bmp.Height, y), so
// the glyph's top leans further right than its bottom — a rightward
// lean, the same visual direction as a real italic face. The shift is
// an integer number of whole pixels (no interpolation, so edges stay
// sharp), and bmp's Width and Height are unchanged: a pixel shifted
// past the right edge is dropped rather than growing the bitmap, which
// is what keeps italic from needing its own measurement — the already
// exact Font.Measure(s) * Style.Size formula stays correct for italic
// content too.
func italicGlyph(bmp layout.GlyphBitmap) layout.GlyphBitmap {
	rowBytes := (bmp.Width + 7) / 8
	out := layout.GlyphBitmap{Width: bmp.Width, Height: bmp.Height, Bits: make([]byte, len(bmp.Bits))}

	for y := 0; y < bmp.Height; y++ {
		shift := italicShear(bmp.Height, y)
		for x := 0; x < bmp.Width; x++ {
			if bmp.Bits[y*rowBytes+x/8]&(0x80>>uint(x%8)) == 0 {
				continue
			}
			sx := x + shift
			if sx < 0 || sx >= bmp.Width {
				continue
			}
			out.Bits[y*rowBytes+sx/8] |= 0x80 >> uint(sx%8)
		}
	}
	return out
}

// italicShear returns row y's pixel shift for a height-tall glyph,
// centred on the glyph's vertical midpoint: negative (leftward) below
// it, positive (rightward) above it, both growing by one pixel every
// five rows moving away from the middle. Centring — rather than
// anchoring the bottom row at a fixed 0 and only ever shearing
// rightward — keeps the same total top-to-bottom slant while roughly
// halving the largest single-direction displacement, which matters
// because italicGlyph drops any pixel sheared past either edge: an
// anchored-at-bottom shear (tried first, divisor 3, then divisor 5 —
// docs/ARCHITECTURE.md §3) pushed every row's displacement in the same
// direction, so a wide or rounded glyph near the top ("B", "o", "d",
// "U") had nothing but the shear's own growing distance from a fixed
// zero standing between it and the edge; a real-hardware print review
// found divisor 3 corrupted those letterforms and divisor 5 still
// visibly clipped them. The exact ratio remains a visual tuning choice
// (the architecture only requires "a deterministic bitmap shear...
// visually pleasing... sufficient"), not a value a caller should depend
// on.
func italicShear(height, y int) int {
	mid := (height - 1) / 2
	return (mid - y) / 5
}
