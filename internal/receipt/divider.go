package receipt

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Divider is a horizontal rule. Style selects "solid" (default) or
// "dashed". Size is an integer thickness scale factor ("0 or omitted means
// unscaled", the Text.Size convention): the line is
// render/layout.DividerThickness dots at Size 1, or a multiple of it for a
// heavier rule — see docs/adr/0012-divider-thickness-default-and-scaling.md.
type Divider struct {
	Style string `json:"style,omitempty"`
	Size  int    `json:"size,omitempty"`
}

// maxDividerSize bounds Size for the same reason as Text.maxTextSize:
// keep the paint pipeline's arithmetic clear of excessive allocation or
// overflow, bounded far above any legitimate use but finite.
const maxDividerSize = 100

// Validate reports whether d is well-formed: Style must be empty,
// "solid", or "dashed", and Size, if set, must be within [0, maxDividerSize].
func (d Divider) Validate() error {
	switch d.Style {
	case "", "solid", "dashed":
	default:
		return fmt.Errorf("divider: invalid style %q", d.Style)
	}
	if d.Size < 0 {
		return errors.New("divider: size must not be negative")
	}
	if d.Size > maxDividerSize {
		return fmt.Errorf("divider: size must not exceed %d, got %d", maxDividerSize, d.Size)
	}
	return nil
}

// MarshalJSON encodes d with the "type":"divider" discriminator the
// registry polymorphism decodes it back through (docs/adr/0001-receipt-model.md).
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
