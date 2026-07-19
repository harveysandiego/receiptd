package escpos_test

import (
	"bytes"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
	"github.com/harveysandiego/receiptd/internal/render/escpos"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// noCut is a Profile that doesn't support cutting at all — the zero value,
// spelled out here so tests read as an explicit choice rather than an
// accidental omission.
var noCut = printer.Profile{}

func TestEncode_EmptyCanvas_ReturnsPermanentError(t *testing.T) {
	c := &canvas.Canvas{}

	_, err := escpos.Encode(c, noCut)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Encode() error = %v, want apperr.KindPermanent", err)
	}
}

func TestEncode_BitsShorterThanDeclaredDimensions_ReturnsPermanentError(t *testing.T) {
	// Width: 8, Height: 2 needs 2 bytes (1 row byte x 2 rows); only 1 given.
	c := &canvas.Canvas{Width: 8, Height: 2, Bits: []byte{0xAA}}

	_, err := escpos.Encode(c, noCut)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Encode() error = %v, want apperr.KindPermanent", err)
	}
}

func TestEncode_BitsLongerThanDeclaredDimensions_ReturnsPermanentError(t *testing.T) {
	// Width: 8, Height: 2 needs 2 bytes; 3 given.
	c := &canvas.Canvas{Width: 8, Height: 2, Bits: []byte{0xAA, 0x55, 0xFF}}

	_, err := escpos.Encode(c, noCut)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Encode() error = %v, want apperr.KindPermanent", err)
	}
}

func TestEncode_InconsistentCanvas_EmitsNoBytes(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 2, Bits: []byte{0xAA}}

	got, err := escpos.Encode(c, noCut)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Encode() error = %v, want apperr.KindPermanent", err)
	}
	if got != nil {
		t.Errorf("Encode() = % x, want nil bytes on error", got)
	}
}

func TestEncode_CorrectlySizedBitmap_StillEncodesSuccessfully(t *testing.T) {
	c := &canvas.Canvas{Width: 10, Height: 3, Bits: make([]byte, 6)} // rowBytes=2, 2*3=6

	_, err := escpos.Encode(c, noCut)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
}

func TestEncode_EmitsInitSequence(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 1, Bits: []byte{0xFF}}

	got, err := escpos.Encode(c, noCut)
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

	got, err := escpos.Encode(c, noCut)
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

	got, err := escpos.Encode(c, noCut)
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

	got, err := escpos.Encode(c, noCut)
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

	first, err := escpos.Encode(c, noCut)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	second, err := escpos.Encode(c, noCut)
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

	if _, err := escpos.Encode(c, noCut); err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	if !bytes.Equal(c.Bits, before) {
		t.Errorf("c.Bits = % x after Encode(), want unchanged % x", c.Bits, before)
	}
	if c.Width != 8 || c.Height != 2 {
		t.Errorf("c.Width/Height changed by Encode()")
	}
}

func TestEncode_ProfileWithoutCutSupport_EmitsNoFeedOrCut(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 1, Bits: []byte{0xFF}}

	got, err := escpos.Encode(c, noCut)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	want := []byte{
		0x1B, 0x40, // ESC @: initialize
		0x1D, 0x76, 0x30, 0x00, // GS v 0 m=0: raster image
		0x01, 0x00, // xL, xH
		0x01, 0x00, // yL, yH
		0xFF, // raster data
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Encode() = % x, want % x (no trailing feed/cut bytes)", got, want)
	}
}

func TestEncode_ProfileWithFullCut_EmitsFeedThenCutAfterRaster(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 1, Bits: []byte{0xFF}}
	profile := printer.Profile{SupportsCut: true, DefaultCut: "full"}

	got, err := escpos.Encode(c, profile)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	want := []byte{0x1B, 0x64, 0x04, 0x1D, 0x56, 0x00} // ESC d 4 feed, then GS V 0 full cut
	if !bytes.HasSuffix(got, want) {
		t.Errorf("Encode() = % x, want suffix % x (feed then full cut)", got, want)
	}
}

