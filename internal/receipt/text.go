package receipt

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Text is a line, or paragraph, of plain content. Align, Bold, Italic,
// Underline, Strikethrough, and Size are rendering hints interpreted by
// the bitmap renderer (docs/ARCHITECTURE.md §3 "Text styling",
// docs/adr/0007-bitmap-text-styling.md, docs/adr/0013-text-and-asset-alignment.md).
// Align is a closed enum — "" (omitted, left), "left", "center", or
// "right" — the same closed-vocabulary pattern Barcode.Symbology and
// QRCode.ErrorCorrection already establish, applied here per ADR-0013.
type Text struct {
	Content       string `json:"content"`
	Align         string `json:"align,omitempty"`
	Bold          bool   `json:"bold,omitempty"`
	Italic        bool   `json:"italic,omitempty"`
	Underline     bool   `json:"underline,omitempty"`
	Strikethrough bool   `json:"strikethrough,omitempty"`
	Size          int    `json:"size,omitempty"`
}

// Validate reports whether t is well-formed: Content must be non-empty,
// Align must be "", "left", "center", or "right" (docs/adr/0013-text-and-asset-alignment.md),
// and Size, if set, must not be negative. Size is an integer bitmap scale
// factor (docs/adr/0007-bitmap-text-styling.md); 0 (omitted) means
// "unscaled" and is normalized to 1 by render/layout.Build, not rejected
// here.
func (t Text) Validate() error {
	if t.Content == "" {
		return errors.New("text: content is required")
	}
	switch t.Align {
	case "", "left", "center", "right":
	default:
		return fmt.Errorf("text: invalid align %q", t.Align)
	}
	if t.Size < 0 {
		return errors.New("text: size must not be negative")
	}
	return nil
}

// MarshalJSON encodes t alongside the "type":"text" discriminator the
// registry-based polymorphism in docs/adr/0001-receipt-model.md relies on
// to decode it back.
func (t Text) MarshalJSON() ([]byte, error) {
	type alias Text
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "text", alias: alias(t)})
}

func init() {
	registerElement("text", func(data []byte, _ int) (Element, error) {
		var t Text
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, err
		}
		return t, nil
	})
}
