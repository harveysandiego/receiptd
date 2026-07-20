package layout_test

import (
	"bytes"
	"context"
	"image/color"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/assets"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// storeWith returns an assets.Store pre-populated with "logo.png" -> data,
// the fixture every test in this file uses to exercise Build's Asset
// resolution without touching a real filesystem.
func storeWith(t *testing.T, data []byte) assets.Store {
	t.Helper()
	s := assets.NewMemoryStore()
	if err := s.Put(context.Background(), "logo.png", data); err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}
	return s
}

func TestBuild_Asset_ResolvesToImageBlock(t *testing.T) {
	// docs/ARCHITECTURE.md §3 "Image vs. Asset": by the time Build returns,
	// an Asset and an Image are the same kind of already-decoded pixel
	// content on a Block — canvas.Paint never distinguishes them, only
	// layout does. Build itself proves that by producing a receipt.Image
	// Block, the exact type an ordinary Image element already produces.
	data := solidPNG(t, 4, 2, color.Black)
	store := storeWith(t, data)
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Asset{Name: "logo.png"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, store)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(doc.Blocks) = %d, want 1", len(doc.Blocks))
	}
	got, ok := doc.Blocks[0].Element.(receipt.Image)
	if !ok {
		t.Fatalf("doc.Blocks[0].Element = %T, want receipt.Image", doc.Blocks[0].Element)
	}
	if !bytes.Equal(got.Data, data) {
		t.Errorf("doc.Blocks[0].Element.Data changed, want the resolved asset's bytes unchanged")
	}
}

func TestBuild_Asset_AdvancesYByDecodedHeight(t *testing.T) {
	store := storeWith(t, solidPNG(t, 4, 7, color.Black))
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Asset{Name: "logo.png"},
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, store)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if doc.Blocks[1].Y != 7 {
		t.Errorf("doc.Blocks[1].Y = %d, want 7 (asset's native height)", doc.Blocks[1].Y)
	}
}

func TestBuild_Asset_WiderThanPrintableWidth_ScalesDownPreservingAspectRatio(t *testing.T) {
	store := storeWith(t, solidPNG(t, 20, 10, color.Black))
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Asset{Name: "logo.png"},
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 10}, layout.EmbeddedFont{}, store)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if doc.Blocks[1].Y != 5 {
		t.Errorf("doc.Blocks[1].Y = %d, want 5 (scaled height, same rule as receipt.Image)", doc.Blocks[1].Y)
	}
}

// This is a regression test for a real vulnerability, and specifically
// for the Asset path rather than Image's: an Asset's resolved bytes reach
// decodeImage/imageHeight via assets.Store.Get, never through
// receipt.Image.Validate — so the MaxImagePixels check inside
// render/layout (checkImageDimensions) is the *only* place a
// decompression-bomb asset already stored via assets.Store gets rejected,
// unlike an inline receipt.Image which Validate also screens on the way
// in. See receipt.MaxImagePixels's doc comment.
func TestBuild_Asset_ExceedsMaxImagePixels_ReturnsPermanentError(t *testing.T) {
	store := storeWith(t, hugePNGHeader(t, 40000, 40000))
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Asset{Name: "logo.png"},
	}}
	_, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, store)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Build() error = %v, want apperr.KindPermanent", err)
	}
}

func TestBuild_Asset_MissingAsset_ReturnsNotFound(t *testing.T) {
	store := assets.NewMemoryStore()
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Asset{Name: "does-not-exist.png"},
	}}
	_, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, store)
	if !apperr.Is(err, apperr.KindNotFound) {
		t.Fatalf("Build() error = %v, want apperr.KindNotFound", err)
	}
}

func TestBuild_Asset_NilStore_ReturnsPermanentErrorNotPanic(t *testing.T) {
	// A nil assets.Store is a legitimate value for any Receipt that
	// carries no receipt.Asset (see TestBuild_NilStore_NoAssetElement_Succeeds)
	// — this only exercises the case where an Asset actually needs
	// resolving and no Store was supplied to resolve it against. That's a
	// caller/wiring mistake, not something a retry fixes, so it gets the
	// same apperr.KindPermanent the unsupported-element-type and
	// invalid-image-data cases already use — and, above all, must not
	// panic via a nil interface method call.
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Asset{Name: "logo.png"},
	}}
	_, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Build() error = %v, want apperr.KindPermanent", err)
	}
}

func TestBuild_NilStore_NoAssetElement_Succeeds(t *testing.T) {
	// nil remains a perfectly valid assets.Store for any Receipt that
	// never actually needs one resolved — the common case for every
	// caller that knows its Receipt carries no receipt.Asset.
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(doc.Blocks) = %d, want 1", len(doc.Blocks))
	}
}

