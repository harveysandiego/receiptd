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

// Feed is a printer-control element: an explicit ESC/POS paper feed of
// Lines print lines, emitted at Feed's position among the Receipt's
// Elements. Unlike Spacer, Feed reserves no space in the rendered
// bitmap — feed and cut are two of the three genuine ESC/POS commands
// this design uses (docs/adr/0002-raster-rendering.md), not Canvas
// content, so render/layout and render/canvas treat Feed as occupying
// zero height while render/escpos.Encode is the stage that turns it into
// bytes at the right position in the output stream.
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

// MarshalJSON encodes f alongside the "type":"feed" discriminator the
// registry-based polymorphism in docs/adr/0001-receipt-model.md relies on
// to decode it back.
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
