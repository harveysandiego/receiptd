package layout_test

import (
	"testing"

	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// asFont exercises EmbeddedFont only through the layout.Font interface,
// so these tests catch a signature that quietly stops satisfying it.
func asFont(f layout.Font) layout.Font { return f }

func TestEmbeddedFont_Measure_Empty(t *testing.T) {
	f := asFont(layout.EmbeddedFont{})
	if got := f.Measure(""); got != 0 {
		t.Errorf("Measure(\"\") = %d, want 0", got)
	}
}

func TestEmbeddedFont_Measure_ScalesWithLength(t *testing.T) {
	f := asFont(layout.EmbeddedFont{})
	one := f.Measure("A")
	if one <= 0 {
		t.Fatalf("Measure(\"A\") = %d, want > 0", one)
	}
	if got, want := f.Measure("AB"), one*2; got != want {
		t.Errorf("Measure(\"AB\") = %d, want %d (2x Measure(\"A\"))", got, want)
	}
	if got, want := f.Measure("ABCDE"), one*5; got != want {
		t.Errorf("Measure(\"ABCDE\") = %d, want %d (5x Measure(\"A\"))", got, want)
	}
}

func TestEmbeddedFont_Measure_SameWidthRegardlessOfGlyph(t *testing.T) {
	// EmbeddedFont is fixed-width: every rune, including ones outside
	// its embedded range, advances by the same amount.
	f := asFont(layout.EmbeddedFont{})
	want := f.Measure("A")
	for _, s := range []string{"a", "0", " ", "~", "€", "字"} {
		if got := f.Measure(s); got != want {
			t.Errorf("Measure(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestEmbeddedFont_Measure_Deterministic(t *testing.T) {
	f := asFont(layout.EmbeddedFont{})
	const s = "Receipt #42"
	first := f.Measure(s)
	for i := 0; i < 5; i++ {
		if got := f.Measure(s); got != first {
			t.Errorf("Measure(%q) call %d = %d, want %d", s, i, got, first)
		}
	}
}

func TestEmbeddedFont_LineHeight_Positive(t *testing.T) {
	f := asFont(layout.EmbeddedFont{})
	if got := f.LineHeight(); got <= 0 {
		t.Errorf("LineHeight() = %d, want > 0", got)
	}
}

func TestEmbeddedFont_LineHeight_Constant(t *testing.T) {
	f := asFont(layout.EmbeddedFont{})
	want := f.LineHeight()
	for i := 0; i < 5; i++ {
		if got := f.LineHeight(); got != want {
			t.Errorf("LineHeight() call %d = %d, want %d", i, got, want)
		}
	}
}

func TestEmbeddedFont_Glyph_DimensionsMatchBits(t *testing.T) {
	f := asFont(layout.EmbeddedFont{})
	for _, r := range []rune{'A', 'a', '0', ' ', '!', '~'} {
		bmp, advance := f.Glyph(r)
		if bmp.Width <= 0 || bmp.Height <= 0 {
			t.Fatalf("Glyph(%q) bitmap = %dx%d, want positive dimensions", r, bmp.Width, bmp.Height)
		}
		if advance <= 0 {
			t.Errorf("Glyph(%q) advance = %d, want > 0", r, advance)
		}
		rowBytes := (bmp.Width + 7) / 8
		wantLen := rowBytes * bmp.Height
		if len(bmp.Bits) != wantLen {
			t.Errorf("Glyph(%q) len(Bits) = %d, want %d ((Width+7)/8 * Height)", r, len(bmp.Bits), wantLen)
		}
	}
}

func TestEmbeddedFont_Glyph_UnsupportedRuneReturnsFallback(t *testing.T) {
	// A rune outside the embedded range (e.g. CJK) has no glyph of its
	// own; EmbeddedFont must still return a usable, correctly sized
	// bitmap rather than a zero value.
	f := asFont(layout.EmbeddedFont{})
	bmp, advance := f.Glyph('字')
	if bmp.Width <= 0 || bmp.Height <= 0 {
		t.Fatalf("Glyph('字') bitmap = %dx%d, want positive dimensions", bmp.Width, bmp.Height)
	}
	if advance <= 0 {
		t.Errorf("Glyph('字') advance = %d, want > 0", advance)
	}
}

func TestEmbeddedFont_Glyph_Deterministic(t *testing.T) {
	f := asFont(layout.EmbeddedFont{})
	bmp1, adv1 := f.Glyph('R')
	bmp2, adv2 := f.Glyph('R')
	if adv1 != adv2 {
		t.Errorf("Glyph('R') advance = %d then %d, want equal", adv1, adv2)
	}
	if bmp1.Width != bmp2.Width || bmp1.Height != bmp2.Height {
		t.Errorf("Glyph('R') dimensions = %dx%d then %dx%d, want equal", bmp1.Width, bmp1.Height, bmp2.Width, bmp2.Height)
	}
	if string(bmp1.Bits) != string(bmp2.Bits) {
		t.Errorf("Glyph('R') Bits differ between calls, want identical")
	}
}

func TestEmbeddedFont_Measure_DoublesTheUnderlyingFaceAdvance(t *testing.T) {
	// EmbeddedFont bakes a fixed 2x upscale of basicfont.Face7x13 into
	// its own native resolution (docs/adr/0008-embedded-font-legibility.md):
	// 7x13 dots measured too small to read reliably on real 203 DPI
	// thermal hardware. Style.Size's own meaning is untouched by this —
	// this test pins EmbeddedFont's own baseline, not Style.Size.
	f := asFont(layout.EmbeddedFont{})
	rawAdvance, _ := basicfont.Face7x13.GlyphAdvance('A')
	if want := rawAdvance.Round() * 2; f.Measure("A") != want {
		t.Errorf("Measure(\"A\") = %d, want %d (2x basicfont.Face7x13's own advance)", f.Measure("A"), want)
	}
}

func TestEmbeddedFont_LineHeight_DoublesTheUnderlyingFaceHeight(t *testing.T) {
	f := asFont(layout.EmbeddedFont{})
	if want := basicfont.Face7x13.Metrics().Height.Round() * 2; f.LineHeight() != want {
		t.Errorf("LineHeight() = %d, want %d (2x basicfont.Face7x13's own line height)", f.LineHeight(), want)
	}
}

func TestEmbeddedFont_Glyph_DoublesTheUnderlyingFaceDimensionsAndAdvance(t *testing.T) {
	f := asFont(layout.EmbeddedFont{})
	bmp, advance := f.Glyph('A')

	dr, _, _, rawAdvance, _ := basicfont.Face7x13.Glyph(fixed.Point26_6{}, 'A')
	if wantWidth, wantHeight := dr.Dx()*2, dr.Dy()*2; bmp.Width != wantWidth || bmp.Height != wantHeight {
		t.Errorf("Glyph('A') = %dx%d, want %dx%d (2x the embedded basicfont.Face7x13 glyph)", bmp.Width, bmp.Height, wantWidth, wantHeight)
	}
	if want := rawAdvance.Round() * 2; advance != want {
		t.Errorf("Glyph('A') advance = %d, want %d (2x)", advance, want)
	}
}

func TestEmbeddedFont_Glyph_PreservesShapeAtDoubleResolution(t *testing.T) {
	// The upscale must be exact nearest-neighbour replication (each
	// source pixel becomes a 2x2 block), not a distortion of the
	// original glyph shape — the same contract render/canvas's own
	// Style.Size scaling makes for scaleGlyph.
	f := asFont(layout.EmbeddedFont{})
	bmp, _ := f.Glyph('A')

	dr, mask, maskp, _, _ := basicfont.Face7x13.Glyph(fixed.Point26_6{}, 'A')
	rawSet := func(x, y int) bool {
		_, _, _, a := mask.At(maskp.X+x, maskp.Y+y).RGBA()
		return a >= 1<<15
	}
	bmpSet := func(x, y int) bool {
		rowBytes := (bmp.Width + 7) / 8
		return bmp.Bits[y*rowBytes+x/8]&(0x80>>uint(x%8)) != 0
	}

	for y := 0; y < dr.Dy(); y++ {
		for x := 0; x < dr.Dx(); x++ {
			want := rawSet(x, y)
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					if got := bmpSet(x*2+dx, y*2+dy); got != want {
						t.Errorf("pixel(%d,%d) = %v, want %v (source glyph pixel %d,%d scaled x2)", x*2+dx, y*2+dy, got, want, x, y)
					}
				}
			}
		}
	}
}

func TestEmbeddedFont_Glyph_DifferentGlyphsDifferentBits(t *testing.T) {
	// A weak sanity check that Glyph isn't returning the same bitmap for
	// every rune (e.g. always blank) - not a claim about visual
	// correctness, which is out of scope for this slice.
	f := asFont(layout.EmbeddedFont{})
	space, _ := f.Glyph(' ')
	letter, _ := f.Glyph('W')
	if string(space.Bits) == string(letter.Bits) {
		t.Errorf("Glyph(' ') and Glyph('W') returned identical Bits, want different")
	}
}
