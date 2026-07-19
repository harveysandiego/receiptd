package canvas_test

import (
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
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

// assertScaledGlyphPainted fails t unless c contains bmp scaled by an
// exact factorxfactor nearest-neighbour block per source pixel, offset
// right by originX (all test glyphs in this file that need this
// assertion start at y=0) — the behavioural contract
// docs/ARCHITECTURE.md §3 "Text styling" describes for Style.Size,
// verified without reaching into any unexported scaling helper.
func assertScaledGlyphPainted(t *testing.T, c *canvas.Canvas, originX int, bmp layout.GlyphBitmap, factor int) {
	t.Helper()
	for y := 0; y < bmp.Height; y++ {
		for x := 0; x < bmp.Width; x++ {
			want := glyphPixelSet(bmp, x, y)
			for dy := 0; dy < factor; dy++ {
				for dx := 0; dx < factor; dx++ {
					px, py := originX+x*factor+dx, y*factor+dy
					if got := pixelSet(c, px, py); got != want {
						t.Errorf("pixel(%d,%d) = %v, want %v (source glyph pixel %d,%d scaled x%d)", px, py, got, want, x, y, factor)
					}
				}
			}
		}
	}
}

// countSetPixels returns how many of c's pixels are painted.
func countSetPixels(c *canvas.Canvas) int {
	count := 0
	for y := 0; y < c.Height; y++ {
		for x := 0; x < c.Width; x++ {
			if pixelSet(c, x, y) {
				count++
			}
		}
	}
	return count
}

// assertGlyphPaintedExceptRows behaves like assertGlyphPainted but skips
// bmp rows in [excludeFrom, excludeTo) — the rows a decoration (e.g. an
// underline sharing bmp's own bottom row) legitimately painted over,
// which assertGlyphPainted's exact-match would otherwise flag as the
// glyph bitmap having been modified when it wasn't.
func assertGlyphPaintedExceptRows(t *testing.T, c *canvas.Canvas, originY int, bmp layout.GlyphBitmap, excludeFrom, excludeTo int) {
	t.Helper()
	for y := 0; y < bmp.Height; y++ {
		if y >= excludeFrom && y < excludeTo {
			continue
		}
		for x := 0; x < bmp.Width; x++ {
			want := glyphPixelSet(bmp, x, y)
			got := pixelSet(c, x, originY+y)
			if got != want {
				t.Errorf("pixel(%d,%d) = %v, want %v (glyph pixel %d,%d)", x, originY+y, got, want, x, y)
			}
		}
	}
}

// assertHLineSet fails t unless every pixel in the horizontal band
// [0, width) x [y0, y0+thickness) is painted in c — the shape a
// rendered underline or strikethrough decoration must have
// (docs/ARCHITECTURE.md §3 "Text styling"; all decorated content in
// this file starts at x=0, the same assumption assertGlyphPainted
// already makes).
func assertHLineSet(t *testing.T, c *canvas.Canvas, width, y0, thickness int) {
	t.Helper()
	for y := y0; y < y0+thickness; y++ {
		for x := 0; x < width; x++ {
			if !pixelSet(c, x, y) {
				t.Errorf("pixel(%d,%d) not set, want decoration line painted", x, y)
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
			{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 1}},
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
			{Y: y, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 1}},
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
			{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 1}},
			{Y: lh, Element: receipt.Text{Content: "B"}, Style: layout.Style{Size: 1}},
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
			{Y: 0, Element: receipt.Text{Content: "Milk"}, Style: layout.Style{Size: 1}},
			{Y: f.LineHeight(), Element: receipt.Text{Content: "Eggs"}, Style: layout.Style{Size: 1}},
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

// unsupportedElement is a receipt.Element with no canvas.Paint support,
// used only to exercise Paint's "unrecognized type" error path — now
// that receipt.Divider is a real, supported element, it can no longer
// stand in for "unsupported" the way earlier tests used it.
type unsupportedElement struct{}

func (unsupportedElement) Validate() error { return nil }

// assertRowClear fails t unless every pixel in row y across [0, width) is
// unset — used to prove a divider's line stops exactly at its documented
// thickness rather than bleeding into neighbouring rows.
func assertRowClear(t *testing.T, c *canvas.Canvas, width, y int) {
	t.Helper()
	for x := 0; x < width; x++ {
		if pixelSet(c, x, y) {
			t.Errorf("pixel(%d,%d) set, want row clear (outside divider thickness)", x, y)
		}
	}
}

func TestPaint_OneDividerBlock_PaintsFullWidthLine(t *testing.T) {
	doc := layout.Document{
		WidthDots: 100,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Divider{}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Width != 100 {
		t.Fatalf("c.Width = %d, want 100", c.Width)
	}
	assertHLineSet(t, c, 100, 0, layout.DividerThickness)
}

func TestPaint_DividerRespectsDocumentWidth_NotContentWidth(t *testing.T) {
	// A Divider must never assume a fixed width of its own: it spans
	// whatever Document.WidthDots resolved to, the same printable width
	// every other Block is painted against.
	narrow := layout.Document{WidthDots: 50, Font: layout.EmbeddedFont{}, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Divider{}, Style: layout.Style{Size: 1}},
	}}
	wide := layout.Document{WidthDots: 300, Font: layout.EmbeddedFont{}, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Divider{}, Style: layout.Style{Size: 1}},
	}}

	cn, err := canvas.Paint(narrow)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	cw, err := canvas.Paint(wide)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	assertHLineSet(t, cn, 50, 0, layout.DividerThickness)
	assertHLineSet(t, cw, 300, 0, layout.DividerThickness)
}

