package receipt

import "fmt"

// Divider is a horizontal rule. Style is optional; when empty,
// render/layout chooses a default.
type Divider struct {
	Style string
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
