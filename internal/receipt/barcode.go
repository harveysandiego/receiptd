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

// Barcode is a receipt element that renders as a generated 1D barcode
// bitmap encoding Content — the same "generation" pattern QRCode already
// establishes (render/layout.GenerateBarcodeBitmap turns it into pixels
// at render time, which then flow through the same raster pipeline Image
// and QRCode already use).
type Barcode struct {
	Content string `json:"content"`

	// Symbology selects which barcode standard Content is encoded as: one
	// of BarcodeSymbologies. See docs/adr/0009-barcode-symbologies.md for
	// why this is a fixed, closed set rather than every symbology
	// github.com/boombuler/barcode itself supports.
	Symbology string `json:"symbology"`

	// Height is the barcode's target height in dots (its bar thickness,
	// not related to its width). Zero or omitted means
	// DefaultBarcodeHeight. Unlike QRCode.Size, this has no effect on the
	// barcode's width: a barcode's width is driven entirely by how many
	// modules Content encodes into.
	Height int `json:"height,omitempty"`

	// ShowText selects whether Content is printed as human-readable text
	// beneath the bars: render/layout.Build adds one extra
	// render/layout.BarcodeCaption Block, Content space-padded to sit
	// roughly centered under the barcode's own rendered width
	// (render/layout.centerBarcodeCaption — leading spaces sized to the
	// embedded font's fixed glyph advance, not a geometric/font-independent
	// centering), which render/canvas.Paint paints through the same
	// glyph-by-glyph path any other text Block uses.
	ShowText bool `json:"show_text,omitempty"`
}

// DefaultBarcodeHeight is the height, in dots, a Barcode renders at when
// Height is omitted or non-positive.
const DefaultBarcodeHeight = 80

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
// encoder for b.Symbology, returning an error if Content is not valid
// data for that symbology. Exported so render/layout.GenerateBarcodeBitmap
// performs the exact same encode Validate already checked.
//
// ean13 and ean8 each require Content's length to match the chosen
// symbology (12/13 digits, 7/8 digits respectively) before delegating to
// github.com/boombuler/barcode/ean, which otherwise infers EAN-8 vs
// EAN-13 purely from length. upca has no dedicated encoder in that
// library, so it is encoded as EAN-13 with Content prefixed "0".
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
// and valid UTF-8, Symbology must be one of BarcodeSymbologies, and
// Content must actually be encodable as that symbology — checked by
// attempting the real encode (Encode), the same "Validate does the real
// local work rather than reimplementing its rules" precedent
// QRCode.Validate already sets. Height and ShowText are never invalid —
// see their doc comments on Barcode for how each is handled.
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
	if _, err := b.Encode(); err != nil {
		return fmt.Errorf("barcode: content cannot be encoded as %s: %w", b.Symbology, err)
	}
	return nil
}

// MarshalJSON encodes b alongside the "type":"barcode" discriminator the
// registry-based polymorphism in docs/adr/0001-receipt-model.md relies on
// to decode it back.
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
