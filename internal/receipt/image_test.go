package receipt_test

import (
	"bytes"
	"encoding/json"
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
