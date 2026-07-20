package layout_test

import (
	"image/color"
	"testing"

	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// --- layout.DecodeAlignedAssetBitmap ---

func TestDecodeAlignedAssetBitmap_MatchesDecodeImageBitmap_WhenWidthAndAlignUnset(t *testing.T) {
	data := solidPNG(t, 4, 2, color.Black)
	want, err := layout.DecodeImageBitmap(data, 100)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	got, err := layout.DecodeAlignedAssetBitmap(layout.AlignedAsset{Data: data}, 100)
	if err != nil {
		t.Fatalf("DecodeAlignedAssetBitmap() error = %v, want nil", err)
	}
	if got.Width != want.Width || got.Height != want.Height {
		t.Fatalf("got = %dx%d, want %dx%d", got.Width, got.Height, want.Width, want.Height)
	}
	if string(got.Bits) != string(want.Bits) {
		t.Errorf("Bits differ from DecodeImageBitmap's output, want identical when Width and Align are both unset")
	}
}

func TestDecodeAlignedAssetBitmap_ExplicitWidthSmallerThanSource_ScalesDown(t *testing.T) {
	data := solidPNG(t, 20, 10, color.Black)
	bmp, err := layout.DecodeAlignedAssetBitmap(layout.AlignedAsset{Data: data, Width: 10}, 0)
	if err != nil {
		t.Fatalf("DecodeAlignedAssetBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 10 || bmp.Height != 5 {
		t.Errorf("bmp = %dx%d, want 10x5", bmp.Width, bmp.Height)
	}
}

func TestDecodeAlignedAssetBitmap_ExplicitWidthLargerThanSource_ScalesUp(t *testing.T) {
	data := solidPNG(t, 4, 2, color.Black)
	bmp, err := layout.DecodeAlignedAssetBitmap(layout.AlignedAsset{Data: data, Width: 40}, 0)
	if err != nil {
		t.Fatalf("DecodeAlignedAssetBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 40 || bmp.Height != 20 {
		t.Errorf("bmp = %dx%d, want 40x20 (upscaled, unlike DecodeImageBitmap's implicit cap)", bmp.Width, bmp.Height)
	}
}

func TestDecodeAlignedAssetBitmap_ExplicitWidthLargerThanMaxWidth_Clamped(t *testing.T) {
	data := solidPNG(t, 4, 2, color.Black)
	bmp, err := layout.DecodeAlignedAssetBitmap(layout.AlignedAsset{Data: data, Width: 1000}, 40)
	if err != nil {
		t.Fatalf("DecodeAlignedAssetBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 40 {
		t.Errorf("bmp.Width = %d, want 40 (clamped to maxWidth)", bmp.Width)
	}
}

func TestDecodeAlignedAssetBitmap_InvalidData_ReturnsError(t *testing.T) {
	if _, err := layout.DecodeAlignedAssetBitmap(layout.AlignedAsset{Data: []byte("not an image")}, 0); err == nil {
		t.Fatal("DecodeAlignedAssetBitmap() error = nil, want non-nil")
	}
}

func TestDecodeAlignedAssetBitmap_Deterministic(t *testing.T) {
	a := layout.AlignedAsset{Data: solidPNG(t, 4, 2, color.Black), Width: 20, Align: "center"}
	first, err := layout.DecodeAlignedAssetBitmap(a, 100)
	if err != nil {
		t.Fatalf("DecodeAlignedAssetBitmap() error = %v, want nil", err)
	}
	second, err := layout.DecodeAlignedAssetBitmap(a, 100)
	if err != nil {
		t.Fatalf("DecodeAlignedAssetBitmap() error = %v, want nil", err)
	}
	if first.Width != second.Width || first.Height != second.Height {
		t.Fatalf("dimensions = %dx%d, then %dx%d, want equal", first.Width, first.Height, second.Width, second.Height)
	}
	if string(first.Bits) != string(second.Bits) {
		t.Errorf("Bits differ between calls, want identical")
	}
}

// --- layout.DecodeAlignedAssetBitmap: alignment padding ---

// firstSetColumn returns the column index of the first set pixel in bmp's
// row 0, or -1 if the row is entirely blank — every fixture in this file
// is a solid-colour image, so row 0 alone locates the painted region's
// left edge.
func firstSetColumn(bmp layout.GlyphBitmap) int {
	for x := 0; x < bmp.Width; x++ {
		if bmp.Bits[x/8]&(0x80>>uint(x%8)) != 0 {
			return x
		}
	}
	return -1
}

func TestDecodeAlignedAssetBitmap_AlignLeft_NoPadding(t *testing.T) {
	data := solidPNG(t, 4, 2, color.Black)
	bmp, err := layout.DecodeAlignedAssetBitmap(layout.AlignedAsset{Data: data, Align: "left"}, 100)
	if err != nil {
		t.Fatalf("DecodeAlignedAssetBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 4 {
		t.Errorf("bmp.Width = %d, want 4 (unpadded)", bmp.Width)
	}
	if firstSetColumn(bmp) != 0 {
		t.Errorf("firstSetColumn = %d, want 0 (no leading blank columns)", firstSetColumn(bmp))
	}
}

func TestDecodeAlignedAssetBitmap_AlignCenter_PadsLeadingBlankColumns(t *testing.T) {
	data := solidPNG(t, 4, 2, color.Black)
	bmp, err := layout.DecodeAlignedAssetBitmap(layout.AlignedAsset{Data: data, Align: "center"}, 100)
	if err != nil {
		t.Fatalf("DecodeAlignedAssetBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 100 {
		t.Errorf("bmp.Width = %d, want 100 (padded out to maxWidth)", bmp.Width)
	}
	want := (100 - 4) / 2
	if got := firstSetColumn(bmp); got != want {
		t.Errorf("firstSetColumn = %d, want %d ((maxWidth - imageWidth) / 2)", got, want)
	}
}

func TestDecodeAlignedAssetBitmap_AlignRight_PadsToRightEdge(t *testing.T) {
	data := solidPNG(t, 4, 2, color.Black)
	bmp, err := layout.DecodeAlignedAssetBitmap(layout.AlignedAsset{Data: data, Align: "right"}, 100)
	if err != nil {
		t.Fatalf("DecodeAlignedAssetBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 100 {
		t.Errorf("bmp.Width = %d, want 100 (padded out to maxWidth)", bmp.Width)
	}
	want := 100 - 4
	if got := firstSetColumn(bmp); got != want {
		t.Errorf("firstSetColumn = %d, want %d (maxWidth - imageWidth, flush against the right edge)", got, want)
	}
}

func TestDecodeAlignedAssetBitmap_AlignRight_PadsMoreThanCenter(t *testing.T) {
	data := solidPNG(t, 4, 2, color.Black)
	center, err := layout.DecodeAlignedAssetBitmap(layout.AlignedAsset{Data: data, Align: "center"}, 100)
	if err != nil {
		t.Fatalf("DecodeAlignedAssetBitmap() error = %v, want nil", err)
	}
	right, err := layout.DecodeAlignedAssetBitmap(layout.AlignedAsset{Data: data, Align: "right"}, 100)
	if err != nil {
		t.Fatalf("DecodeAlignedAssetBitmap() error = %v, want nil", err)
	}
	if firstSetColumn(right) <= firstSetColumn(center) {
		t.Errorf("firstSetColumn(right) = %d, firstSetColumn(center) = %d, want right to start further right", firstSetColumn(right), firstSetColumn(center))
	}
}

func TestDecodeAlignedAssetBitmap_AlignCenter_ZeroMaxWidth_NoPadding(t *testing.T) {
	// maxWidth <= 0 is Build's documented "no printer configured" sentinel
	// (the same convention alignPad and every other alignment path here
	// already respect) — there is no printable width to align against.
	data := solidPNG(t, 4, 2, color.Black)
	bmp, err := layout.DecodeAlignedAssetBitmap(layout.AlignedAsset{Data: data, Align: "center"}, 0)
	if err != nil {
		t.Fatalf("DecodeAlignedAssetBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 4 {
		t.Errorf("bmp.Width = %d, want 4 (unpadded, no maxWidth to center within)", bmp.Width)
	}
}

func TestDecodeAlignedAssetBitmap_AlignCenter_ImageAsWideAsMaxWidth_NoPadding(t *testing.T) {
	data := solidPNG(t, 40, 20, color.Black)
	bmp, err := layout.DecodeAlignedAssetBitmap(layout.AlignedAsset{Data: data, Align: "center"}, 40)
	if err != nil {
		t.Fatalf("DecodeAlignedAssetBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 40 {
		t.Errorf("bmp.Width = %d, want 40 (no room to pad)", bmp.Width)
	}
	if firstSetColumn(bmp) != 0 {
		t.Errorf("firstSetColumn = %d, want 0", firstSetColumn(bmp))
	}
}

func TestDecodeAlignedAssetBitmap_AlignCenter_ResizedAsset_PadsAroundResizedWidth(t *testing.T) {
	// Alignment must be computed against the resolved (resized) bitmap
	// width, not the source image's native width.
	data := solidPNG(t, 20, 10, color.Black)
	bmp, err := layout.DecodeAlignedAssetBitmap(layout.AlignedAsset{Data: data, Width: 10, Align: "center"}, 100)
	if err != nil {
		t.Fatalf("DecodeAlignedAssetBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 100 {
		t.Errorf("bmp.Width = %d, want 100", bmp.Width)
	}
	want := (100 - 10) / 2
	if got := firstSetColumn(bmp); got != want {
		t.Errorf("firstSetColumn = %d, want %d ((maxWidth - resizedWidth) / 2)", got, want)
	}
}