func TestPaint_DividerThickness_StopsExactlyAtDocumentedThickness(t *testing.T) {
	doc := layout.Document{
		WidthDots: 20,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Divider{}, Style: layout.Style{Size: 1}},
			{Y: layout.DividerThickness, Element: receipt.Spacer{Height: 5}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	assertHLineSet(t, c, 20, 0, layout.DividerThickness)
	assertRowClear(t, c, 20, layout.DividerThickness)
}

func TestPaint_OneDividerBlock_CanvasHeightMatchesThickness(t *testing.T) {
	doc := layout.Document{
		WidthDots: 20,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Divider{}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Height != layout.DividerThickness {
		t.Errorf("c.Height = %d, want %d (layout.DividerThickness)", c.Height, layout.DividerThickness)
	}
}

func TestPaint_DividerBetweenTextBlocks(t *testing.T) {
	f := layout.EmbeddedFont{}
	lh := f.LineHeight()
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "A"},
		receipt.Divider{},
		receipt.Text{Content: "B"},
	}}
	doc, err := layout.Build(r, printer.Profile{WidthDots: f.Measure("A")}, f)
	if err != nil {
		t.Fatalf("layout.Build() error = %v, want nil", err)
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}

	bmpA, _ := f.Glyph('A')
	bmpB, _ := f.Glyph('B')
	assertGlyphPainted(t, c, 0, bmpA)
	assertHLineSet(t, c, c.Width, lh, layout.DividerThickness)
	assertGlyphPainted(t, c, lh+layout.DividerThickness, bmpB)
}

func TestPaint_MultipleDividers_EachPaintsOwnLine(t *testing.T) {
	doc := layout.Document{
		WidthDots: 30,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Divider{}, Style: layout.Style{Size: 1}},
			{Y: layout.DividerThickness, Element: receipt.Divider{}, Style: layout.Style{Size: 1}},
			{Y: 2 * layout.DividerThickness, Element: receipt.Divider{}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	for i := 0; i < 3; i++ {
		assertHLineSet(t, c, 30, i*layout.DividerThickness, layout.DividerThickness)
	}
}

func TestPaint_DividerAfterSpacer(t *testing.T) {
	doc := layout.Document{
		WidthDots: 20,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Spacer{Height: 20}, Style: layout.Style{Size: 1}},
			{Y: 20, Element: receipt.Divider{}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	assertRowClear(t, c, 20, 0)
	assertHLineSet(t, c, 20, 20, layout.DividerThickness)
}

func TestPaint_DividerAsLastBlock(t *testing.T) {
	// Stands in for "divider before feed" — see the equivalent note on
	// layout.TestBuild_DividerAsFinalElement: receipt.Feed isn't a Go type
	// this codebase implements yet, so the closest verifiable behaviour is
	// that a trailing Divider paints and sizes the Canvas correctly with
	// nothing after it.
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "A"},
		receipt.Divider{},
	}}
	doc, err := layout.Build(r, printer.Profile{WidthDots: f.Measure("A")}, f)
	if err != nil {
		t.Fatalf("layout.Build() error = %v, want nil", err)
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if want := f.LineHeight() + layout.DividerThickness; c.Height != want {
		t.Errorf("c.Height = %d, want %d", c.Height, want)
	}
	assertHLineSet(t, c, c.Width, f.LineHeight(), layout.DividerThickness)
}

