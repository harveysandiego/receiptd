package receipt

import (
	"encoding/json"
	"fmt"
)

// Divider is a horizontal rule. Style is optional; when empty,
// render/layout chooses a default.
type Divider struct {
	Style string `json:"style,omitempty"`
}

// Validate reports whether d is well-formed: Style must be empty,
// "solid", or "dashed" — the values docs/ARCHITECTURE.md defines.
func (d Divider) Validate() error {
	switch d.Style {
	case "", "solid", "dashed":
		return nil
	default:
		return fmt.Errorf("divider: invalid style %q", d.Style)
	}
}

// MarshalJSON encodes d alongside the "type":"divider" discriminator the
// registry-based polymorphism in docs/adr/0001-receipt-model.md relies on
// to decode it back.
func (d Divider) MarshalJSON() ([]byte, error) {
	type alias Divider
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "divider", alias: alias(d)})
}

func init() {
	registerElement("divider", func(data []byte) (Element, error) {
		var d Divider
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, err
		}
		return d, nil
	})
}
