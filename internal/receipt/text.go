package receipt

import "errors"

// Text is a line, or paragraph, of plain content. Align, Bold, and Size
// are rendering hints interpreted by render/layout; docs/ARCHITECTURE.md
// does not define a fixed set of valid values for Align or Size, so
// Validate does not restrict them.
type Text struct {
	Content string
	Align   string
	Bold    bool
	Size    string
}

// Validate reports whether t is well-formed: Content must be non-empty.
func (t Text) Validate() error {
	if t.Content == "" {
		return errors.New("text: content is required")
	}
	return nil
}