func TestPaint_DividerOnly_ContentFit_ProducesZeroWidthCanvas_NoPanic(t *testing.T) {
	// A Divider contributes no text content to measure, so — like a
	// Spacer-only Document (TestPaint_OneSpacerBlock_ProducesBlankCanvasOfHeight)
	// — a Divider-only Document with no printer.Profile has nothing to
	// size a content-fit width against. This documents that behaviour
	// rather than asserting Paint should invent a width.
	doc := layout.Document{
		WidthDots: 0,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Divider{}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Width != 0 {
		t.Errorf("c.Width = %d, want 0 (no text content to size against)", c.Width)
	}
	if c.Height != layout.DividerThickness {
		t.Errorf("c.Height = %d, want %d", c.Height, layout.DividerThickness)
	}
}

func TestPaint_DividerDeterministic(t *testing.T) {
	doc := layout.Document{
		WidthDots: 40,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Divider{}, Style: layout.Style{Size: 1}},
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

func TestPaint_DividerStyleValue_DoesNotAffectRendering(t *testing.T) {
	// receipt.Divider.Style ("solid"/"dashed", docs/ARCHITECTURE.md §3) is
	// accepted by receipt.Divider.Validate() but dashed-pattern rendering
	// is explicitly out of scope for this slice — both styles, and the
	// empty (default) style, must paint an identical solid line, the same
	// "accepted but not yet visually distinct" position Text's
	// Italic/Underline/Strikethrough fields held before their own
	// rendering landed.
	widthDots := 20
	styles := []string{"", "solid", "dashed"}
	var results [][]byte
	for _, style := range styles {
		doc := layout.Document{
			WidthDots: widthDots,
			Font:      layout.EmbeddedFont{},
			Blocks: []layout.Block{
				{Y: 0, Element: receipt.Divider{Style: style}, Style: layout.Style{Size: 1}},
			},
		}
		c, err := canvas.Paint(doc)
		if err != nil {
			t.Fatalf("Paint() error = %v, want nil (style %q)", err, style)
		}
		results = append(results, c.Bits)
	}
	for i := 1; i < len(results); i++ {
		if string(results[i]) != string(results[0]) {
			t.Errorf("Bits for style %q differ from style %q, want identical (dashed rendering not yet implemented)", styles[i], styles[0])
		}
	}
}

func TestPaint_UnsupportedElementReturnsPermanentError(t *testing.T) {
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: unsupportedElement{}},
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
			{Y: 0, Element: receipt.Heading{Content: "A"}, Style: layout.Style{Size: 1}},
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
			{Y: 0, Element: receipt.Heading{Content: "A"}, Style: layout.Style{Size: 1}},
			{Y: lh, Element: receipt.Text{Content: "B"}, Style: layout.Style{Size: 1}},
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

func TestPaint_OneSpacerBlock_ProducesBlankCanvasOfHeight(t *testing.T) {
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Spacer{Height: 20}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Width != 0 {
		t.Errorf("c.Width = %d, want 0", c.Width)
	}
	if c.Height != 20 {
		t.Errorf("c.Height = %d, want 20", c.Height)
	}
	if len(c.Bits) != 0 {
		t.Errorf("len(c.Bits) = %d, want 0 (no glyphs painted)", len(c.Bits))
	}
}

func TestPaint_SpacerAndTextBlocks_PreservesOrder(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc := layout.Document{
		Font: f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Spacer{Height: 20}},
			{Y: 20, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if want := 20 + f.LineHeight(); c.Height != want {
		t.Errorf("c.Height = %d, want %d", c.Height, want)
	}
	bmp, _ := f.Glyph('A')
	assertGlyphPainted(t, c, 20, bmp)
}

func TestPaint_DocumentWidthDots_SetsCanvasWidth(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc := layout.Document{
		WidthDots: 384,
		Font:      f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Width != 384 {
		t.Errorf("c.Width = %d, want 384 (doc.WidthDots), not content-fit (%d)", c.Width, f.Measure("A"))
	}
}

func TestPaint_NarrowContent_StillFillsFullWidthCanvas(t *testing.T) {
	// A Spacer-only Document has no text content at all, so the old
	// content-fit sizing would have produced a zero-width Canvas. With
	// doc.WidthDots set, the Canvas must still be sized to it.
	doc := layout.Document{
		WidthDots: 384,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Spacer{Height: 20}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Width != 384 {
		t.Errorf("c.Width = %d, want 384", c.Width)
	}
}

func TestPaint_DifferentDocumentWidths_ProduceDifferentCanvasWidths(t *testing.T) {
	f := layout.EmbeddedFont{}
	blocks := []layout.Block{{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 1}}}

	narrow, err := canvas.Paint(layout.Document{WidthDots: 200, Font: f, Blocks: blocks})
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	wide, err := canvas.Paint(layout.Document{WidthDots: 400, Font: f, Blocks: blocks})
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}

	if narrow.Width != 200 || wide.Width != 400 {
		t.Errorf("Width = %d, %d, want 200, 400 (each Canvas reflects its own Document.WidthDots)", narrow.Width, wide.Width)
	}
}

func TestPaint_ZeroDocumentWidthDots_FallsBackToContentFit(t *testing.T) {
	// The same assertion TestPaint_OneTextBlock_MatchesFontGlyph already
	// makes implicitly (via a Document with no WidthDots set at all) —
	// stated explicitly here as the documented "0 = content-fit" contract,
	// exercised through Build's own zero-value Profile case rather than a
	// hand-built Document.
	f := layout.EmbeddedFont{}
	doc := layout.Document{
		WidthDots: 0,
		Font:      f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Text{Content: "Milk"}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if want := f.Measure("Milk"); c.Width != want {
		t.Errorf("c.Width = %d, want %d (content-fit)", c.Width, want)
	}
}

func TestPaint_ContentFit_WidthIsMaxAcrossAllBlocks_NotJustTheFirst(t *testing.T) {
	// Content-fit width must be the max f.Measure(content) * Style.Size
	// over every block, not just the first block with non-empty content —
	// a later, wider block must still grow the Canvas rather than being
	// silently clipped to the first block's width.
	f := layout.EmbeddedFont{}
	doc := layout.Document{
		WidthDots: 0,
		Font:      f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Text{Content: "Hi"}, Style: layout.Style{Size: 1}},
			{Y: f.LineHeight(), Element: receipt.Text{Content: "This is a much longer line"}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if want := f.Measure("This is a much longer line"); c.Width != want {
		t.Errorf("c.Width = %d, want %d (widest block, not the first)", c.Width, want)
	}

	bmp, _ := f.Glyph('T')
	assertGlyphPainted(t, c, f.LineHeight(), bmp)
}

func TestPaint_ContentWiderThanDocumentWidth_ClipsWithoutPanicking(t *testing.T) {
	f := layout.EmbeddedFont{}
	// "Hello world" measures far wider than 8 dots at this embedded face;
	// before doc.WidthDots existed, the Canvas always grew to fit content,
	// so this situation could never arise. paintGlyph must clip rather
	// than index past c.Bits.
	doc := layout.Document{
		WidthDots: 8,
		Font:      f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Text{Content: "Hello world"}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Width != 8 {
		t.Errorf("c.Width = %d, want 8 (fixed to doc.WidthDots, not grown for the wider content)", c.Width)
	}
}

func TestPaint_DocumentWidthDots_Deterministic(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc := layout.Document{
		WidthDots: 384,
		Font:      f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Text{Content: "Milk"}, Style: layout.Style{Size: 1}},
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

func TestPaint_WrappedTextFromBuild_ProducesTallerCanvas(t *testing.T) {
	f := layout.EmbeddedFont{}
	width := f.Measure("Hello World") // "Foo" wraps to a second line
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Hello World Foo"},
	}}
	doc, err := layout.Build(r, printer.Profile{WidthDots: width}, f)
	if err != nil {
		t.Fatalf("layout.Build() error = %v, want nil", err)
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if want := 2 * f.LineHeight(); c.Height != want {
		t.Errorf("c.Height = %d, want %d (two wrapped lines)", c.Height, want)
	}
	if c.Width != width {
		t.Errorf("c.Width = %d, want %d (doc.WidthDots)", c.Width, width)
	}
}

func TestPaint_Size2Block_ScalesGlyphAndMeasurement(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc := layout.Document{
		Font: f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 2}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if want := f.Measure("A") * 2; c.Width != want {
		t.Errorf("c.Width = %d, want %d (f.Measure(\"A\") * Style.Size)", c.Width, want)
	}
	if want := f.LineHeight() * 2; c.Height != want {
		t.Errorf("c.Height = %d, want %d (f.LineHeight() * Style.Size)", c.Height, want)
	}
	bmp, _ := f.Glyph('A')
	assertScaledGlyphPainted(t, c, 0, bmp, 2)
}

func TestPaint_Size3Block_ScalesGlyphAndMeasurement(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc := layout.Document{
		Font: f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 3}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if want := f.Measure("A") * 3; c.Width != want {
		t.Errorf("c.Width = %d, want %d (f.Measure(\"A\") * Style.Size)", c.Width, want)
	}
	if want := f.LineHeight() * 3; c.Height != want {
		t.Errorf("c.Height = %d, want %d (f.LineHeight() * Style.Size)", c.Height, want)
	}
	bmp, _ := f.Glyph('A')
	assertScaledGlyphPainted(t, c, 0, bmp, 3)
}

func TestPaint_ScaledMultiCharContent_AdvancesByScaledWidth(t *testing.T) {
	// Per-glyph advance must scale the same way overall measurement does,
	// or a scaled multi-character string would paint with overlapping or
	// gapped glyphs even though its total measured width is correct.
	f := layout.EmbeddedFont{}
	doc := layout.Document{
		Font: f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Text{Content: "AB"}, Style: layout.Style{Size: 2}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	bmpA, advanceA := f.Glyph('A')
	bmpB, _ := f.Glyph('B')
	assertScaledGlyphPainted(t, c, 0, bmpA, 2)
	assertScaledGlyphPainted(t, c, advanceA*2, bmpB, 2)
}

func TestPaint_ScaledText_DeterministicAcrossCalls(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc := layout.Document{
		Font: f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Text{Content: "Milk"}, Style: layout.Style{Size: 2}},
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

func TestPaint_WrappedScaledTextFromBuild_HeightAccountsForScale(t *testing.T) {
	f := layout.EmbeddedFont{}
	// At Size: 2, "Hello World" needs twice f.Measure("Hello World") to
	// fit on one line (see layout.TestBuild_ScaledText_...), so doubling
	// the width here reproduces the same two-line wrap ("Hello World" /
	// "Foo") the unscaled TestPaint_WrappedTextFromBuild_... test uses.
	width := 2 * f.Measure("Hello World")
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Hello World Foo", Size: 2},
	}}
	doc, err := layout.Build(r, printer.Profile{WidthDots: width}, f)
	if err != nil {
		t.Fatalf("layout.Build() error = %v, want nil", err)
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if want := 2 * f.LineHeight() * 2; c.Height != want {
		t.Errorf("c.Height = %d, want %d (two wrapped lines, each twice f.LineHeight() at Size: 2)", c.Height, want)
	}
}

func TestPaint_BoldStyle_PaintsAtLeastAsManyPixelsAsUnstyled(t *testing.T) {
	f := layout.EmbeddedFont{}
	plain := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 1}},
	}}
	bold := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 1, Bold: true}},
	}}

	cp, err := canvas.Paint(plain)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	cb, err := canvas.Paint(bold)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}

	if cp.Width != cb.Width || cp.Height != cb.Height {
		t.Errorf("bold dimensions = %dx%d, plain = %dx%d, want equal (bold does not change glyph advance)", cb.Width, cb.Height, cp.Width, cp.Height)
	}
	if plainCount, boldCount := countSetPixels(cp), countSetPixels(cb); boldCount <= plainCount {
		t.Errorf("bold set %d pixels, plain set %d, want bold strictly more (neighbouring-pixel overdraw thickens strokes)", boldCount, plainCount)
	}
}

