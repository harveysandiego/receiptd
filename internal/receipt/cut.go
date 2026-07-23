package receipt

import (
	"encoding/json"
	"fmt"
)

// Cut is a printer-control element: an explicit ESC/POS cut. Mode selects
// "full" or "partial"; when empty, render/escpos.Encode supplies
// printer.Profile.DefaultCut instead. Like Feed it reserves no space in
// the rendered bitmap — layout and canvas treat it as zero height, and
// render/escpos.Encode is the only stage that turns it into bytes.
type Cut struct {
	Mode string `json:"mode,omitempty"`
}

// Validate reports whether c is well-formed: Mode must be empty, "full",
// or "partial". These are the values printer.Profile.DefaultCut accepts,
// checked here without importing printer since Validate is fast and local
// (docs/ARCHITECTURE.md §5).
func (c Cut) Validate() error {
	switch c.Mode {
	case "", "full", "partial":
		return nil
	default:
		return fmt.Errorf("cut: invalid mode %q", c.Mode)
	}
}

// MarshalJSON encodes c with the "type":"cut" discriminator the registry
// polymorphism decodes it back through (docs/adr/0001-receipt-model.md).
func (c Cut) MarshalJSON() ([]byte, error) {
	type alias Cut
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "cut", alias: alias(c)})
}

func init() {
	registerElement("cut", func(data []byte, _ int) (Element, error) {
		var c Cut
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, err
		}
		return c, nil
	})
}
