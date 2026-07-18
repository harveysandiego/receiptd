package canvas_test

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

func TestEncodePNG_EmptyCanvas_ReturnsPermanentError(t *testing.T) {
	c := &canvas.Canvas{}
	_, err := c.EncodePNG()
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("EncodePNG() error = %v, want apperr.KindPermanent", err)
	}
}

func TestEncodePNG_PreservesDimensions(t *testing.T) {
	c := &canvas.Canvas{Width: 2, Height: 1, Bits: []byte{0x80}}
	b, err := c.EncodePNG()
	if err != nil {
		t.Fatalf("EncodePNG() error = %v, want nil", err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("png.Decode() error = %v", err)
	}
	if got, want := img.Bounds(), image.Rect(0, 0, c.Width, c.Height); got != want {
		t.Errorf("img.Bounds() = %v, want %v", got, want)
	}
}

func TestEncodePNG_PixelValuesPreserved(t *testing.T) {
	c := &canvas.Canvas{Width: 2, Height: 1, Bits: []byte{0x80}}
	b, err := c.EncodePNG()
	if err != nil {
		t.Fatalf("EncodePNG() error = %v, want nil", err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("png.Decode() error = %v", err)
	}
	for y := 0; y < c.Height; y++ {
		for x := 0; x < c.Width; x++ {
			want := pixelSet(c, x, y)
			r, _, _, _ := img.At(x, y).RGBA()
			got := r == 0
			if got != want {
				t.Errorf("pixel(%d,%d) black = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestEncodePNG_Pipeline_PixelValuesMatchPaintedGlyph(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc, err := layout.Build(receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "A"},
	}}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	b, err := c.EncodePNG()
	if err != nil {
		t.Fatalf("EncodePNG() error = %v, want nil", err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("png.Decode() error = %v", err)
	}
	for y := 0; y < c.Height; y++ {
		for x := 0; x < c.Width; x++ {
			want := pixelSet(c, x, y)
			r, _, _, _ := img.At(x, y).RGBA()
			got := r == 0
			if got != want {
				t.Errorf("pixel(%d,%d) black = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestEncodePNG_Deterministic(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 2, Bits: []byte{0xAA, 0x55}}
	first, err := c.EncodePNG()
	if err != nil {
		t.Fatalf("EncodePNG() error = %v, want nil", err)
	}
	second, err := c.EncodePNG()
	if err != nil {
		t.Fatalf("EncodePNG() error = %v, want nil", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("EncodePNG() output differs between calls, want identical")
	}
}

func TestEncodePNG_DoesNotMutateCanvas(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 2, Bits: []byte{0xAA, 0x55}}
	before := append([]byte(nil), c.Bits...)
	if _, err := c.EncodePNG(); err != nil {
		t.Fatalf("EncodePNG() error = %v, want nil", err)
	}
	if !bytes.Equal(c.Bits, before) {
		t.Errorf("c.Bits = %v after EncodePNG(), want unchanged %v", c.Bits, before)
	}
	if c.Width != 8 || c.Height != 2 {
		t.Errorf("c.Width/Height changed by EncodePNG()")
	}
}
