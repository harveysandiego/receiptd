package receipt

import (
	"encoding/json"
	"errors"
)

// Text is a line, or paragraph, of plain content. Align, Bold, and Size
// are rendering hints interpreted by render/layout; docs/ARCHITECTURE.md
// does not define a fixed set of valid values for Align or Size, so
// Validate does not restrict them.
type Text struct {
	Content string `json:"content"`
	Align   string `json:"align,omitempty"`
	Bold    bool   `json:"bold,omitempty"`
	Size    string `json:"size,omitempty"`
}

// Validate reports whether t is well-formed: Content must be non-empty.
func (t Text) Validate() error {
	if t.Content == "" {
		return errors.New("text: content is required")
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
	registerElement("text", func(data []byte) (Element, error) {
		var t Text
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, err
		}
		return t, nil
	})
}
