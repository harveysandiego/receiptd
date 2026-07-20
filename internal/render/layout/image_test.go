package layout_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/bmp"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// solidPNG returns the encoded bytes of a width x height PNG filled with
// c, the smallest deterministic fixture for exercising real PNG decoding.
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

func TestBuild_OneImage(t *testing.T) {
	data := solidPNG(t, 4, 2, color.Black)
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Image{Data: data},
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
	got, ok := doc.Blocks[0].Element.(receipt.Image)
	if !ok {
		t.Fatalf("doc.Blocks[0].Element = %T, want receipt.Image", doc.Blocks[0].Element)
	}
	if !bytes.Equal(got.Data, data) {
		t.Errorf("doc.Blocks[0].Element.Data changed, want unchanged")
	}
}

func TestBuild_ImageAdvancesYByDecodedHeight_NoPrinterProfile(t *testing.T) {
	// With no printer.Profile (WidthDots 0, Build's documented "no printer
	// configured" sentinel — same convention wrapText already uses), an
	// Image is never scaled, so it advances Y by its native pixel height.
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Image{Data: solidPNG(t, 4, 7, color.Black)},
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2", len(doc.Blocks))
	}
	if doc.Blocks[1].Y != 7 {
		t.Errorf("doc.Blocks[1].Y = %d, want 7 (image's native height)", doc.Blocks[1].Y)
	}
}

func TestBuild_ImageWiderThanPrintableWidth_ScalesDownPreservingAspectRatio(t *testing.T) {
	// A 20x10 image against a 10-dot printable width must scale to 10x5
	// (half width, half height) — the same "content must fit the
	// printable width" principle already established for text wrapping.
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Image{Data: solidPNG(t, 20, 10, color.Black)},
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 10}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2", len(doc.Blocks))
	}
	if doc.Blocks[1].Y != 5 {
		t.Errorf("doc.Blocks[1].Y = %d, want 5 (scaled height)", doc.Blocks[1].Y)
	}
}

func TestBuild_ImageNarrowerThanPrintableWidth_NeverUpscaled(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Image{Data: solidPNG(t, 4, 2, color.Black)},
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 1000}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if doc.Blocks[1].Y != 2 {
		t.Errorf("doc.Blocks[1].Y = %d, want 2 (native height, not upscaled)", doc.Blocks[1].Y)
	}
}

func TestBuild_ImageBetweenTextBlocks_PreservesOrderAndPosition(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Before"},
		receipt.Image{Data: solidPNG(t, 4, 6, color.Black)},
		receipt.Text{Content: "After"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
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

func TestBuild_ImageAfterDivider(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Divider{},
		receipt.Image{Data: solidPNG(t, 4, 6, color.Black)},
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

func TestBuild_InvalidImageData_ReturnsPermanentError(t *testing.T) {
	// layout.Build trusts its caller has already run receipt.Receipt.Validate()
	// (docs/ARCHITECTURE.md §5); a directly hand-built Receipt bypassing
	// that (as this test does, matching TestBuild_UnsupportedElementReturnsPermanentError's
	// existing pattern) still must not panic — Build's own decode failure
	// is reported as apperr.KindPermanent, the same Kind any other
	// renderer-side failure uses.
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Image{Data: []byte("not an image")},
	}}
	_, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Build() error = %v, want apperr.KindPermanent", err)
	}
}

func TestBuild_ImageDeterministic(t *testing.T) {
	// receipt.Image holds a []byte Data field, which makes the concrete
	// type itself uncomparable — Blocks[i] != Blocks[i] (the comparison
	// every other …_Deterministic test in this package uses) panics at
	// runtime once an Element dynamically holds a receipt.Image, so this
	// test compares each field a Build caller actually cares about
	// (Y, Style, and Data by content) instead of the whole Block.
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Milk"},
		receipt.Image{Data: solidPNG(t, 4, 6, color.Black)},
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
		if !okA && a.Element != b.Element {
			t.Errorf("Blocks[%d].Element = %v, then %v, want equal", i, a.Element, b.Element)
		}
	}
}

// --- layout.DecodeImageBitmap ---

// cornerPixelSet reports whether bmp's pixel at (0, 0) is set — every
// test in this file that inspects pixel content uses a solid-colour
// fixture, so the top-left corner alone is enough to tell black from
// white.
func cornerPixelSet(bmp layout.GlyphBitmap) bool {
	return bmp.Bits[0]&0x80 != 0
}

