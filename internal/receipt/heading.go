package receipt

import (
	"encoding/json"
	"errors"
)

// Heading is a Text-like element that implies bold, large styling at
// render time. It has no Align, Bold, or Size fields of its own — those
// are fixed by definition, not client-configurable.
type Heading struct {
	Content string `json:"content"`
}

// Validate reports whether h is well-formed: Content must be non-empty.
func (h Heading) Validate() error {
	if h.Content == "" {
		return errors.New("heading: content is required")
	}
	return nil
}

// MarshalJSON encodes h alongside the "type":"heading" discriminator the
// registry-based polymorphism in docs/adr/0001-receipt-model.md relies on
// to decode it back.
func (h Heading) MarshalJSON() ([]byte, error) {
	type alias Heading
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "heading", alias: alias(h)})
}

func init() {
	registerElement("heading", func(data []byte) (Element, error) {
		var h Heading
		if err := json.Unmarshal(data, &h); err != nil {
			return nil, err
		}
		return h, nil
	})
}
