package layout_test

import (
	"testing"

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
