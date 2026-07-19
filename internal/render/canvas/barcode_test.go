package canvas_test

import (
	"image/color"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

func TestPaint_OneBarcodeBlock_MatchesGenerateBarcodeBitmap(t *testing.T) {
	bc := receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 40}
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: bc, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	bmp, err := layout.GenerateBarcodeBitmap(bc, 0)
	if err != nil {
		t.Fatalf("GenerateBarcodeBitmap() error = %v, want nil", err)
	}
	if c.Width != bmp.Width || c.Height != bmp.Height {
		t.Fatalf("Canvas = %dx%d, want %dx%d", c.Width, c.Height, bmp.Width, bmp.Height)
	}
	assertGlyphPainted(t, c, 0, bmp)
}

func TestPaint_BarcodeRespectsDocumentWidth_ScalesDownWidthOnly(t *testing.T) {
	bc := receipt.Barcode{Content: "HELLO-128-WITH-EXTRA-CONTENT-TO-MAKE-IT-WIDE", Symbology: "code128", Height: 40}
	doc := layout.Document{
		WidthDots: 20,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: bc, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if c.Width != 20 {
		t.Errorf("c.Width = %d, want 20 (doc.WidthDots)", c.Width)
	}
	if c.Height != 40 {
		t.Errorf("c.Height = %d, want 40 (unaffected by width scaling)", c.Height)
	}
}

func TestPaint_BarcodeBetweenTextBlocks(t *testing.T) {
	f := layout.EmbeddedFont{}
	lh := f.LineHeight()
	bc := receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 30}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "A"},
		bc,
		receipt.Text{Content: "B"},
	}}
	doc, err := layout.Build(r, printer.Profile{WidthDots: f.Measure("A") + 200}, f)
	if err != nil {
		t.Fatalf("layout.Build() error = %v, want nil", err)
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}

	bmpA, _ := f.Glyph('A')
	bmpB, _ := f.Glyph('B')
	bmpBarcode, err := layout.GenerateBarcodeBitmap(bc, doc.WidthDots)
	if err != nil {
		t.Fatalf("GenerateBarcodeBitmap() error = %v, want nil", err)
	}
	assertGlyphPainted(t, c, 0, bmpA)
	assertGlyphPainted(t, c, lh, bmpBarcode)
	assertGlyphPainted(t, c, lh+bmpBarcode.Height, bmpB)
}

func TestPaint_BarcodeAfterDivider(t *testing.T) {
	bc := receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 20}
	doc := layout.Document{
		WidthDots: 200,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Divider{}, Style: layout.Style{Size: 1}},
			{Y: layout.DividerThickness, Element: bc, Style: layout.Style{Size: 1}},
		},
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	bmp, err := layout.GenerateBarcodeBitmap(bc, 200)
	if err != nil {
		t.Fatalf("GenerateBarcodeBitmap() error = %v, want nil", err)
	}
	assertHLineSet(t, c, 200, 0, layout.DividerThickness)
	assertGlyphPainted(t, c, layout.DividerThickness, bmp)
}

func TestPaint_BarcodeOnly_DocumentHeightMatchesBarcodeHeight(t *testing.T) {
	bc := receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 33}
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: bc, Style: layout.Style{Size: 1}},
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

func TestPaint_BarcodeInvalidContent_ReturnsPermanentError(t *testing.T) {
	doc := layout.Document{
		Font: layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: receipt.Barcode{Content: "12345", Symbology: "itf"}, Style: layout.Style{Size: 1}},
		},
	}
	_, err := canvas.Paint(doc)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Paint() error = %v, want apperr.KindPermanent", err)
	}
}

func TestPaint_BarcodeDeterministic(t *testing.T) {
	bc := receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 30}
	doc := layout.Document{
		WidthDots: 200,
		Font:      layout.EmbeddedFont{},
		Blocks: []layout.Block{
			{Y: 0, Element: bc, Style: layout.Style{Size: 1}},
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

func TestPaint_BarcodeImageAndQRCodeTogether_AllPaintViaSameRasterPath(t *testing.T) {
	// Exercises Paint's raster dispatch handling a mix of receipt.Image,
	// receipt.QRCode, and receipt.Barcode Blocks in one Document, without
	// any of the three disturbing the others' bitmap resolution.
	data := solidPNG(t, 4, 3, color.Black)
	qr := receipt.QRCode{Content: "https://example.com", Size: 20}
	bc := receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 20}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Image{Data: data},
		qr,
		bc,
	}}
	doc, err := layout.Build(r, printer.Profile{WidthDots: 200}, layout.EmbeddedFont{})
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
	bmpBarcode, err := layout.GenerateBarcodeBitmap(bc, doc.WidthDots)
	if err != nil {
		t.Fatalf("GenerateBarcodeBitmap() error = %v, want nil", err)
	}
	assertGlyphPainted(t, c, 0, bmpImg)
	assertGlyphPainted(t, c, bmpImg.Height, bmpQR)
	assertGlyphPainted(t, c, bmpImg.Height+bmpQR.Height, bmpBarcode)
}
