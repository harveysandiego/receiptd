package layout_test

import (
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
	doc, err := layout.Build(r, printer.Profile{}, layout.EmbeddedFont{})
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
	doc, err := layout.Build(r, printer.Profile{}, layout.EmbeddedFont{})
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
	doc, err := layout.Build(r, printer.Profile{}, layout.EmbeddedFont{})
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
	doc, err := layout.Build(r, printer.Profile{WidthDots: 20}, layout.EmbeddedFont{})
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
	doc, err := layout.Build(r, printer.Profile{}, f)
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
	doc, err := layout.Build(r, printer.Profile{}, layout.EmbeddedFont{})
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

	first, err := layout.Build(r, printer.Profile{WidthDots: 100}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	second, err := layout.Build(r, printer.Profile{WidthDots: 100}, f)
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
	_, err := layout.Build(r, printer.Profile{}, layout.EmbeddedFont{})
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Build() error = %v, want apperr.KindPermanent", err)
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