func TestPaint_BoldStyle_DeterministicAcrossCalls(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "Milk"}, Style: layout.Style{Size: 1, Bold: true}},
	}}
	first, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	second, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if string(first.Bits) != string(second.Bits) {
		t.Errorf("Bits differ between calls, want identical")
	}
}

func TestPaint_HeadingAndTextWithSameStyle_RenderIdentically(t *testing.T) {
	// Heading is presentation sugar over Text (docs/ARCHITECTURE.md §3):
	// given the same resolved Style, canvas.Paint must not treat a
	// receipt.Heading Block any differently from a receipt.Text Block —
	// there is exactly one rendering pipeline.
	f := layout.EmbeddedFont{}
	style := layout.Style{Bold: true, Size: 2}
	headingDoc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Heading{Content: "A"}, Style: style},
	}}
	textDoc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "A"}, Style: style},
	}}

	ch, err := canvas.Paint(headingDoc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	ct, err := canvas.Paint(textDoc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}

	if ch.Width != ct.Width || ch.Height != ct.Height {
		t.Fatalf("Heading dimensions = %dx%d, Text = %dx%d, want equal", ch.Width, ch.Height, ct.Width, ct.Height)
	}
	if string(ch.Bits) != string(ct.Bits) {
		t.Errorf("Heading and Text Bits differ given the same Style, want identical")
	}
}

