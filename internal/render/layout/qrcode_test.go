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

func TestBuild_OneQRCode(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.QRCode{Content: "https://example.com"},
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
	got, ok := doc.Blocks[0].Element.(receipt.QRCode)
	if !ok {
		t.Fatalf("doc.Blocks[0].Element = %T, want receipt.QRCode", doc.Blocks[0].Element)
	}
	if got.Content != "https://example.com" {
		t.Errorf("doc.Blocks[0].Element.Content changed, want unchanged")
	}
}

func TestBuild_QRCodeAdvancesYByDefaultSize_NoPrinterProfile(t *testing.T) {
	// With no printer.Profile (WidthDots 0, Build's documented "no printer
	// configured" sentinel), a QRCode with no explicit Size advances Y by
	// receipt.DefaultQRCodeSize, the same way an Image advances Y by its
	// native height.
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.QRCode{Content: "https://example.com"},
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2", len(doc.Blocks))
	}
	if doc.Blocks[1].Y != receipt.DefaultQRCodeSize {
		t.Errorf("doc.Blocks[1].Y = %d, want %d (default QR size)", doc.Blocks[1].Y, receipt.DefaultQRCodeSize)
	}
}

func TestBuild_QRCodeWiderThanPrintableWidth_ScalesDown(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.QRCode{Content: "https://example.com", Size: 400},
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 100}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if doc.Blocks[1].Y != 100 {
		t.Errorf("doc.Blocks[1].Y = %d, want 100 (scaled down to printable width)", doc.Blocks[1].Y)
	}
}

func TestBuild_QRCodeNarrowerThanPrintableWidth_NeverUpscaled(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.QRCode{Content: "https://example.com", Size: 50},
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 1000}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if doc.Blocks[1].Y != 50 {
		t.Errorf("doc.Blocks[1].Y = %d, want 50 (native size, not upscaled)", doc.Blocks[1].Y)
	}
}

func TestBuild_QRCodeBetweenTextBlocks_PreservesOrderAndPosition(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Before"},
		receipt.QRCode{Content: "https://example.com", Size: 60},
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
	if _, ok := doc.Blocks[1].Element.(receipt.QRCode); !ok {
		t.Fatalf("doc.Blocks[1].Element = %T, want receipt.QRCode", doc.Blocks[1].Element)
	}
	if wantY := f.LineHeight() + 60; doc.Blocks[2].Y != wantY {
		t.Errorf("doc.Blocks[2].Y = %d, want %d", doc.Blocks[2].Y, wantY)
	}
}

