package layout_test

import (
	"context"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

func TestBuild_OneBarcode(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Barcode{Content: "HELLO-128", Symbology: "code128"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(doc.Blocks) = %d, want 1", len(doc.Blocks))
	}
	if doc.Blocks[0].Y != 0 {
		t.Errorf("doc.Blocks[0].Y = %d, want 0", doc.Blocks[0].Y)
	}
	got, ok := doc.Blocks[0].Element.(receipt.Barcode)
	if !ok {
		t.Fatalf("doc.Blocks[0].Element = %T, want receipt.Barcode", doc.Blocks[0].Element)
	}
	if got.Content != "HELLO-128" {
		t.Errorf("doc.Blocks[0].Element.Content changed, want unchanged")
	}
}

func TestBuild_BarcodeAdvancesYByDefaultHeight_NoPrinterProfile(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Barcode{Content: "HELLO-128", Symbology: "code128"},
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2", len(doc.Blocks))
	}
	if doc.Blocks[1].Y != receipt.DefaultBarcodeHeight {
		t.Errorf("doc.Blocks[1].Y = %d, want %d (default barcode height)", doc.Blocks[1].Y, receipt.DefaultBarcodeHeight)
	}
}

func TestBuild_BarcodeExplicitHeight(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 40},
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if doc.Blocks[1].Y != 40 {
		t.Errorf("doc.Blocks[1].Y = %d, want 40", doc.Blocks[1].Y)
	}
}

func TestBuild_BarcodeWiderThanPrintableWidth_ScalesDownWidthOnly(t *testing.T) {
	// A wide Code 128 payload, scaled down to fit a narrow printer profile —
	// Height must stay exactly as configured (40), unlike QRCode where
	// width and height scale together.
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Barcode{Content: "HELLO-128-WITH-LOTS-OF-CONTENT-TO-MAKE-IT-WIDE", Symbology: "code128", Height: 40},
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 20}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if doc.Blocks[1].Y != 40 {
		t.Errorf("doc.Blocks[1].Y = %d, want 40 (Height unaffected by width scaling)", doc.Blocks[1].Y)
	}
}

func TestBuild_BarcodeBetweenTextBlocks_PreservesOrderAndPosition(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Before"},
		receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 30},
		receipt.Text{Content: "After"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("len(doc.Blocks) = %d, want 3", len(doc.Blocks))
	}
	if wantY := f.LineHeight(); doc.Blocks[1].Y != wantY {
		t.Errorf("doc.Blocks[1].Y = %d, want %d", doc.Blocks[1].Y, wantY)
	}
	if _, ok := doc.Blocks[1].Element.(receipt.Barcode); !ok {
		t.Fatalf("doc.Blocks[1].Element = %T, want receipt.Barcode", doc.Blocks[1].Element)
	}
	if wantY := f.LineHeight() + 30; doc.Blocks[2].Y != wantY {
		t.Errorf("doc.Blocks[2].Y = %d, want %d", doc.Blocks[2].Y, wantY)
	}
}

func TestBuild_BarcodeAfterDivider(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Divider{},
		receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 25},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2", len(doc.Blocks))
	}
	if doc.Blocks[1].Y != layout.DividerThickness {
		t.Errorf("doc.Blocks[1].Y = %d, want %d", doc.Blocks[1].Y, layout.DividerThickness)
	}
}

func TestBuild_BarcodeDeterministic(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Milk"},
		receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 30},
		receipt.Text{Content: "Eggs"},
	}}
	f := layout.EmbeddedFont{}

	first, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 100}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	second, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 100}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(first.Blocks) != len(second.Blocks) {
		t.Fatalf("len(first.Blocks) = %d, len(second.Blocks) = %d, want equal", len(first.Blocks), len(second.Blocks))
	}
	for i := range first.Blocks {
		a, b := first.Blocks[i], second.Blocks[i]
		if a.Y != b.Y || a.Style != b.Style || a.Element != b.Element {
			t.Errorf("Blocks[%d] differs between calls, want equal", i)
		}
	}
}

func TestBuild_BarcodeInvalidContent_ReturnsPermanentError(t *testing.T) {
	// layout.Build trusts its caller has already run receipt.Receipt.Validate()
	// (docs/ARCHITECTURE.md §5); a directly hand-built Receipt bypassing that
	// still must not panic — Build's own encode failure is reported as
	// apperr.KindPermanent, the same Kind receipt.Image's and receipt.QRCode's
	// decode/encode failures use.
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Barcode{Content: "12345", Symbology: "itf"}, // odd digit count
	}}
	_, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Build() error = %v, want apperr.KindPermanent", err)
	}
}

// --- receipt.Barcode.ShowText: caption Block ---

func TestBuild_BarcodeShowTextFalse_NoCaptionBlock(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Barcode{Content: "HELLO-128", Symbology: "code128"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(doc.Blocks) = %d, want 1 (no caption Block when ShowText is false)", len(doc.Blocks))
	}
}

