package receipt

import (
	"encoding/json"
	"fmt"
)

// Spacer inserts vertical blank space, Height dots tall.
type Spacer struct {
	Height int `json:"height"`
}

// Validate reports whether s is well-formed: Height must not be negative.
func (s Spacer) Validate() error {
	if s.Height < 0 {
		return fmt.Errorf("spacer: height must be non-negative, got %d", s.Height)
	}
	return nil
}

// MarshalJSON encodes s alongside the "type":"spacer" discriminator the
// registry-based polymorphism in docs/adr/0001-receipt-model.md relies on
// to decode it back.
func (s Spacer) MarshalJSON() ([]byte, error) {
	type alias Spacer
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "spacer", alias: alias(s)})
}

func init() {
	registerElement("spacer", func(data []byte) (Element, error) {
		var s Spacer
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		return s, nil
	})
}