func TestBuild_QRCodeAfterDivider(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Divider{},
		receipt.QRCode{Content: "https://example.com", Size: 40},
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

func TestBuild_QRCodeDeterministic(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Milk"},
		receipt.QRCode{Content: "https://example.com", Size: 60},
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

// --- layout.GenerateQRCodeBitmap ---

func TestGenerateQRCodeBitmap_DefaultSize(t *testing.T) {
	bmp, err := layout.GenerateQRCodeBitmap(receipt.QRCode{Content: "https://example.com"}, 0)
	if err != nil {
		t.Fatalf("GenerateQRCodeBitmap() error = %v, want nil", err)
	}
	if bmp.Width != receipt.DefaultQRCodeSize || bmp.Height != receipt.DefaultQRCodeSize {
		t.Errorf("bmp = %dx%d, want %dx%d", bmp.Width, bmp.Height, receipt.DefaultQRCodeSize, receipt.DefaultQRCodeSize)
	}
}

func TestGenerateQRCodeBitmap_ExplicitSize(t *testing.T) {
	bmp, err := layout.GenerateQRCodeBitmap(receipt.QRCode{Content: "https://example.com", Size: 80}, 0)
	if err != nil {
		t.Fatalf("GenerateQRCodeBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 80 || bmp.Height != 80 {
		t.Errorf("bmp = %dx%d, want 80x80", bmp.Width, bmp.Height)
	}
}

func TestGenerateQRCodeBitmap_ScalesDownToMaxWidth(t *testing.T) {
	bmp, err := layout.GenerateQRCodeBitmap(receipt.QRCode{Content: "https://example.com", Size: 200}, 50)
	if err != nil {
		t.Fatalf("GenerateQRCodeBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 50 || bmp.Height != 50 {
		t.Errorf("bmp = %dx%d, want 50x50 (scaled down to maxWidth, still square)", bmp.Width, bmp.Height)
	}
}

func TestGenerateQRCodeBitmap_MaxWidthLargerThanSize_NeverUpscales(t *testing.T) {
	bmp, err := layout.GenerateQRCodeBitmap(receipt.QRCode{Content: "https://example.com", Size: 50}, 1000)
	if err != nil {
		t.Fatalf("GenerateQRCodeBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 50 || bmp.Height != 50 {
		t.Errorf("bmp = %dx%d, want 50x50 (native size, not upscaled)", bmp.Width, bmp.Height)
	}
}

func TestGenerateQRCodeBitmap_ContainsSetAndUnsetPixels(t *testing.T) {
	// A real QR code is never solid: at minimum its finder patterns paint
	// both black and white modules, so this catches a degenerate
	// all-white or all-black bitmap (e.g. from a broken threshold) without
	// asserting on any specific module layout.
	bmp, err := layout.GenerateQRCodeBitmap(receipt.QRCode{Content: "https://example.com"}, 0)
	if err != nil {
		t.Fatalf("GenerateQRCodeBitmap() error = %v, want nil", err)
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

func TestGenerateQRCodeBitmap_DifferentContentProducesDifferentBits(t *testing.T) {
	a, err := layout.GenerateQRCodeBitmap(receipt.QRCode{Content: "https://example.com/a"}, 0)
	if err != nil {
		t.Fatalf("GenerateQRCodeBitmap() error = %v, want nil", err)
	}
	b, err := layout.GenerateQRCodeBitmap(receipt.QRCode{Content: "https://example.com/b"}, 0)
	if err != nil {
		t.Fatalf("GenerateQRCodeBitmap() error = %v, want nil", err)
	}
	if string(a.Bits) == string(b.Bits) {
		t.Errorf("different Content produced identical Bits, want different")
	}
}

func TestGenerateQRCodeBitmap_LargePayload(t *testing.T) {
	bmp, err := layout.GenerateQRCodeBitmap(receipt.QRCode{Content: strings.Repeat("a", 500)}, 0)
	if err != nil {
		t.Fatalf("GenerateQRCodeBitmap() error = %v, want nil", err)
	}
	if bmp.Width <= 0 || bmp.Height <= 0 {
		t.Errorf("bmp = %dx%d, want positive dimensions", bmp.Width, bmp.Height)
	}
}

func TestGenerateQRCodeBitmap_ContentTooLargeForCapacity_ReturnsError(t *testing.T) {
	_, err := layout.GenerateQRCodeBitmap(receipt.QRCode{Content: strings.Repeat("A", 10000)}, 0)
	if err == nil {
		t.Fatal("GenerateQRCodeBitmap() error = nil, want non-nil")
	}
}

func TestGenerateQRCodeBitmap_Deterministic(t *testing.T) {
	qr := receipt.QRCode{Content: "https://example.com", Size: 60}
	first, err := layout.GenerateQRCodeBitmap(qr, 0)
	if err != nil {
		t.Fatalf("GenerateQRCodeBitmap() error = %v, want nil", err)
	}
	second, err := layout.GenerateQRCodeBitmap(qr, 0)
	if err != nil {
		t.Fatalf("GenerateQRCodeBitmap() error = %v, want nil", err)
	}
	if first.Width != second.Width || first.Height != second.Height {
		t.Fatalf("dimensions = %dx%d, then %dx%d, want equal", first.Width, first.Height, second.Width, second.Height)
	}
	if string(first.Bits) != string(second.Bits) {
		t.Errorf("Bits differ between calls, want identical")
	}
}

func TestBuild_QRCodeContentTooLargeForCapacity_ReturnsPermanentError(t *testing.T) {
	// layout.Build trusts its caller has already run receipt.Receipt.Validate()
	// (docs/ARCHITECTURE.md §5); a directly hand-built Receipt bypassing that
	// still must not panic — Build's own encode failure is reported as
	// apperr.KindPermanent, the same Kind receipt.Image's decode failure uses.
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.QRCode{Content: strings.Repeat("A", 10000)},
	}}
	_, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Build() error = %v, want apperr.KindPermanent", err)
	}
}
