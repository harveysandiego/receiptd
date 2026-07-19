package canvas_test

import (
	"image/color"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

func TestPaint_OneQRCodeBlock_MatchesGenerateQRCodeBitmap(t *testing.T) {
	qr := receipt.QRCode{Content: "https://example.com", Size: 40}
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: qr, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	bmp, err := layout.GenerateQRCodeBitmap(qr, 0)
	if err != nil {
		t.Fatalf("GenerateQRCodeBitmap() error = %v, want nil", err)
	}
	if c.Width != bmp.Width || c.Height != bmp.Height {
		t.Fatalf("Canvas = %dx%d, want %dx%d", c.Width, c.Height, bmp.Width, bmp.Height)
	}
	assertGlyphPainted(t, c, 0, bmp)
}

func TestPaint_QRCodeRespectsDocumentWidth_ScalesDown(t *testing.T) {
	qr := receipt.QRCode{Content: "https://example.com", Size: 40}
	doc := layout.Document{
		WidthDots: 20,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: qr, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Width != 20 {
		t.Errorf("c.Width = %d, want 20 (doc.WidthDots)", c.Width)
	}
	if c.Height != 20 {
		t.Errorf("c.Height = %d, want 20 (scaled, still square)", c.Height)
	}
}

func TestPaint_QRCodeBetweenTextBlocks(t *testing.T) {
	f := layout.EmbeddedFont{}
	lh := f.LineHeight()
	qr := receipt.QRCode{Content: "https://example.com", Size: 30}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "A"},
		qr,
		receipt.Text{Content: "B"},
	}}
	doc, err := layout.Build(r, printer.Profile{WidthDots: f.Measure("A") + 40}, f)
	if err != nil {
		t.Fatalf("layout.Build() error = %v, want nil", err)
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}

	bmpA, _ := f.Glyph('A')
	bmpB, _ := f.Glyph('B')
	bmpQR, err := layout.GenerateQRCodeBitmap(qr, doc.WidthDots)
	if err != nil {
		t.Fatalf("GenerateQRCodeBitmap() error = %v, want nil", err)
	}
	assertGlyphPainted(t, c, 0, bmpA)
	assertGlyphPainted(t, c, lh, bmpQR)
	assertGlyphPainted(t, c, lh+bmpQR.Height, bmpB)
}

func TestPaint_QRCodeAfterDivider(t *testing.T) {
	qr := receipt.QRCode{Content: "https://example.com", Size: 20}
	doc := layout.Document{
		WidthDots: 20,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Divider{}, Style: layout.Style{Size: 1}},
			{Y: layout.DividerThickness, Element: qr, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	bmp, err := layout.GenerateQRCodeBitmap(qr, 20)
	if err != nil {
		t.Fatalf("GenerateQRCodeBitmap() error = %v, want nil", err)
	}
	assertHLineSet(t, c, 20, 0, layout.DividerThickness)
	assertGlyphPainted(t, c, layout.DividerThickness, bmp)
}

func TestPaint_QRCodeOnly_DocumentHeightMatchesQRCodeHeight(t *testing.T) {
	qr := receipt.QRCode{Content: "https://example.com", Size: 33}
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: qr, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Height != 33 {
		t.Errorf("c.Height = %d, want 33", c.Height)
	}
}

func TestPaint_QRCodeOnly_ContentFitWidthMatchesQRCodeWidth(t *testing.T) {
	qr := receipt.QRCode{Content: "https://example.com", Size: 33}
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: qr, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Width != 33 {
		t.Errorf("c.Width = %d, want 33 (content-fit to the QR code's own width)", c.Width)
	}
}

func TestPaint_QRCodeContentTooLargeForCapacity_ReturnsPermanentError(t *testing.T) {
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.QRCode{Content: strings.Repeat("A", 10000)}, Style: layout.Style{Size: 1}},
		},
	}
	_, err := canvas.Paint(doc)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Paint() error = %v, want apperr.KindPermanent", err)
	}
}

func TestPaint_QRCodeDeterministic(t *testing.T) {
	qr := receipt.QRCode{Content: "https://example.com", Size: 30}
	doc := layout.Document{
		WidthDots: 20,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: qr, Style: layout.Style{Size: 1}},
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

func TestPaint_QRCodeAndImageSideBySideBlocks_BothPaintViaSameRasterPath(t *testing.T) {
	// Not literally side-by-side (columns are out of scope), but both in
	// the same Document — exercising that Paint's raster dispatch handles
	// a mix of receipt.Image and receipt.QRCode Blocks without either
	// disturbing the other's bitmap resolution.
	data := solidPNG(t, 4, 3, color.Black)
	qr := receipt.QRCode{Content: "https://example.com", Size: 20}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Image{Data: data},
		qr,
	}}
	doc, err := layout.Build(r, printer.Profile{}, layout.EmbeddedFont{})
	if err != nil {
		t.Fatalf("layout.Build() error = %v, want nil", err)
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	bmpImg, err := layout.DecodeImageBitmap(data, doc.WidthDots)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	bmpQR, err := layout.GenerateQRCodeBitmap(qr, doc.WidthDots)
	if err != nil {
		t.Fatalf("GenerateQRCodeBitmap() error = %v, want nil", err)
	}
	assertGlyphPainted(t, c, 0, bmpImg)
	assertGlyphPainted(t, c, bmpImg.Height, bmpQR)
}
