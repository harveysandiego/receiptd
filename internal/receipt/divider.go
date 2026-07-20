package receipt

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Divider is a horizontal rule. Style selects "solid" (the default, a
// continuous line) or "dashed" (a repeating on/off pattern — see
// render/canvas.paintDivider); both render distinctly, unlike
// Text.Align and Asset.Align, which are still ahead of their own
// implementation (see each type's own doc comment). Size is
// an integer thickness scale factor, the same "0 or omitted means
// unscaled" convention Text.Size uses (docs/adr/0007-bitmap-text-styling.md):
// the rendered line is render/layout.DividerThickness dots at Size 1, or
// a Size multiple of it for a deliberately heavier rule — see
// docs/adr/0012-divider-thickness-default-and-scaling.md.
type Divider struct {
	Style string `json:"style,omitempty"`
	Size  int    `json:"size,omitempty"`
}

// Validate reports whether d is well-formed: Style must be empty,
// "solid", or "dashed" — the values docs/ARCHITECTURE.md defines — and
// Size, if set, must not be negative (the same rule Text.Validate()
// applies to its own Size).
func (d Divider) Validate() error {
	switch d.Style {
	case "", "solid", "dashed":
	default:
		return fmt.Errorf("divider: invalid style %q", d.Style)
	}
	if d.Size < 0 {
		return errors.New("divider: size must not be negative")
	}
	return nil
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
	registerElement("divider", func(data []byte, _ int) (Element, error) {
		var d Divider
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, err
		}
		return d, nil
	})
}