func TestPaint_ItalicStyle_PreservesMeasurementAndDimensions(t *testing.T) {
	// Italic is a bitmap shear, not a width change (docs/ARCHITECTURE.md
	// §3): the sheared glyph still occupies its original Width, so
	// measurement (f.Measure) stays valid for italic content without a
	// second, italic-aware measurement path.
	f := layout.EmbeddedFont{}
	plain := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 1}},
	}}
	italic := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 1, Italic: true}},
	}}

	cp, err := canvas.Paint(plain)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	ci, err := canvas.Paint(italic)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if cp.Width != ci.Width || cp.Height != ci.Height {
		t.Errorf("italic dimensions = %dx%d, plain = %dx%d, want equal (italic does not change glyph advance)", ci.Width, ci.Height, cp.Width, cp.Height)
	}
}

func TestPaint_ItalicStyle_ChangesPixelArrangement(t *testing.T) {
	f := layout.EmbeddedFont{}
	plain := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 1}},
	}}
	italic := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 1, Italic: true}},
	}}

	cp, err := canvas.Paint(plain)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	ci, err := canvas.Paint(italic)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if string(cp.Bits) == string(ci.Bits) {
		t.Errorf("italic Bits identical to plain, want a sheared (different) pixel arrangement")
	}
}