func TestEncode_ProfileWithPartialCut_EmitsFeedThenCutAfterRaster(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 1, Bits: []byte{0xFF}}
	profile := printer.Profile{SupportsCut: true, DefaultCut: "partial"}

	got, err := escpos.Encode(c, profile)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	want := []byte{0x1B, 0x64, 0x04, 0x1D, 0x56, 0x01} // ESC d 4 feed, then GS V 1 partial cut
	if !bytes.HasSuffix(got, want) {
		t.Errorf("Encode() = % x, want suffix % x (feed then partial cut)", got, want)
	}
}

func TestEncode_ProfileWithCut_ExactCommandOrdering(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 2, Bits: []byte{0xAA, 0x55}}
	profile := printer.Profile{SupportsCut: true, DefaultCut: "partial"}

	got, err := escpos.Encode(c, profile)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	want := []byte{
		0x1B, 0x40, // ESC @: initialize
		0x1D, 0x76, 0x30, 0x00, // GS v 0 m=0: raster image
		0x01, 0x00, // xL, xH
		0x02, 0x00, // yL, yH
		0xAA, 0x55, // raster data
		0x1B, 0x64, 0x04, // ESC d 4: feed 4 lines
		0x1D, 0x56, 0x01, // GS V 1: partial cut
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Encode() = % x, want % x (init, raster, feed, cut)", got, want)
	}
}

func TestEncode_ProfileWithCut_RasterPayloadUnchanged(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 2, Bits: []byte{0xAA, 0x55}}

	plain, err := escpos.Encode(c, noCut)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	withTermination, err := escpos.Encode(c, printer.Profile{SupportsCut: true, DefaultCut: "partial"})
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}

	if !bytes.HasPrefix(withTermination, plain) {
		t.Errorf("Encode() with cut = % x, want it to start with the unmodified raster output % x", withTermination, plain)
	}
}

func TestEncode_ProfileWithCut_Deterministic(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 2, Bits: []byte{0xAA, 0x55}}
	profile := printer.Profile{SupportsCut: true, DefaultCut: "partial"}

	first, err := escpos.Encode(c, profile)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	second, err := escpos.Encode(c, profile)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("Encode() output differs between calls, want identical")
	}
}

func TestEncode_ProfileSupportsCutWithUnknownDefaultCut_ReturnsPermanentError(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 1, Bits: []byte{0xFF}}
	profile := printer.Profile{SupportsCut: true, DefaultCut: "sideways"}

	_, err := escpos.Encode(c, profile)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Encode() error = %v, want apperr.KindPermanent", err)
	}
}

func TestEncode_ProfileSupportsCutWithEmptyDefaultCut_ReturnsPermanentError(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 1, Bits: []byte{0xFF}}
	profile := printer.Profile{SupportsCut: true}

	_, err := escpos.Encode(c, profile)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Encode() error = %v, want apperr.KindPermanent", err)
	}
}

func TestEncode_HeightWithinMaxImageHeightDots_EmitsSingleRasterCommand(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 2, Bits: []byte{0xAA, 0x55}}
	profile := printer.Profile{MaxImageHeightDots: 5}

	got, err := escpos.Encode(c, profile)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	rasterHeader := []byte{0x1D, 0x76, 0x30, 0x00}
	if n := bytes.Count(got, rasterHeader); n != 1 {
		t.Errorf("raster command count = %d, want 1 (canvas height fits within MaxImageHeightDots)", n)
	}
}

func TestEncode_HeightExceedsMaxImageHeightDots_EmitsMultipleRasterCommands(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 5, Bits: []byte{0x01, 0x02, 0x03, 0x04, 0x05}}
	profile := printer.Profile{MaxImageHeightDots: 2}

	got, err := escpos.Encode(c, profile)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	want := []byte{
		0x1B, 0x40, // ESC @: initialize
		0x1D, 0x76, 0x30, 0x00, 0x01, 0x00, 0x02, 0x00, 0x01, 0x02, // band 1: rows 0-1
		0x1D, 0x76, 0x30, 0x00, 0x01, 0x00, 0x02, 0x00, 0x03, 0x04, // band 2: rows 2-3
		0x1D, 0x76, 0x30, 0x00, 0x01, 0x00, 0x01, 0x00, 0x05, // band 3: row 4 (remainder)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Encode() = % x, want % x (three raster bands of heights 2, 2, 1, in row order)", got, want)
	}
}

