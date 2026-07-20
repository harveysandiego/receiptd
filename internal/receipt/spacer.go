// Feed and Spacer are two of several structurally-identical Element
// boilerplate types the registry pattern produces by design (see
// docs/adr/0001-receipt-model.md) — one struct field, a bound check, and
// the same Validate/MarshalJSON/init shape every other Element file
// repeats. A shared abstraction for two ~50-line files would cost more
// than the duplication it removes (CLAUDE.md: "Three similar lines is
// better than a premature abstraction").

//nolint:dupl // see the file comment above
package receipt

import (
	"encoding/json"
	"fmt"
)

// Spacer inserts vertical blank space, Height dots tall.
type Spacer struct {
	Height int `json:"height"`
}

// maxSpacerHeightDots bounds a single Spacer's Height. render/canvas
// allocates its bitmap buffer sized directly off summed block heights, so
// an unbounded Height would let one Spacer force an arbitrarily large
// allocation; 10,000 dots is about 1.25m at 203 DPI, far beyond any real
// receipt's use, but finite.
const maxSpacerHeightDots = 10000

// Validate reports whether s is well-formed: Height must be non-negative
// and no larger than maxSpacerHeightDots.
func (s Spacer) Validate() error {
	if s.Height < 0 {
		return fmt.Errorf("spacer: height must be non-negative, got %d", s.Height)
	}
	if s.Height > maxSpacerHeightDots {
		return fmt.Errorf("spacer: height must not exceed %d dots, got %d", maxSpacerHeightDots, s.Height)
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
	registerElement("spacer", func(data []byte, _ int) (Element, error) {
		var s Spacer
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		return s, nil
	})
}