func TestDecodeImageBitmap_BlackPixelIsSet(t *testing.T) {
	bmp, err := layout.DecodeImageBitmap(solidPNG(t, 1, 1, color.Black), 0)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 1 || bmp.Height != 1 {
		t.Fatalf("bmp = %dx%d, want 1x1", bmp.Width, bmp.Height)
	}
	if !cornerPixelSet(bmp) {
		t.Errorf("pixel(0,0) not set, want set (opaque black)")
	}
}

func TestDecodeImageBitmap_WhitePixelIsUnset(t *testing.T) {
	bmp, err := layout.DecodeImageBitmap(solidPNG(t, 1, 1, color.White), 0)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	if cornerPixelSet(bmp) {
		t.Errorf("pixel(0,0) set, want unset (opaque white)")
	}
}

func TestDecodeImageBitmap_TransparentPixelIsUnset(t *testing.T) {
	// A fully transparent pixel composites to the receipt paper's own
	// white, regardless of its nominal colour — docs/ARCHITECTURE.md
	// never defines transparency behaviour explicitly, so this generalises
	// render/layout/embedded_font.go's packMask "alpha decides, composite
	// over white" convention from glyph masks to full-colour images.
	transparentBlack := color.RGBA{R: 0, G: 0, B: 0, A: 0}
	bmp, err := layout.DecodeImageBitmap(solidPNG(t, 1, 1, transparentBlack), 0)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	if cornerPixelSet(bmp) {
		t.Errorf("pixel(0,0) set, want unset (fully transparent composites to white)")
	}
}

func TestDecodeImageBitmap_ScalesDownToMaxWidth_PreservesAspectRatio(t *testing.T) {
	bmp, err := layout.DecodeImageBitmap(solidPNG(t, 4, 2, color.Black), 2)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 2 || bmp.Height != 1 {
		t.Errorf("bmp = %dx%d, want 2x1 (half width, half height)", bmp.Width, bmp.Height)
	}
}

func TestDecodeImageBitmap_MaxWidthLargerThanImage_NeverUpscales(t *testing.T) {
	bmp, err := layout.DecodeImageBitmap(solidPNG(t, 4, 2, color.Black), 1000)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 4 || bmp.Height != 2 {
		t.Errorf("bmp = %dx%d, want 4x2 (native size, not upscaled)", bmp.Width, bmp.Height)
	}
}

func TestDecodeImageBitmap_ZeroMaxWidth_NeverScales(t *testing.T) {
	bmp, err := layout.DecodeImageBitmap(solidPNG(t, 4, 2, color.Black), 0)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 4 || bmp.Height != 2 {
		t.Errorf("bmp = %dx%d, want 4x2 (0 = no printer configured, matches Build's convention)", bmp.Width, bmp.Height)
	}
}

func TestDecodeImageBitmap_InvalidData_ReturnsError(t *testing.T) {
	if _, err := layout.DecodeImageBitmap([]byte("not an image"), 0); err == nil {
		t.Fatal("DecodeImageBitmap() error = nil, want non-nil")
	}
}

func TestDecodeImageBitmap_Deterministic(t *testing.T) {
	data := solidPNG(t, 4, 2, color.Black)
	first, err := layout.DecodeImageBitmap(data, 0)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	second, err := layout.DecodeImageBitmap(data, 0)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	if first.Width != second.Width || first.Height != second.Height {
		t.Fatalf("dimensions = %dx%d, then %dx%d, want equal", first.Width, first.Height, second.Width, second.Height)
	}
	if string(first.Bits) != string(second.Bits) {
		t.Errorf("Bits differ between calls, want identical")
	}
}

// --- additional raster formats ---

func solidJPEG(t *testing.T, width, height int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	// Quality: 100 keeps this solid-color fixture free of JPEG's lossy
	// blocking artefacts, so its decoded pixels threshold identically to
	// the PNG/GIF/BMP fixtures of the same nominal colour.
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatalf("jpeg.Encode() error = %v, want nil", err)
	}
	return buf.Bytes()
}

func solidGIF(t *testing.T, width, height int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatalf("gif.Encode() error = %v, want nil", err)
	}
	return buf.Bytes()
}

func solidBMP(t *testing.T, width, height int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := bmp.Encode(&buf, img); err != nil {
		t.Fatalf("bmp.Encode() error = %v, want nil", err)
	}
	return buf.Bytes()
}

// twoFrameGIF returns an animated GIF whose first frame is solid first
// and second frame is solid second, both widthxheight — used to prove
// DecodeImageBitmap renders only the first frame (see
// TestDecodeImageBitmap_AnimatedGIF_RendersFirstFrameOnly).
func twoFrameGIF(t *testing.T, width, height int, first, second color.Color) []byte {
	t.Helper()
	toPaletted := func(c color.Color) *image.Paletted {
		p := image.NewPaletted(image.Rect(0, 0, width, height), color.Palette{color.Black, color.White, c})
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				p.Set(x, y, c)
			}
		}
		return p
	}
	g := &gif.GIF{
		Image: []*image.Paletted{toPaletted(first), toPaletted(second)},
		Delay: []int{0, 0},
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatalf("gif.EncodeAll() error = %v, want nil", err)
	}
	return buf.Bytes()
}

