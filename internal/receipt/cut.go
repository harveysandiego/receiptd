package receipt

import (
	"encoding/json"
	"fmt"
)

// Cut is a printer-control element: an explicit ESC/POS cut, emitted at
// Cut's position among the Receipt's Elements. Mode selects "full" or
// "partial"; when empty, render/escpos.Encode supplies
// printer.Profile.DefaultCut instead, per docs/ARCHITECTURE.md §3's
// element table. Like Feed, Cut reserves no space in the rendered
// bitmap — render/layout and render/canvas treat it as occupying zero
// height, and render/escpos.Encode is the only stage that turns it into
// bytes.
type Cut struct {
	Mode string `json:"mode,omitempty"`
}

// Validate reports whether c is well-formed: Mode must be empty, "full",
// or "partial" — the same values printer.Profile.DefaultCut accepts, but
// checked here without importing printer, since Validate is fast and
// local by design (docs/ARCHITECTURE.md §5) and has no access to a
// specific printer's Profile to resolve an empty Mode against.
func (c Cut) Validate() error {
	switch c.Mode {
	case "", "full", "partial":
		return nil
	default:
		return fmt.Errorf("cut: invalid mode %q", c.Mode)
	}
}

// MarshalJSON encodes c alongside the "type":"cut" discriminator the
// registry-based polymorphism in docs/adr/0001-receipt-model.md relies on
// to decode it back.
func (c Cut) MarshalJSON() ([]byte, error) {
	type alias Cut
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "cut", alias: alias(c)})
}

func init() {
	registerElement("cut", func(data []byte) (Element, error) {
		var c Cut
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, err
		}
		return c, nil
	})
}
