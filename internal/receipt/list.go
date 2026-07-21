package receipt

import (
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

// List is a bulleted, numbered, or checkbox list — one Element type for
// all three, distinguished by the closed-enum Kind, per
// docs/adr/0014-list-elements.md. List carries no styling fields of its
// own, rendering as plain unstyled text like Table and Columns; an author
// wanting styled list text composes it from Text.
type List struct {
	// Kind selects the marker style: "" and "bullet" are equivalent (the
	// default), "number" renders a 1-based sequential number independent
	// of Indent, and "checkbox" renders ListItem.Checked as "[x]"/"[ ]".
	Kind  string     `json:"kind,omitempty"`
	Items []ListItem `json:"items"`
}

// ListItem is one entry in a List. Content is plain text, not arbitrary
// Elements — the same "flat, no nested Elements" design Table's
// Headers/Rows already use. Checked is only meaningful when the parent
// List.Kind is "checkbox". Indent is a semantic nesting level (0 = top
// level), not a count of characters — how much visual space one level
// occupies is a rendering choice, not part of this type's contract. See
// docs/adr/0014-list-elements.md.
type ListItem struct {
	Content string `json:"content"`
	Checked bool   `json:"checked,omitempty"`
	Indent  int    `json:"indent,omitempty"`
}

// maxListIndent defensively bounds ListItem.Indent against pathological
// input that would consume the entire printable width in leading space
// before any content is rendered — the same input-hygiene posture
// maxElementDepth (registry.go) already takes for Columns nesting depth.
// This is an implementation detail, not part of List's public contract
// (docs/adr/0014-list-elements.md deliberately leaves the exact bound
// unspecified), so it is unexported and may be tuned without that being a
// schema-visible or architectural change.
const maxListIndent = 8

// Validate reports whether l is well-formed: Kind must be "", "bullet",
// "number", or "checkbox"; Items must be non-empty; and every item's own
// Content must be non-empty and valid UTF-8, Indent must be between 0 and
// maxListIndent inclusive, and Checked must be false unless Kind is
// "checkbox" — an invalid combination is rejected outright rather than
// silently ignored, the same posture this schema always takes for a
// combination a closed enum doesn't otherwise prevent (e.g. an
// unrecognized Barcode.Symbology).
func (l List) Validate() error {
	switch l.Kind {
	case "", "bullet", "number", "checkbox":
	default:
		return fmt.Errorf("list: invalid kind %q", l.Kind)
	}
	if len(l.Items) == 0 {
		return errors.New("list: at least one item is required")
	}

	var errs []error
	for i, item := range l.Items {
		if item.Content == "" {
			errs = append(errs, fmt.Errorf("list: item %d: content is required", i))
		} else if !utf8.ValidString(item.Content) {
			errs = append(errs, fmt.Errorf("list: item %d: content must be valid UTF-8", i))
		}
		if item.Indent < 0 {
			errs = append(errs, fmt.Errorf("list: item %d: indent must not be negative", i))
		} else if item.Indent > maxListIndent {
			errs = append(errs, fmt.Errorf("list: item %d: indent %d exceeds the maximum of %d", i, item.Indent, maxListIndent))
		}
		if item.Checked && l.Kind != "checkbox" {
			errs = append(errs, fmt.Errorf("list: item %d: checked is only valid when kind is %q", i, "checkbox"))
		}
	}
	return errors.Join(errs...)
}

// MarshalJSON encodes l alongside the "type":"list" discriminator the
// registry-based polymorphism in docs/adr/0001-receipt-model.md relies on
// to decode it back.
func (l List) MarshalJSON() ([]byte, error) {
	type alias List
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "list", alias: alias(l)})
}

func init() {
	registerElement("list", func(data []byte, _ int) (Element, error) {
		var l List
		if err := json.Unmarshal(data, &l); err != nil {
			return nil, err
		}
		return l, nil
	})
}
