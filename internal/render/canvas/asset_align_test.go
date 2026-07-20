package canvas_test

import (
	"image/color"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

func TestPaint_OneAlignedAssetBlock_MatchesDecodeAlignedAssetBitmap(t *testing.T) {
	// No doc.WidthDots (0, Build's "no printer configured" sentinel): the
	// Canvas sizes itself to fit the painted content, exactly like
	// TestPaint_OneBarcodeBlock_MatchesGenerateBarcodeBitmap's analogous
	// receipt.Barcode proof.
	a := layout.AlignedAsset{Data: solidPNG(t, 4, 3, color.Black)}
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: a, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	bmp, err := layout.DecodeAlignedAssetBitmap(a, 0)
	if err != nil {
		t.Fatalf("DecodeAlignedAssetBitmap() error = %v, want nil", err)
	}
	if c.Width != bmp.Width || c.Height != bmp.Height {
		t.Fatalf("Canvas = %dx%d, want %dx%d", c.Width, c.Height, bmp.Width, bmp.Height)
	}
	assertGlyphPainted(t, c, 0, bmp)
}

func TestPaint_AlignedAsset_LeftAlign_PaintsFlushAgainstX0(t *testing.T) {
	data := solidPNG(t, 4, 2, color.Black)
	doc := layout.Document{
		WidthDots: 100,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: layout.AlignedAsset{Data: data, Align: "left"}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if !pixelSet(c, 0, 0) {
		t.Errorf("pixel(0,0) not set, want set (left-aligned asset starts at x=0)")
	}
}

func TestPaint_AlignedAsset_RightAlign_PaintsFlushAgainstRightEdge(t *testing.T) {
	data := solidPNG(t, 4, 2, color.Black)
	doc := layout.Document{
		WidthDots: 100,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: layout.AlignedAsset{Data: data, Align: "right"}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if pixelSet(c, 0, 0) {
		t.Errorf("pixel(0,0) set, want unset (right-aligned asset must not start at x=0)")
	}
	if !pixelSet(c, c.Width-1, 0) {
		t.Errorf("pixel(%d,0) not set, want set (right-aligned asset flush against the right edge)", c.Width-1)
	}
}

func TestPaint_AlignedAsset_CenterAlign_PaintsBetweenLeftAndRightEdges(t *testing.T) {
	data := solidPNG(t, 4, 2, color.Black)
	doc := layout.Document{
		WidthDots: 100,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: layout.AlignedAsset{Data: data, Align: "center"}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if pixelSet(c, 0, 0) {
		t.Errorf("pixel(0,0) set, want unset (centered asset must not start at x=0)")
	}
	if pixelSet(c, c.Width-1, 0) {
		t.Errorf("pixel(%d,0) set, want unset (centered asset must not reach the right edge)", c.Width-1)
	}
	if !pixelSet(c, 48, 0) {
		t.Errorf("pixel(48,0) not set, want set ((100-4)/2 = 48, the centered asset's left edge)")
	}
}

func TestPaint_AlignedAsset_ExplicitWidth_ScalesBeforePainting(t *testing.T) {
	data := solidPNG(t, 4, 2, color.Black)
	doc := layout.Document{
		WidthDots: 100,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: layout.AlignedAsset{Data: data, Width: 40}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Height != 20 {
		t.Errorf("c.Height = %d, want 20 (4x2 scaled to Width 40, aspect ratio preserved)", c.Height)
	}
	if !pixelSet(c, 39, 10) {
		t.Errorf("pixel(39,10) not set, want set (scaled asset's rightmost column)")
	}
}

func TestPaint_AlignedAssetInvalidData_ReturnsPermanentError(t *testing.T) {
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: layout.AlignedAsset{Data: []byte("not an image")}, Style: layout.Style{Size: 1}},
		},
	}
	_, err := canvas.Paint(doc)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Paint() error = %v, want apperr.KindPermanent", err)
	}
}

func TestPaint_AlignedAssetBetweenTextBlocks_PreservesOrderAndPosition(t *testing.T) {
	f := layout.EmbeddedFont{}
	lh := f.LineHeight()
	data := solidPNG(t, 4, 6, color.Black)
	a := layout.AlignedAsset{Data: data}
	doc := layout.Document{
		WidthDots: 100,
		Font:      f,
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Text{Content: "A"}, Style: layout.Style{Size: 1}},
			{Y: lh, Element: a, Style: layout.Style{Size: 1}},
			{Y: lh + 6, Element: receipt.Text{Content: "B"}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	bmp, err := layout.DecodeAlignedAssetBitmap(a, 100)
	if err != nil {
		t.Fatalf("DecodeAlignedAssetBitmap() error = %v, want nil", err)
	}
	assertGlyphPainted(t, c, lh, bmp)
}

func TestPaint_AlignedAssetDeterministic(t *testing.T) {
	a := layout.AlignedAsset{Data: solidPNG(t, 4, 3, color.Black), Width: 20, Align: "center"}
	doc := layout.Document{
		WidthDots: 100,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: a, Style: layout.Style{Size: 1}},
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