func TestPaint_ItalicStyle_DeterministicAcrossCalls(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "Milk"}, Style: layout.Style{Size: 1, Italic: true}},
	}}
	first, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	second, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if string(first.Bits) != string(second.Bits) {
		t.Errorf("Bits differ between calls, want identical")
	}
}

func TestPaint_BoldItalicComposition_ProducesDistinctDeterministicResult(t *testing.T) {
	// Composition must not degrade into "only one style wins": bold-only,
	// italic-only, and bold+italic together must all paint distinct
	// pixel arrangements, and bold+italic must itself be deterministic.
	f := layout.EmbeddedFont{}
	styles := map[string]layout.Style{
		"bold":       {Size: 1, Bold: true},
		"italic":     {Size: 1, Italic: true},
		"boldItalic": {Size: 1, Bold: true, Italic: true},
	}
	results := make(map[string]*canvas.Canvas, len(styles))
	for name, style := range styles {
		doc := layout.Document{Font: f, Blocks: []layout.Block{
			{Y: 0, Element: receipt.Text{Content: "A"}, Style: style},
		}}
		c, err := canvas.Paint(doc)
		if err != nil {
			t.Fatalf("Paint() error = %v, want nil", err)
		}
		results[name] = c
	}

	if string(results["bold"].Bits) == string(results["boldItalic"].Bits) {
		t.Errorf("bold+italic Bits identical to bold-only, want italic to have visibly composed")
	}
	if string(results["italic"].Bits) == string(results["boldItalic"].Bits) {
		t.Errorf("bold+italic Bits identical to italic-only, want bold to have visibly composed")
	}

	doc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "A"}, Style: styles["boldItalic"]},
	}}
	again, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if string(again.Bits) != string(results["boldItalic"].Bits) {
		t.Errorf("bold+italic Bits differ between calls, want identical")
	}
}