// realWebP returns a real WebP file checked into testdata/ — see
// receipt's own realWebP helper for why a real file is needed (no pure-Go
// WebP encoder exists).
func realWebP(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/sample.webp")
	if err != nil {
		t.Fatalf("os.ReadFile(testdata/sample.webp) error = %v, want nil", err)
	}
	return data
}

const minimalSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="2" height="2"><rect width="2" height="2"/></svg>`

func TestDecodeImageBitmap_JPEG(t *testing.T) {
	bmp, err := layout.DecodeImageBitmap(solidJPEG(t, 4, 2, color.Black), 0)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 4 || bmp.Height != 2 {
		t.Fatalf("bmp = %dx%d, want 4x2", bmp.Width, bmp.Height)
	}
	if !cornerPixelSet(bmp) {
		t.Errorf("pixel(0,0) not set, want set (opaque black)")
	}
}

func TestDecodeImageBitmap_GIF(t *testing.T) {
	bmp, err := layout.DecodeImageBitmap(solidGIF(t, 4, 2, color.Black), 0)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 4 || bmp.Height != 2 {
		t.Fatalf("bmp = %dx%d, want 4x2", bmp.Width, bmp.Height)
	}
	if !cornerPixelSet(bmp) {
		t.Errorf("pixel(0,0) not set, want set (opaque black)")
	}
}

func TestDecodeImageBitmap_BMP(t *testing.T) {
	bmp, err := layout.DecodeImageBitmap(solidBMP(t, 4, 2, color.Black), 0)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 4 || bmp.Height != 2 {
		t.Fatalf("bmp = %dx%d, want 4x2", bmp.Width, bmp.Height)
	}
	if !cornerPixelSet(bmp) {
		t.Errorf("pixel(0,0) not set, want set (opaque black)")
	}
}

func TestDecodeImageBitmap_WebP_RealFile(t *testing.T) {
	// The real fixture is a photo, not a solid colour, so this only
	// asserts it decodes to its known real dimensions (Google's WebP
	// gallery sample #1 is 550x368) without error — pixel-level content
	// is already covered by the synthetic-format tests above and the
	// format-agnostic rasterizeImage/darkOverWhite logic they share.
	bmp, err := layout.DecodeImageBitmap(realWebP(t), 0)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 550 || bmp.Height != 368 {
		t.Fatalf("bmp = %dx%d, want 550x368", bmp.Width, bmp.Height)
	}
}

func TestDecodeImageBitmap_WebP_ScalesDownToFitLikeAnyOtherFormat(t *testing.T) {
	// Confirms WebP goes through the exact same scaledImageSize path
	// every other format already uses — no format-specific scaling logic.
	bmp, err := layout.DecodeImageBitmap(realWebP(t), 275)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	if bmp.Width != 275 || bmp.Height != 184 {
		t.Fatalf("bmp = %dx%d, want 275x184 (550x368 scaled to fit 275 wide)", bmp.Width, bmp.Height)
	}
}

func TestDecodeImageBitmap_AnimatedGIF_RendersFirstFrameOnly(t *testing.T) {
	data := twoFrameGIF(t, 4, 2, color.Black, color.White)
	bmp, err := layout.DecodeImageBitmap(data, 0)
	if err != nil {
		t.Fatalf("DecodeImageBitmap() error = %v, want nil", err)
	}
	if !cornerPixelSet(bmp) {
		t.Errorf("pixel(0,0) not set, want set (first frame is black, second frame — white — must not be used)")
	}
}

func TestDecodeImageBitmap_SVG_ReturnsError(t *testing.T) {
	if _, err := layout.DecodeImageBitmap([]byte(minimalSVG), 0); err == nil {
		t.Fatal("DecodeImageBitmap() error = nil, want non-nil (SVG is not a supported raster format)")
	}
}

func TestBuild_JPEGImage(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Image{Data: solidJPEG(t, 4, 6, color.Black)},
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if doc.Blocks[1].Y != 6 {
		t.Errorf("doc.Blocks[1].Y = %d, want 6", doc.Blocks[1].Y)
	}
}

func TestBuild_SVGImage_ReturnsPermanentError(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Image{Data: []byte(minimalSVG)},
	}}
	_, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Build() error = %v, want apperr.KindPermanent", err)
	}
}
