package canvas_test

import (
	"context"
	"image/color"
	"testing"

	"github.com/harveysandiego/receiptd/internal/assets"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// storeWith returns an assets.Store pre-populated with name -> data — the
// same fixture render/layout's own asset_test.go uses, duplicated here
// (rather than exported from either package) since test helpers aren't
// part of either package's public API.
func storeWith(t *testing.T, name string, data []byte) assets.Store {
	t.Helper()
	s := assets.NewMemoryStore()
	if err := s.Put(context.Background(), name, data); err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}
	return s
}

// TestPaint_AssetFromBuild_ProducesSameBitmapAsEquivalentImage proves the
// end-to-end claim docs/ARCHITECTURE.md §3 "Image vs. Asset" makes:
// "canvas.Paint never distinguishes between them, only layout does." A
// receipt.Asset resolved through the real layout.Build pipeline must
// paint an identical bitmap to a receipt.Image carrying the exact same
// bytes directly — proving Paint has no Asset-specific code path at all,
// not just that none is visible from this package's exported API.
func TestPaint_AssetFromBuild_ProducesSameBitmapAsEquivalentImage(t *testing.T) {
	f := layout.EmbeddedFont{}
	data := solidPNG(t, 4, 3, color.Black)
	profile := printer.Profile{WidthDots: 20}

	assetReceipt := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "A"},
		receipt.Asset{Name: "logo.png"},
		receipt.Text{Content: "B"},
	}}
	assetDoc, err := layout.Build(context.Background(), assetReceipt, profile, f, storeWith(t, "logo.png", data))
	if err != nil {
		t.Fatalf("layout.Build() (asset) error = %v, want nil", err)
	}
	assetCanvas, err := canvas.Paint(assetDoc)
	if err != nil {
		t.Fatalf("Paint() (asset) error = %v, want nil", err)
	}

	imageReceipt := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "A"},
		receipt.Image{Data: data},
		receipt.Text{Content: "B"},
	}}
	imageDoc, err := layout.Build(context.Background(), imageReceipt, profile, f, nil)
	if err != nil {
		t.Fatalf("layout.Build() (image) error = %v, want nil", err)
	}
	imageCanvas, err := canvas.Paint(imageDoc)
	if err != nil {
		t.Fatalf("Paint() (image) error = %v, want nil", err)
	}

	if assetCanvas.Width != imageCanvas.Width || assetCanvas.Height != imageCanvas.Height {
		t.Fatalf("asset Canvas = %dx%d, image Canvas = %dx%d, want equal",
			assetCanvas.Width, assetCanvas.Height, imageCanvas.Width, imageCanvas.Height)
	}
	if string(assetCanvas.Bits) != string(imageCanvas.Bits) {
		t.Errorf("asset and image Canvas.Bits differ for equivalent content, want byte-identical")
	}
}
