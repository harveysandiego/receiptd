// Feed and Spacer are structurally-identical boilerplate the registry
// pattern produces by design (docs/adr/0001-receipt-model.md). A shared
// abstraction for two ~50-line files would cost more than the duplication
// it removes (CLAUDE.md: "Three similar lines is better than a premature
// abstraction").

//nolint:dupl // see the file comment above
package receipt

import (
	"encoding/json"
	"fmt"
)

// Feed is a printer-control element: an explicit ESC/POS paper feed of
// Lines print lines. Unlike Spacer it reserves no space in the rendered
// bitmap — feed and cut are genuine ESC/POS commands, not Canvas content
// (docs/adr/0002-raster-rendering.md), so layout and canvas treat Feed as
// zero height and render/escpos.Encode is the stage that emits its bytes.
type Feed struct {
	Lines int `json:"lines"`
}

// maxFeedLines bounds Lines to what render/escpos.Encode's ESC d n command
// can represent: n is a single byte, so anything larger would silently
// wrap rather than feed the requested distance.
const maxFeedLines = 255

// Validate reports whether f is well-formed: Lines must be strictly
// positive and no larger than maxFeedLines. Unlike Spacer.Height, 0 is
// not a meaningful Feed — a receipt author who wants no feed simply omits
// the element — so it is rejected here rather than treated as a no-op.
func (f Feed) Validate() error {
	if f.Lines <= 0 {
		return fmt.Errorf("feed: lines must be positive, got %d", f.Lines)
	}
	if f.Lines > maxFeedLines {
		return fmt.Errorf("feed: lines must not exceed %d, got %d", maxFeedLines, f.Lines)
	}
	return nil
}

// MarshalJSON encodes f with the "type":"feed" discriminator the registry
// polymorphism decodes it back through (docs/adr/0001-receipt-model.md).
func (f Feed) MarshalJSON() ([]byte, error) {
	type alias Feed
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "feed", alias: alias(f)})
}

func init() {
	registerElement("feed", func(data []byte, _ int) (Element, error) {
		var f Feed
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, err
		}
		return f, nil
	})
}
