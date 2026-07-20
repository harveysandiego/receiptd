package receipt_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"reflect"
	"testing"

	"golang.org/x/image/bmp"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

// onePixelPNG returns the encoded bytes of a single c-colored pixel PNG —
// the smallest input that still exercises real decoding.
func onePixelPNG(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, c)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode() error = %v, want nil", err)
	}
	return buf.Bytes()
}

func onePixelJPEG(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, c)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("jpeg.Encode() error = %v, want nil", err)
	}
	return buf.Bytes()
}

func onePixelGIF(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, c)
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatalf("gif.Encode() error = %v, want nil", err)
	}
	return buf.Bytes()
}

func onePixelBMP(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, c)
	var buf bytes.Buffer
	if err := bmp.Encode(&buf, img); err != nil {
		t.Fatalf("bmp.Encode() error = %v, want nil", err)
	}
	return buf.Bytes()
}

// realWebP returns a real, non-trivial WebP file (Google's own WebP
// gallery sample, https://www.gstatic.com/webp/gallery/1.webp) checked
// into testdata/ — golang.org/x/image/webp has no encoder (WebP encoding
// needs cgo/libwebp, which this project deliberately avoids — see
// go.mod's CGO_ENABLED=0 static-binary goal), so unlike the other
// formats, a real file is the only way to exercise real WebP decoding.
func realWebP(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/sample.webp")
	if err != nil {
		t.Fatalf("os.ReadFile(testdata/sample.webp) error = %v, want nil", err)
	}
	return data
}

// pngChunk encodes one PNG chunk: a 4-byte big-endian length, the 4-byte
// type, data, and a CRC32 of type+data — the structure hugePNGHeader
// assembles by hand.
func pngChunk(typ string, data []byte) []byte {
	var buf bytes.Buffer
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(data)))
	buf.Write(length[:])
	buf.WriteString(typ)
	buf.Write(data)
	crc := crc32.NewIEEE()
	crc.Write([]byte(typ))
	crc.Write(data)
	var sum [4]byte
	binary.BigEndian.PutUint32(sum[:], crc.Sum32())
	buf.Write(sum[:])
	return buf.Bytes()
}

// hugePNGHeader returns a structurally valid PNG — signature, IHDR
// declaring width x height, IEND — with no IDAT (pixel data) chunk at
// all. image.DecodeConfig only ever needs IHDR to report Config.Width/
// Height, so this decodes successfully despite containing zero actual
// pixel bytes: exactly the shape a real "decompression bomb" image
// takes, a tiny file whose header claims dimensions wildly out of
// proportion to its size. Used to prove MaxImagePixels is checked from
// the declared header, not from how much pixel data was actually
// decoded.
func hugePNGHeader(t *testing.T, width, height uint32) []byte {
	t.Helper()
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = 8 // bit depth
	ihdr[9] = 6 // color type: truecolor with alpha

	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	buf.Write(pngChunk("IHDR", ihdr))
	buf.Write(pngChunk("IEND", nil))
	return buf.Bytes()
}

// minimalSVG is well-formed SVG/XML — not a raster format, and this
// project deliberately does not implement an SVG rasterizer (see
// docs/adr/0002-raster-rendering.md and this slice's own scope), so it
// must always be reported as unsupported.
const minimalSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="2" height="2"><rect width="2" height="2"/></svg>`

func TestImageValidate(t *testing.T) {
	tests := []struct {
		name    string
		image   receipt.Image
		wantErr bool
	}{
		{"zero value, no data", receipt.Image{}, true},
		{"valid PNG", receipt.Image{Data: onePixelPNG(t, color.Black)}, false},
		{"valid JPEG", receipt.Image{Data: onePixelJPEG(t, color.Black)}, false},
		{"valid GIF", receipt.Image{Data: onePixelGIF(t, color.Black)}, false},
		{"valid BMP", receipt.Image{Data: onePixelBMP(t, color.Black)}, false},
		{"valid WebP", receipt.Image{Data: realWebP(t)}, false},
		{"SVG, unsupported format", receipt.Image{Data: []byte(minimalSVG)}, true},
		{"garbage bytes", receipt.Image{Data: []byte("not an image")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.image.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// This is a regression test for a real vulnerability: Validate used to
// call image.Decode (full pixel decode) before this check existed, so a
// tiny file declaring huge dimensions ("decompression bomb") forced a
// multi-gigabyte allocation per /preview or /print request. See
// MaxImagePixels's doc comment.
func TestImageValidate_ExceedsMaxImagePixels_ReturnsError(t *testing.T) {
	img := receipt.Image{Data: hugePNGHeader(t, 40000, 40000)} // 1.6B pixels
	if err := img.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error for a declared pixel count over MaxImagePixels")
	}
}

func TestImageValidate_WithinMaxImagePixels_NoError(t *testing.T) {
	img := receipt.Image{Data: hugePNGHeader(t, 4000, 4000)} // 16M pixels, under the 20M cap
	if err := img.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil for a declared pixel count under MaxImagePixels", err)
	}
}

// Guards the pixel-count check's arithmetic itself: width*height here
// (3.6 billion) overflows a 32-bit int, which is what plain `int`
// multiplication would silently wrap to on a 32-bit build (e.g. 32-bit
// ARM) — a wrapped negative or small product could pass the ">
// MaxImagePixels" comparison outright, defeating the check on exactly the
// platform-and-input combination it exists to guard. This can't observe
// an actual wraparound on this repo's 64-bit test/CI hosts (Go's `int` is
// 64-bit there), but it does pin down that the check must keep rejecting
// a product this large regardless of how it's computed.
func TestImageValidate_DimensionsOverflow32BitInt_StillRejected(t *testing.T) {
	img := receipt.Image{Data: hugePNGHeader(t, 60000, 60000)} // 3.6B pixels
	if err := img.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error for a pixel count that overflows a 32-bit int")
	}
}

func TestIsSupportedImageFormat(t *testing.T) {
	tests := []struct {
		format string
		want   bool
	}{
		{"png", true},
		{"jpeg", true},
		{"gif", true},
		{"bmp", true},
		{"webp", true},
		{"svg", false},
		{"tiff", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := receipt.IsSupportedImageFormat(tt.format); got != tt.want {
			t.Errorf("IsSupportedImageFormat(%q) = %v, want %v", tt.format, got, tt.want)
		}
	}
}

func TestImage_JSONRoundTrip(t *testing.T) {
	original := receipt.Image{Data: onePixelPNG(t, color.Black)}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("json.Unmarshal() into map error = %v, want nil", err)
	}
	if wire["type"] != "image" {
		t.Errorf(`wire["type"] = %v, want "image"`, wire["type"])
	}

	var decoded receipt.Image
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}

func TestImage_JSONRoundTrip_DataIsBase64Encoded(t *testing.T) {
	// docs/ARCHITECTURE.md §3: Image's "data" field is inline base64 — the
	// standard encoding/json behaviour for a []byte field, not something
	// this package encodes itself.
	original := receipt.Image{Data: onePixelPNG(t, color.Black)}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("json.Unmarshal() into map error = %v, want nil", err)
	}
	if _, ok := wire["data"].(string); !ok {
		t.Errorf(`wire["data"] = %T, want a base64 string`, wire["data"])
	}
}

func TestReceipt_WithImage_JSONRoundTrip(t *testing.T) {
	original := receipt.Receipt{
		Elements: []receipt.Element{
			receipt.Text{Content: "Before"},
			receipt.Image{Data: onePixelPNG(t, color.Black)},
			receipt.Text{Content: "After"},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}
	var decoded receipt.Receipt
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}
