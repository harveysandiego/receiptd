package canvas_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// solidPNG returns the encoded bytes of a width x height PNG filled with
// c — the same minimal deterministic fixture render/layout's own tests use.
func solidPNG(t *testing.T, width, height int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode() error = %v, want nil", err)
	}
	return buf.Bytes()
}

func TestPaint_OneImageBlock_MatchesDecodeImageBitmap(t *testing.T) {
	data := solidPNG(t, 4, 3, color.Black)
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Image{Data: data}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	bmp, err := layout.DecodeImageBitmap(data, 0)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	if c.Width != bmp.Width || c.Height != bmp.Height {
		t.Fatalf("Canvas = %dx%d, want %dx%d", c.Width, c.Height, bmp.Width, bmp.Height)
	}
	assertGlyphPainted(t, c, 0, bmp)
}

func TestPaint_ImageRespectsDocumentWidth_ScalesDown(t *testing.T) {
	data := solidPNG(t, 20, 10, color.Black)
	doc := layout.Document{
		WidthDots: 10,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Image{Data: data}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Width != 10 {
		t.Errorf("c.Width = %d, want 10 (doc.WidthDots)", c.Width)
	}
	if c.Height != 5 {
		t.Errorf("c.Height = %d, want 5 (scaled preserving aspect ratio)", c.Height)
	}
}

func TestPaint_ImageBetweenTextBlocks(t *testing.T) {
	f := layout.EmbeddedFont{}
	lh := f.LineHeight()
	data := solidPNG(t, 4, 3, color.Black)
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "A"},
		receipt.Image{Data: data},
		receipt.Text{Content: "B"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: f.Measure("A") + 20}, f, nil)
	if err != nil {
		t.Fatalf("layout.Build() error = %v, want nil", err)
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}

	bmpA, _ := f.Glyph('A')
	bmpB, _ := f.Glyph('B')
	bmpImg, err := layout.DecodeImageBitmap(data, doc.WidthDots)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	assertGlyphPainted(t, c, 0, bmpA)
	assertGlyphPainted(t, c, lh, bmpImg)
	assertGlyphPainted(t, c, lh+bmpImg.Height, bmpB)
}

func TestPaint_ImageAfterDivider(t *testing.T) {
	data := solidPNG(t, 4, 3, color.Black)
	doc := layout.Document{
		WidthDots: 20,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Divider{}, Style: layout.Style{Size: 1}},
			{Y: layout.DividerThickness, Element: receipt.Image{Data: data}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	bmp, err := layout.DecodeImageBitmap(data, 20)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	assertHLineSet(t, c, 20, 0, layout.DividerThickness)
	assertGlyphPainted(t, c, layout.DividerThickness, bmp)
}

func TestPaint_ImageOnly_DocumentHeightMatchesImageHeight(t *testing.T) {
	data := solidPNG(t, 4, 9, color.Black)
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Image{Data: data}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Height != 9 {
		t.Errorf("c.Height = %d, want 9", c.Height)
	}
}

func TestPaint_ImageOnly_ContentFitWidthMatchesImageWidth(t *testing.T) {
	// Like text, an Image contributes to content-fit Canvas sizing when no
	// printer.Profile constrains doc.WidthDots — otherwise an image-only
	// Document with no printer configured would silently produce a
	// zero-width Canvas and clip every painted pixel.
	data := solidPNG(t, 6, 4, color.Black)
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Image{Data: data}, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Width != 6 {
		t.Errorf("c.Width = %d, want 6 (content-fit to the image's own width)", c.Width)
	}
}

func TestPaint_InvalidImageData_ReturnsPermanentError(t *testing.T) {
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Image{Data: []byte("not an image")}, Style: layout.Style{Size: 1}},
		},
	}
	_, err := canvas.Paint(doc)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Paint() error = %v, want apperr.KindPermanent", err)
	}
}

func TestPaint_ImageDeterministic(t *testing.T) {
	data := solidPNG(t, 4, 3, color.Black)
	doc := layout.Document{
		WidthDots: 20,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Image{Data: data}, Style: layout.Style{Size: 1}},
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

// solidJPEG returns the encoded bytes of a width x height JPEG filled
// with c — used only to prove Paint has no PNG-specific logic: it paints
// whatever layout.DecodeImageBitmap decodes, regardless of the source
// receipt.Image.Data's format (docs/ARCHITECTURE.md §4's "exactly one
// rendering pipeline" — render/canvas.Paint never inspects a format
// itself, only render/layout does, at decode time).
func solidJPEG(t *testing.T, width, height int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatalf("jpeg.Encode() error = %v, want nil", err)
	}
	return buf.Bytes()
}

func TestPaint_NonPNGFormat_PaintsIdenticallyToEquivalentPNG(t *testing.T) {
	pngDoc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Image{Data: solidPNG(t, 4, 3, color.Black)}, Style: layout.Style{Size: 1}},
		},
	}
	jpegDoc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Image{Data: solidJPEG(t, 4, 3, color.Black)}, Style: layout.Style{Size: 1}},
		},
	}
	cp, err := canvas.Paint(pngDoc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil (PNG)", err)
	}
	cj, err := canvas.Paint(jpegDoc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil (JPEG)", err)
	}
	if cp.Width != cj.Width || cp.Height != cj.Height {
		t.Fatalf("PNG = %dx%d, JPEG = %dx%d, want equal", cp.Width, cp.Height, cj.Width, cj.Height)
	}
	if string(cp.Bits) != string(cj.Bits) {
		t.Errorf("PNG and JPEG Bits differ for an equivalent solid-colour image, want identical")
	}
}

func TestPaint_SVGImageData_ReturnsPermanentErrorNotPanic(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="2" height="2"><rect width="2" height="2"/></svg>`)
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Image{Data: svg}, Style: layout.Style{Size: 1}},
		},
	}
	_, err := canvas.Paint(doc)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Paint() error = %v, want apperr.KindPermanent", err)
	}
}
