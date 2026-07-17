package receipt

import "errors"

// Heading is a Text-like element that implies bold, large styling at
// render time. It has no Align, Bold, or Size fields of its own — those
// are fixed by definition, not client-configurable.
type Heading struct {
	Content string
}

// Validate reports whether h is well-formed: Content must be non-empty.
func (h Heading) Validate() error {
	if h.Content == "" {
		return errors.New("heading: content is required")
	}
	return nil
}