func TestBuild_Asset_InvalidImageData_ReturnsPermanentError(t *testing.T) {
	store := storeWith(t, []byte("not an image"))
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Asset{Name: "logo.png"},
	}}
	_, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, store)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Build() error = %v, want apperr.KindPermanent", err)
	}
}

func TestBuild_AssetBetweenTextBlocks_PreservesOrderAndPosition(t *testing.T) {
	f := layout.EmbeddedFont{}
	store := storeWith(t, solidPNG(t, 4, 6, color.Black))
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Before"},
		receipt.Asset{Name: "logo.png"},
		receipt.Text{Content: "After"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, store)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("len(doc.Blocks) = %d, want 3", len(doc.Blocks))
	}
	if _, ok := doc.Blocks[0].Element.(receipt.Text); !ok {
		t.Fatalf("doc.Blocks[0].Element = %T, want receipt.Text", doc.Blocks[0].Element)
	}
	if wantY := f.LineHeight(); doc.Blocks[1].Y != wantY {
		t.Errorf("doc.Blocks[1].Y = %d, want %d", doc.Blocks[1].Y, wantY)
	}
	if _, ok := doc.Blocks[1].Element.(receipt.Image); !ok {
		t.Fatalf("doc.Blocks[1].Element = %T, want receipt.Image", doc.Blocks[1].Element)
	}
	if wantY := f.LineHeight() + 6; doc.Blocks[2].Y != wantY {
		t.Errorf("doc.Blocks[2].Y = %d, want %d", doc.Blocks[2].Y, wantY)
	}
}

func TestBuild_AssetAfterDivider(t *testing.T) {
	store := storeWith(t, solidPNG(t, 4, 6, color.Black))
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Divider{},
		receipt.Asset{Name: "logo.png"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, store)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if doc.Blocks[1].Y != layout.DividerThickness {
		t.Errorf("doc.Blocks[1].Y = %d, want %d", doc.Blocks[1].Y, layout.DividerThickness)
	}
}

func TestBuild_AssetThenQRCode(t *testing.T) {
	f := layout.EmbeddedFont{}
	store := storeWith(t, solidPNG(t, 4, 6, color.Black))
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Asset{Name: "logo.png"},
		receipt.QRCode{Content: "https://example.com"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, store)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2", len(doc.Blocks))
	}
	if _, ok := doc.Blocks[1].Element.(receipt.QRCode); !ok {
		t.Fatalf("doc.Blocks[1].Element = %T, want receipt.QRCode", doc.Blocks[1].Element)
	}
}

func TestBuild_AssetThenBarcode(t *testing.T) {
	f := layout.EmbeddedFont{}
	store := storeWith(t, solidPNG(t, 4, 6, color.Black))
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Asset{Name: "logo.png"},
		receipt.Barcode{Content: "12345678", Symbology: "code128"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, store)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2", len(doc.Blocks))
	}
	if _, ok := doc.Blocks[1].Element.(receipt.Barcode); !ok {
		t.Fatalf("doc.Blocks[1].Element = %T, want receipt.Barcode", doc.Blocks[1].Element)
	}
}

func TestBuild_AssetDeterministic(t *testing.T) {
	store := storeWith(t, solidPNG(t, 4, 6, color.Black))
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Milk"},
		receipt.Asset{Name: "logo.png"},
		receipt.Text{Content: "Eggs"},
	}}
	f := layout.EmbeddedFont{}

	first, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 100}, f, store)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	second, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 100}, f, store)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(first.Blocks) != len(second.Blocks) {
		t.Fatalf("len(first.Blocks) = %d, len(second.Blocks) = %d, want equal", len(first.Blocks), len(second.Blocks))
	}
	for i := range first.Blocks {
		a, b := first.Blocks[i], second.Blocks[i]
		if a.Y != b.Y || a.Style != b.Style {
			t.Errorf("Blocks[%d] = {Y:%d, Style:%+v}, then {Y:%d, Style:%+v}, want equal", i, a.Y, a.Style, b.Y, b.Style)
		}
		imgA, okA := a.Element.(receipt.Image)
		imgB, okB := b.Element.(receipt.Image)
		if okA != okB {
			t.Fatalf("Blocks[%d].Element types differ between calls", i)
		}
		if okA && !bytes.Equal(imgA.Data, imgB.Data) {
			t.Errorf("Blocks[%d].Element.Data differs between calls, want equal", i)
		}
	}
}
