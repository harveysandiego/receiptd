package canvas_test

import (
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// pixelSet reports whether c's pixel at (x, y) is painted.
func pixelSet(c *canvas.Canvas, x, y int) bool {
	rowBytes := (c.Width + 7) / 8
	return c.Bits[y*rowBytes+x/8]&(0x80>>uint(x%8)) != 0
}

// glyphPixelSet reports whether bmp's pixel at (x, y) is set.
func glyphPixelSet(bmp layout.GlyphBitmap, x, y int) bool {
	rowBytes := (bmp.Width + 7) / 8
	return bmp.Bits[y*rowBytes+x/8]&(0x80>>uint(x%8)) != 0
}

// assertGlyphPainted fails t unless every pixel of bmp appears in c,
// offset down by originY (all test glyphs in this file start at x=0).
func assertGlyphPainted(t *testing.T, c *canvas.Canvas, originY int, bmp layout.GlyphBitmap) {
	t.Helper()
	for y := 0; y < bmp.Height; y++ {
		for x := 0; x < bmp.Width; x++ {
			want := glyphPixelSet(bmp, x, y)
			got := pixelSet(c, x, originY+y)
			if got != want {
				t.Errorf("pixel(%d,%d) = %v, want %v (glyph pixel %d,%d)", x, originY+y, got, want, x, y)
			}
		}
	}
}

func TestPaint_EmptyDocument(t *testing.T) {
	c, err := canvas.Paint(layout.Document{Font: layout.EmbeddedFont{}})
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Width != 0 || c.Height != 0 {
		t.Errorf("Canvas = %dx%d, want 0x0", c.Width, c.Height)
	}
	if len(c.Bits) != 0 {
		t.Errorf("len(c.Bits) = %d, want 0", len(c.Bits))
	}
}

func TestPaint_OneTextBlock_MatchesFontGlyph(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc := layout.Document{
		Font: f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Text{Content: "A"}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if want := f.Measure("A"); c.Width != want {
		t.Errorf("c.Width = %d, want %d", c.Width, want)
	}
	if want := f.LineHeight(); c.Height != want {
		t.Errorf("c.Height = %d, want %d", c.Height, want)
	}
	bmp, _ := f.Glyph('A')
	assertGlyphPainted(t, c, 0, bmp)
}

func TestPaint_GlyphPlacementUsesBlockY(t *testing.T) {
	f := layout.EmbeddedFont{}
	const y = 2 * 13 // an arbitrary non-zero line offset
	doc := layout.Document{
		Font: f,
		Blocks: []layout.Block{
			{Y: y, Element: receipt.Text{Content: "A"}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	bmp, _ := f.Glyph('A')
	assertGlyphPainted(t, c, y, bmp)
}

func TestPaint_PreservesBlockOrder(t *testing.T) {
	f := layout.EmbeddedFont{}
	lh := f.LineHeight()
	doc := layout.Document{
		Font: f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Text{Content: "A"}},
			{Y: lh, Element: receipt.Text{Content: "B"}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	bmpA, _ := f.Glyph('A')
	bmpB, _ := f.Glyph('B')
	assertGlyphPainted(t, c, 0, bmpA)
	assertGlyphPainted(t, c, lh, bmpB)
}

func TestPaint_Deterministic(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc := layout.Document{
		Font: f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Text{Content: "Milk"}},
			{Y: f.LineHeight(), Element: receipt.Text{Content: "Eggs"}},
		},
	}

	first, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	second, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}

	if first.Width != second.Width || first.Height != second.Height {
		t.Fatalf("dimensions = %dx%d, then %dx%d, want equal", first.Width, first.Height, second.Width, second.Height)
	}
	if string(first.Bits) != string(second.Bits) {
		t.Errorf("Bits differ between calls, want identical")
	}
}

func TestPaint_UnsupportedElementReturnsPermanentError(t *testing.T) {
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Divider{Style: "solid"}},
		},
	}
	_, err := canvas.Paint(doc)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Paint() error = %v, want apperr.KindPermanent", err)
	}
}

func TestPaint_OneHeadingBlock_MatchesFontGlyph(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc := layout.Document{
		Font: f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Heading{Content: "A"}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if want := f.Measure("A"); c.Width != want {
		t.Errorf("c.Width = %d, want %d", c.Width, want)
	}
	if want := f.LineHeight(); c.Height != want {
		t.Errorf("c.Height = %d, want %d", c.Height, want)
	}
	bmp, _ := f.Glyph('A')
	assertGlyphPainted(t, c, 0, bmp)
}

func TestPaint_HeadingAndTextBlocks_PreservesOrder(t *testing.T) {
	f := layout.EmbeddedFont{}
	lh := f.LineHeight()
	doc := layout.Document{
		Font: f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Heading{Content: "A"}},
			{Y: lh, Element: receipt.Text{Content: "B"}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	bmpA, _ := f.Glyph('A')
	bmpB, _ := f.Glyph('B')
	assertGlyphPainted(t, c, 0, bmpA)
	assertGlyphPainted(t, c, lh, bmpB)
}

func TestPaint_UnsupportedElementAmongSupportedOnes(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc := layout.Document{
		Font: f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Text{Content: "Milk"}},
			{Y: f.LineHeight(), Element: receipt.Divider{Style: "solid"}},
		},
	}
	_, err := canvas.Paint(doc)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Paint() error = %v, want apperr.KindPermanent", err)
	}
}
