package canvas

import "github.com/harveysandiego/receiptd/internal/render/layout"

// styleGlyph applies style to bmp (the base bitmap from layout.Font.Glyph)
// per docs/ARCHITECTURE.md §3's fixed pipeline: scale, then bold, then
// italic. Font never sees style; everything style changes happens here.
// Underline and strikethrough aren't part of this pipeline — they don't
// transform a glyph's bitmap, so they're painted directly onto the Canvas
// afterwards (see paintDecorations in paint.go).
//
// Bold-before-italic is not load-bearing: both are per-row translations
// under the same final bounds check and compose identically either way
// (verified by swapping the order — byte-for-byte identical output). Kept
// in the originally documented order since reordering bought nothing.
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
// pixel becomes a factor x factor block (nearest neighbour), so edges stay
// sharp. factor <= 1 returns bmp unchanged — Build always resolves Size to
// >= 1, so that is the ordinary "no scaling" case, not a defensive check.
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

// italicGlyph returns bmp with a synthetic-italic shear: each row is
// shifted by italicShear(bmp.Height, y) whole pixels, so the top leans
// further right than the bottom. Width and Height are unchanged — a pixel
// sheared past an edge is dropped rather than growing the bitmap, which is
// what keeps the Font.Measure(s)*Style.Size formula correct for italic too.
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

// italicShear returns row y's pixel shift for a height-tall glyph, centred
// on the vertical midpoint (leftward below, rightward above), growing one
// pixel every five rows. Centring roughly halves the largest
// single-direction displacement versus anchoring the bottom at 0, which
// matters because italicGlyph drops any pixel sheared past an edge: a
// real-hardware review found the earlier anchored shears (divisor 3, then
// 5) clipped wide/rounded glyphs near the top. The exact ratio is a visual
// tuning choice (docs/ARCHITECTURE.md §3), not something a caller depends on.
func italicShear(height, y int) int {
	mid := (height - 1) / 2
	return (mid - y) / 5
}