func TestEncode_HeightExactMultipleOfMaxImageHeightDots_NoEmptyTrailingBand(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 4, Bits: []byte{0x01, 0x02, 0x03, 0x04}}
	profile := printer.Profile{MaxImageHeightDots: 2}

	got, err := escpos.Encode(c, profile)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	rasterHeader := []byte{0x1D, 0x76, 0x30, 0x00}
	if n := bytes.Count(got, rasterHeader); n != 2 {
		t.Errorf("raster command count = %d, want 2 (4 rows / 2-row chunks divides evenly, no empty trailing band)", n)
	}
}

func TestEncode_ChunkedRasterPayload_ConcatenatesToOriginalCanvasBits(t *testing.T) {
	// A height (7) that doesn't divide evenly by the chunk size (3) exercises
	// a different remainder than the exact-byte test above (5/2), and
	// reassembles every band's data to confirm chunking never drops, reorders,
	// or duplicates a single row of the original Canvas.
	bits := []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}
	c := &canvas.Canvas{Width: 8, Height: 7, Bits: bits}
	profile := printer.Profile{MaxImageHeightDots: 3}

	got, err := escpos.Encode(c, profile)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}

	rasterHeader := []byte{0x1D, 0x76, 0x30, 0x00}
	body := got[2:] // strip ESC @
	var reassembled []byte
	for len(body) > 0 {
		if !bytes.HasPrefix(body, rasterHeader) {
			t.Fatalf("expected raster header % x at % x", rasterHeader, body)
		}
		body = body[len(rasterHeader):]
		rowBytes := int(body[0]) | int(body[1])<<8
		height := int(body[2]) | int(body[3])<<8
		body = body[4:]
		dataLen := rowBytes * height
		reassembled = append(reassembled, body[:dataLen]...)
		body = body[dataLen:]
	}

	if !bytes.Equal(reassembled, bits) {
		t.Errorf("reassembled raster payload = % x, want % x (original canvas bits, unmodified and in row order)", reassembled, bits)
	}
}

func TestEncode_ChunkedImage_FeedAndCutStillEmittedOnceAtEnd(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 5, Bits: []byte{0x01, 0x02, 0x03, 0x04, 0x05}}
	profile := printer.Profile{MaxImageHeightDots: 2, SupportsCut: true, DefaultCut: "partial"}

	got, err := escpos.Encode(c, profile)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	wantSuffix := []byte{0x1B, 0x64, 0x04, 0x1D, 0x56, 0x01} // feed 4 lines, then partial cut
	if !bytes.HasSuffix(got, wantSuffix) {
		t.Errorf("Encode() = % x, want suffix % x (feed+cut once, after every raster band)", got, wantSuffix)
	}
	if n := bytes.Count(got, wantSuffix); n != 1 {
		t.Errorf("feed+cut sequence count = %d, want exactly 1", n)
	}
	rasterHeader := []byte{0x1D, 0x76, 0x30, 0x00}
	if n := bytes.Count(got, rasterHeader); n != 3 {
		t.Errorf("raster command count = %d, want 3", n)
	}
}

func TestEncode_ChunkedImage_Deterministic(t *testing.T) {
	c := &canvas.Canvas{Width: 8, Height: 5, Bits: []byte{0x01, 0x02, 0x03, 0x04, 0x05}}
	profile := printer.Profile{MaxImageHeightDots: 2}

	first, err := escpos.Encode(c, profile)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	second, err := escpos.Encode(c, profile)
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("Encode() output differs between calls, want identical")
	}
}

func TestEncode_Pipeline_TextReceiptProducesRasterOutput(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc, err := layout.Build(receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "A"},
	}}, printer.Profile{}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}

	got, err := escpos.Encode(c, noCut)
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
