package receipt

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/code39"
	"github.com/boombuler/barcode/ean"
	"github.com/boombuler/barcode/twooffive"
)

// Barcode renders as a generated 1D barcode bitmap encoding Content — the
// same "generation" pattern as QRCode: render/layout.GenerateBarcodeBitmap
// turns it into pixels at render time, which flow through the raster
// pipeline Image and QRCode also use.
type Barcode struct {
	Content string `json:"content"`

	// Symbology selects which barcode standard Content is encoded as: one
	// of BarcodeSymbologies. See docs/adr/0009-barcode-symbologies.md for
	// why this is a fixed, closed set rather than every symbology
	// github.com/boombuler/barcode itself supports.
	Symbology string `json:"symbology"`

	// Height is the barcode's target height in dots. Zero or omitted means
	// DefaultBarcodeHeight. Unlike QRCode.Size it does not affect width,
	// which is driven entirely by how many modules Content encodes into.
	Height int `json:"height,omitempty"`

	// ShowText selects whether Content is printed as human-readable text
	// beneath the bars, as an extra render/layout.BarcodeCaption Block
	// roughly centered under the barcode's rendered width.
	ShowText bool `json:"show_text,omitempty"`
}

// DefaultBarcodeHeight is the height, in dots, a Barcode renders at when
// Height is omitted or non-positive.
const DefaultBarcodeHeight = 80

// maxBarcodeHeightDots bounds Height: render/layout.GenerateBarcodeBitmap
// sizes its bitmap directly off Height, so an unbounded Height would let
// one Barcode force an arbitrarily large allocation. 10,000 dots is about
// 1.25m at 203 DPI, far beyond any real barcode's use, but finite.
const maxBarcodeHeightDots = 10000

// BarcodeSymbologies is every value Barcode.Symbology accepts: the
// complete, stable v1 set frozen by docs/adr/0009-barcode-symbologies.md.
var BarcodeSymbologies = []string{"code128", "ean13", "ean8", "upca", "code39", "itf"}

var barcodeSymbologySet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(BarcodeSymbologies))
	for _, s := range BarcodeSymbologies {
		m[s] = struct{}{}
	}
	return m
}()

// IsSupportedBarcodeSymbology reports whether symbology is one
// Barcode.Symbology may use.
func IsSupportedBarcodeSymbology(symbology string) bool {
	_, ok := barcodeSymbologySet[symbology]
	return ok
}

// Encode encodes b.Content using the github.com/boombuler/barcode
// encoder for b.Symbology, erroring if Content is not valid data for that
// symbology. Exported so render/layout.GenerateBarcodeBitmap performs the
// exact same encode Validate already checked.
//
// The explicit length checks for ean13/ean8/upca pin down a symbology the
// underlying ean encoder otherwise infers purely from length; upca has no
// dedicated encoder, so it is encoded as EAN-13 with Content prefixed "0".
func (b Barcode) Encode() (barcode.Barcode, error) {
	switch b.Symbology {
	case "code128":
		return code128.Encode(b.Content)
	case "ean13":
		if len(b.Content) != 12 && len(b.Content) != 13 {
			return nil, fmt.Errorf("ean13 requires 12 or 13 digits, got %d", len(b.Content))
		}
		return ean.Encode(b.Content)
	case "ean8":
		if len(b.Content) != 7 && len(b.Content) != 8 {
			return nil, fmt.Errorf("ean8 requires 7 or 8 digits, got %d", len(b.Content))
		}
		return ean.Encode(b.Content)
	case "upca":
		if len(b.Content) != 11 && len(b.Content) != 12 {
			return nil, fmt.Errorf("upca requires 11 or 12 digits, got %d", len(b.Content))
		}
		return ean.Encode("0" + b.Content)
	case "code39":
		return code39.Encode(b.Content, false, false)
	case "itf":
		return twooffive.Encode(b.Content, true)
	default:
		return nil, fmt.Errorf("unsupported symbology %q", b.Symbology)
	}
}

// Validate reports whether b is well-formed: Content must be non-empty
// and valid UTF-8, Symbology must be one of BarcodeSymbologies, Content
// must be encodable as that symbology — checked by attempting the real
// encode (Encode) rather than reimplementing its rules — and Height, if
// positive, must not exceed maxBarcodeHeightDots. A zero or negative
// Height is valid: it means DefaultBarcodeHeight, not an error.
func (b Barcode) Validate() error {
	if b.Content == "" {
		return errors.New("barcode: content is required")
	}
	if !utf8.ValidString(b.Content) {
		return errors.New("barcode: content must be valid UTF-8")
	}
	if b.Symbology == "" {
		return errors.New("barcode: symbology is required")
	}
	if !IsSupportedBarcodeSymbology(b.Symbology) {
		return fmt.Errorf("barcode: unsupported symbology %q (supported: %s)", b.Symbology, strings.Join(BarcodeSymbologies, ", "))
	}
	if b.Height > maxBarcodeHeightDots {
		return fmt.Errorf("barcode: height must not exceed %d, got %d", maxBarcodeHeightDots, b.Height)
	}
	if _, err := b.Encode(); err != nil {
		return fmt.Errorf("barcode: content cannot be encoded as %s: %w", b.Symbology, err)
	}
	return nil
}

// MarshalJSON encodes b with the "type":"barcode" discriminator the
// registry polymorphism decodes it back through (docs/adr/0001-receipt-model.md).
func (b Barcode) MarshalJSON() ([]byte, error) {
	type alias Barcode
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "barcode", alias: alias(b)})
}

func init() {
	registerElement("barcode", func(data []byte, _ int) (Element, error) {
		var b Barcode
		if err := json.Unmarshal(data, &b); err != nil {
			return nil, err
		}
		return b, nil
	})
}