func TestPaint_SizeItalicComposition_ScalesDimensions(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 2, Italic: true}},
	}}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if want := f.Measure("A") * 2; c.Width != want {
		t.Errorf("c.Width = %d, want %d (italic does not change scaled measurement)", c.Width, want)
	}
	if want := f.LineHeight() * 2; c.Height != want {
		t.Errorf("c.Height = %d, want %d (italic does not change scaled measurement)", c.Height, want)
	}
}

func TestPaint_UnderlineStyle_DrawsLineAtBottomOfBlock(t *testing.T) {
	f := layout.EmbeddedFont{}
	width := f.Measure("A")
	lh := f.LineHeight()
	doc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 1, Underline: true}},
	}}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	assertHLineSet(t, c, width, lh-1, 1)

	// The decoration must not have modified the glyph bitmap itself —
	// every row outside the underline's own band still matches the
	// unstyled glyph exactly.
	bmp, _ := f.Glyph('A')
	assertGlyphPaintedExceptRows(t, c, 0, bmp, lh-1, lh)
}

func TestPaint_UnderlineStyle_ScalesWithSize(t *testing.T) {
	f := layout.EmbeddedFont{}
	const size = 2
	width := f.Measure("A") * size
	lh := f.LineHeight() * size
	doc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: size, Underline: true}},
	}}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	assertHLineSet(t, c, width, lh-size, size)
}

func TestPaint_StrikethroughStyle_DrawsLineThroughMiddle(t *testing.T) {
	f := layout.EmbeddedFont{}
	width := f.Measure("A")
	lh := f.LineHeight()
	doc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 1, Strikethrough: true}},
	}}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	assertHLineSet(t, c, width, lh/2, 1)

	// Same guarantee as the underline test above: rows outside the
	// strikethrough's own band are unaffected.
	bmp, _ := f.Glyph('A')
	assertGlyphPaintedExceptRows(t, c, 0, bmp, lh/2, lh/2+1)
}

func TestPaint_StrikethroughStyle_ScalesWithSize(t *testing.T) {
	f := layout.EmbeddedFont{}
	const size = 2
	width := f.Measure("A") * size
	lh := f.LineHeight() * size
	doc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: size, Strikethrough: true}},
	}}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	assertHLineSet(t, c, width, lh/2, size)
}

func TestPaint_UnderlineAndStrikethrough_BothRender(t *testing.T) {
	f := layout.EmbeddedFont{}
	width := f.Measure("A")
	lh := f.LineHeight()
	doc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 1, Underline: true, Strikethrough: true}},
	}}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	assertHLineSet(t, c, width, lh-1, 1)
	assertHLineSet(t, c, width, lh/2, 1)
}

func TestPaint_EmptyContentWithUnderline_DoesNotPanic(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: ""}, Style: layout.Style{Size: 1, Underline: true, Strikethrough: true}},
	}}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Width != 0 {
		t.Errorf("c.Width = %d, want 0 (no content to underline)", c.Width)
	}
}

func TestPaint_AllStylesComposed_DeterministicAcrossCalls(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "Milk"}, Style: layout.Style{
			Size: 2, Bold: true, Italic: true, Underline: true, Strikethrough: true,
		}},
	}}
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

func TestPaint_HeadingWithUnderline_RendersSameAsTextWithSameStyle(t *testing.T) {
	// Extends TestPaint_HeadingAndTextWithSameStyle_RenderIdentically to
	// decorations: there is still exactly one rendering pipeline once
	// underline exists.
	f := layout.EmbeddedFont{}
	style := layout.Style{Bold: true, Size: 2, Underline: true, Strikethrough: true}
	headingDoc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Heading{Content: "A"}, Style: style},
	}}
	textDoc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "A"}, Style: style},
	}}

	ch, err := canvas.Paint(headingDoc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	ct, err := canvas.Paint(textDoc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if string(ch.Bits) != string(ct.Bits) {
		t.Errorf("Heading and Text Bits differ given the same Style, want identical")
	}
}

func TestPaint_UnsupportedElementAmongSupportedOnes(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc := layout.Document{
		Font: f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Text{Content: "Milk"}, Style: layout.Style{Size: 1}},
			{Y: f.LineHeight(), Element: unsupportedElement{}},
		},
	}
	_, err := canvas.Paint(doc)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Paint() error = %v, want apperr.KindPermanent", err)
	}
}
