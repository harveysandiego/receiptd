package escpos_test

import (
	"bytes"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
	"github.com/harveysandiego/receiptd/internal/render/escpos"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

func TestEncode_EmptyCanvas_ReturnsPermanentError(t *testing.T) {
	c := &canvas.Canvas{}

	_, err := escpos.Encode(c)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Encode() error = %v, want apperr.KindPermanent", err)
	}
}

func TestEncode_BitsShorterThanDeclaredDimensions_ReturnsPermanentError(t *testing.T) {
	// Width: 8, Height: 2 needs 2 bytes (1 row byte x 2 rows); only 1 given.
	c := &canvas.Canvas{Width: 8, Height: 2, Bits: []byte{0xAA}}

	_, err := escpos.Encode(c)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Encode() error = %v, want apperr.KindPermanent", err)
	}
}

func TestEncode_BitsLongerThanDeclaredDimensions_ReturnsPermanentError(t *testing.T) {
	// Width: 8, Height: 2 needs 2 bytes; 3 given.
	c := &canvas.Canvas{Width: 8, Height: 2, Bits: []byte{0xAA, 0x55, 0xFF}}

	_, err := escpos.Encode(c)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Encode() error = %v, want apperr.KindPermanent", err)
	}
}

func TestEncode_InconsistentCanvas_EmitsNoBytes(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 2, Bits: []byte{0xAA}}

	got, err := escpos.Encode(c)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Encode() error = %v, want apperr.KindPermanent", err)
	}
	if got != nil {
		t.Errorf("Encode() = % x, want nil bytes on error", got)
	}
}

func TestEncode_CorrectlySizedBitmap_StillEncodesSuccessfully(t *testing.T) {
	c := &canvas.Canvas{Width: 10, Height: 3, Bits: make([]byte, 6)} // rowBytes=2, 2*3=6

	_, err := escpos.Encode(c)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
}

func TestEncode_EmitsInitSequence(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 1, Bits: []byte{0xFF}}

	got, err := escpos.Encode(c)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	want := []byte{0x1B, 0x40} // ESC @
	if !bytes.HasPrefix(got, want) {
		t.Errorf("Encode() = % x, want prefix % x (ESC @ init sequence)", got, want)
	}
}

func TestEncode_SimpleCanvas_ExactByteSequence(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 2, Bits: []byte{0xAA, 0x55}}

	got, err := escpos.Encode(c)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	want := []byte{
		0x1B, 0x40, // ESC @: initialize
		0x1D, 0x76, 0x30, 0x00, // GS v 0 m=0: raster image, normal mode
		0x01, 0x00, // xL, xH: 1 byte per row
		0x02, 0x00, // yL, yH: 2 rows
		0xAA, 0x55, // raster data, verbatim from Canvas.Bits
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Encode() = % x, want % x", got, want)
	}
}

func TestEncode_WidthNotByteAligned_RoundsWidthBytesUp(t *testing.T) {
	// 10 dots wide needs ceil(10/8) = 2 bytes per row, same packing
	// convention Canvas.Bits already uses.
	c := &canvas.Canvas{Width: 10, Height: 1, Bits: []byte{0xFF, 0xC0}}

	got, err := escpos.Encode(c)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	xL, xH := got[6], got[7]
	if xL != 2 || xH != 0 {
		t.Errorf("width bytes = (%d, %d), want (2, 0)", xL, xH)
	}
}

func TestEncode_RasterDataMatchesCanvasBits(t *testing.T) {
	bits := []byte{0x12, 0x34, 0x56, 0x78}
	c := &canvas.Canvas{Width: 8, Height: 4, Bits: bits}

	got, err := escpos.Encode(c)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	data := got[len(got)-len(bits):]
	if !bytes.Equal(data, bits) {
		t.Errorf("raster data = % x, want % x", data, bits)
	}
}

func TestEncode_Deterministic(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 2, Bits: []byte{0xAA, 0x55}}

	first, err := escpos.Encode(c)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	second, err := escpos.Encode(c)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("Encode() output differs between calls, want identical")
	}
}

func TestEncode_DoesNotMutateCanvas(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 2, Bits: []byte{0xAA, 0x55}}
	before := append([]byte(nil), c.Bits...)

	if _, err := escpos.Encode(c); err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	if !bytes.Equal(c.Bits, before) {
		t.Errorf("c.Bits = % x after Encode(), want unchanged % x", c.Bits, before)
	}
	if c.Width != 8 || c.Height != 2 {
		t.Errorf("c.Width/Height changed by Encode()")
	}
}

func TestEncode_Pipeline_TextReceiptProducesRasterOutput(t *testing.T) {
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

	got, err := escpos.Encode(c)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	wantLen := 2 + 4 + 4 + len(c.Bits) // init + raster header + width/height + data
	if len(got) != wantLen {
		t.Errorf("len(Encode()) = %d, want %d", len(got), wantLen)
	}
	if !bytes.HasPrefix(got, []byte{0x1B, 0x40}) {
		t.Error("Encode() missing init sequence prefix")
	}
	if !bytes.Equal(got[len(got)-len(c.Bits):], c.Bits) {
		t.Error("Encode() raster data doesn't match painted canvas bits — encoder must not reinterpret text itself")
	}
}