func TestBuild_BarcodeShowTextTrue_AddsCaptionBlock(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Barcode{Content: "HELLO-128", Symbology: "code128", ShowText: true},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2 (barcode + caption)", len(doc.Blocks))
	}
	caption, ok := doc.Blocks[1].Element.(layout.BarcodeCaption)
	if !ok {
		t.Fatalf("doc.Blocks[1].Element = %T, want layout.BarcodeCaption", doc.Blocks[1].Element)
	}
	if got := strings.TrimSpace(caption.Content); got != "HELLO-128" {
		t.Errorf("caption.Content = %q (trimmed %q), want %q", caption.Content, got, "HELLO-128")
	}
}

func TestBuild_BarcodeShowTextTrue_CaptionPositionedBelowBarcode(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 40, ShowText: true},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if doc.Blocks[1].Y != 40 {
		t.Errorf("doc.Blocks[1].Y = %d, want 40 (immediately below the barcode's own height)", doc.Blocks[1].Y)
	}
}

func TestBuild_BarcodeShowTextTrue_AdvancesYByCaptionLineHeight(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 40, ShowText: true},
		receipt.Text{Content: "After"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("len(doc.Blocks) = %d, want 3 (barcode + caption + text)", len(doc.Blocks))
	}
	if want := 40 + f.LineHeight(); doc.Blocks[2].Y != want {
		t.Errorf("doc.Blocks[2].Y = %d, want %d", doc.Blocks[2].Y, want)
	}
}

func TestBuild_BarcodeShowTextTrue_CaptionCenteredUnderBarcodeWidth(t *testing.T) {
	// A caption narrower than the barcode's own rendered width must gain
	// leading space padding (centerBarcodeCaption's technique, the same
	// leading-space-padding idea tableRowLines/columnsLines already use for
	// trailing padding) so it paints roughly centered, once the ordinary
	// text-glyph path (starting at x=0) paints it, against the embedded
	// font's fixed glyph advance — not a font-independent geometric
	// centering (see centerBarcodeCaption's own doc comment).
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Barcode{Content: "1", Symbology: "code39", ShowText: true},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 1000}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	caption := doc.Blocks[1].Element.(layout.BarcodeCaption)
	if !strings.HasPrefix(caption.Content, " ") {
		t.Errorf("caption.Content = %q, want leading space padding to center it under a much wider barcode", caption.Content)
	}
}

func TestBuild_BarcodeShowTextTrue_CaptionWiderThanBarcode_NoPaddingPanic(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Barcode{Content: "1", Symbology: "code39", ShowText: true},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 4}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	caption := doc.Blocks[1].Element.(layout.BarcodeCaption)
	if caption.Content == "" {
		t.Errorf("caption.Content is empty, want the barcode's own Content unchanged")
	}
}

func TestBuild_BarcodeShowTextTrue_Deterministic(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Barcode{Content: "HELLO-128", Symbology: "code128", ShowText: true},
	}}
	f := layout.EmbeddedFont{}
	first, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 300}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	second, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 300}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	for i := range first.Blocks {
		a, b := first.Blocks[i], second.Blocks[i]
		if a.Y != b.Y || a.Style != b.Style || a.Element != b.Element {
			t.Errorf("Blocks[%d] differs between calls, want equal", i)
		}
	}
}

// --- layout.GenerateBarcodeBitmap ---

func TestGenerateBarcodeBitmap_DefaultHeight(t *testing.T) {
	bmp, err := layout.GenerateBarcodeBitmap(receipt.Barcode{Content: "HELLO-128", Symbology: "code128"}, 0)
	if err != nil {
		t.Fatalf("GenerateBarcodeBitmap() error = %v, want nil", err)
	}
	if bmp.Height != receipt.DefaultBarcodeHeight {
		t.Errorf("bmp.Height = %d, want %d", bmp.Height, receipt.DefaultBarcodeHeight)
	}
	if bmp.Width <= 0 {
		t.Errorf("bmp.Width = %d, want positive", bmp.Width)
	}
}

func TestGenerateBarcodeBitmap_ExplicitHeight(t *testing.T) {
	bmp, err := layout.GenerateBarcodeBitmap(receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 55}, 0)
	if err != nil {
		t.Fatalf("GenerateBarcodeBitmap() error = %v, want nil", err)
	}
	if bmp.Height != 55 {
		t.Errorf("bmp.Height = %d, want 55", bmp.Height)
	}
}

func TestGenerateBarcodeBitmap_ScalesDownWidthToMaxWidth_HeightUnaffected(t *testing.T) {
	bmp, err := layout.GenerateBarcodeBitmap(receipt.Barcode{Content: "HELLO-128-LONGER-CONTENT", Symbology: "code128", Height: 40}, 30)
	if err != nil {
		t.Fatalf("GenerateBarcodeBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 30 {
		t.Errorf("bmp.Width = %d, want 30 (scaled down to maxWidth)", bmp.Width)
	}
	if bmp.Height != 40 {
		t.Errorf("bmp.Height = %d, want 40 (unaffected by width scaling)", bmp.Height)
	}
}

func TestGenerateBarcodeBitmap_MaxWidthLargerThanNative_NeverUpscales(t *testing.T) {
	small, err := layout.GenerateBarcodeBitmap(receipt.Barcode{Content: "12", Symbology: "code128"}, 0)
	if err != nil {
		t.Fatalf("GenerateBarcodeBitmap() error = %v, want nil", err)
	}
	scaled, err := layout.GenerateBarcodeBitmap(receipt.Barcode{Content: "12", Symbology: "code128"}, 100000)
	if err != nil {
		t.Fatalf("GenerateBarcodeBitmap() error = %v, want nil", err)
	}
	if scaled.Width != small.Width {
		t.Errorf("scaled.Width = %d, want %d (native width, not upscaled)", scaled.Width, small.Width)
	}
}

func TestGenerateBarcodeBitmap_ContainsSetAndUnsetPixels(t *testing.T) {
	bmp, err := layout.GenerateBarcodeBitmap(receipt.Barcode{Content: "HELLO-128", Symbology: "code128"}, 0)
	if err != nil {
		t.Fatalf("GenerateBarcodeBitmap() error = %v, want nil", err)
	}
	var sawSet, sawUnset bool
	rowBytes := (bmp.Width + 7) / 8
	for row := 0; row < bmp.Height; row++ {
		for col := 0; col < bmp.Width; col++ {
			if bmp.Bits[row*rowBytes+col/8]&(0x80>>uint(col%8)) != 0 {
				sawSet = true
			} else {
				sawUnset = true
			}
		}
	}
	if !sawSet || !sawUnset {
		t.Errorf("sawSet = %v, sawUnset = %v, want both true", sawSet, sawUnset)
	}
}

func TestGenerateBarcodeBitmap_DifferentContentProducesDifferentBits(t *testing.T) {
	a, err := layout.GenerateBarcodeBitmap(receipt.Barcode{Content: "AAAAAAA", Symbology: "code128"}, 0)
	if err != nil {
		t.Fatalf("GenerateBarcodeBitmap() error = %v, want nil", err)
	}
	b, err := layout.GenerateBarcodeBitmap(receipt.Barcode{Content: "ZZZZZZZ", Symbology: "code128"}, 0)
	if err != nil {
		t.Fatalf("GenerateBarcodeBitmap() error = %v, want nil", err)
	}
	if string(a.Bits) == string(b.Bits) {
		t.Errorf("different Content produced identical Bits, want different")
	}
}

func TestGenerateBarcodeBitmap_InvalidContent_ReturnsError(t *testing.T) {
	_, err := layout.GenerateBarcodeBitmap(receipt.Barcode{Content: "12345", Symbology: "itf"}, 0)
	if err == nil {
		t.Fatal("GenerateBarcodeBitmap() error = nil, want non-nil")
	}
}

func TestGenerateBarcodeBitmap_Deterministic(t *testing.T) {
	b := receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 30}
	first, err := layout.GenerateBarcodeBitmap(b, 0)
	if err != nil {
		t.Fatalf("GenerateBarcodeBitmap() error = %v, want nil", err)
	}
	second, err := layout.GenerateBarcodeBitmap(b, 0)
	if err != nil {
		t.Fatalf("GenerateBarcodeBitmap() error = %v, want nil", err)
	}
	if first.Width != second.Width || first.Height != second.Height {
		t.Fatalf("dimensions = %dx%d, then %dx%d, want equal", first.Width, first.Height, second.Width, second.Height)
	}
	if string(first.Bits) != string(second.Bits) {
		t.Errorf("Bits differ between calls, want identical")
	}
}

// --- Every supported symbology ---

func TestGenerateBarcodeBitmap_EverySupportedSymbology(t *testing.T) {
	tests := []struct {
		symbology string
		content   string
	}{
		{"code128", "HELLO-128"},
		{"ean13", "400638133393"},
		{"ean8", "7351353"},
		{"upca", "12345678901"},
		{"code39", "CODE-39"},
		{"itf", "12345678"},
	}
	for _, tt := range tests {
		t.Run(tt.symbology, func(t *testing.T) {
			bmp, err := layout.GenerateBarcodeBitmap(receipt.Barcode{Content: tt.content, Symbology: tt.symbology}, 0)
			if err != nil {
				t.Fatalf("GenerateBarcodeBitmap() error = %v, want nil", err)
			}
			if bmp.Width <= 0 || bmp.Height <= 0 {
				t.Errorf("bmp = %dx%d, want positive dimensions", bmp.Width, bmp.Height)
			}
		})
	}
}
