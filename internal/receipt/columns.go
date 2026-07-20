package receipt

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// Column is one side-by-side segment of a Columns element. Weight controls
// its share of the printable width relative to its sibling columns (0 or
// omitted means "unscaled", the same convention Text.Size and Divider.Size
// already use — see render/layout.ResolveSize); Elements is its own
// ordered content, validated and laid out exactly like Receipt.Elements
// (docs/ARCHITECTURE.md §3).
type Column struct {
	Weight   int       `json:"weight,omitempty"`
	Elements []Element `json:"elements"`
}

// Columns lays its Columns out side by side across the printable width,
// each sized proportionally to its own Weight (docs/ARCHITECTURE.md §3).
type Columns struct {
	Columns []Column `json:"columns"`
}

// Validate reports whether c is well-formed: at least one column, no
// column with a negative Weight (the same "negative is the only invalid
// scale factor" rule Text.Size and Divider.Size already use), and every
// column's own Elements recursively valid — the "recursing into Columns"
// behaviour docs/ARCHITECTURE.md §3 already documents for Receipt.Validate.
func (c Columns) Validate() error {
	if len(c.Columns) == 0 {
		return errors.New("columns: at least one column is required")
	}
	var errs []error
	for i, col := range c.Columns {
		if col.Weight < 0 {
			errs = append(errs, fmt.Errorf("columns: column %d: weight must not be negative", i))
		}
		for j, el := range col.Elements {
			if el == nil {
				errs = append(errs, fmt.Errorf("columns: column %d: element %d: nil element", i, j))
				continue
			}
			if err := el.Validate(); err != nil {
				errs = append(errs, fmt.Errorf("columns: column %d: element %d: %w", i, j, err))
			}
		}
	}
	return errors.Join(errs...)
}

// MarshalJSON encodes c alongside the "type":"columns" discriminator the
// registry-based polymorphism in docs/adr/0001-receipt-model.md relies on
// to decode it back. Each Column's own Elements marshal via their
// individual Element.MarshalJSON implementations, the same "no second
// marshaling method needed" property Receipt.Elements already relies on.
func (c Columns) MarshalJSON() ([]byte, error) {
	type alias Columns
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "columns", alias: alias(c)})
}

// UnmarshalJSON decodes c from the discriminated-union JSON shape
// described in docs/ARCHITECTURE.md §3: each column's "elements" entries
// carry their own "type" string, resolved through the same registry
// Receipt.UnmarshalJSON uses — encoding/json cannot populate a []Element
// field on its own, since Element is an interface. Always starts depth
// tracking fresh at 0 — see unmarshalJSON's own doc comment for why a
// caller reaching Columns through this method (rather than through the
// registry's own "columns" decoder) is definitionally the top of its own
// decode.
func (c *Columns) UnmarshalJSON(data []byte) error {
	return c.unmarshalJSON(data, 0)
}

// unmarshalJSON is UnmarshalJSON's depth-aware implementation. It is
// called directly — bypassing the json.Unmarshaler dispatch UnmarshalJSON
// satisfies, which cannot carry extra arguments — by the "columns" entry
// this file registers in init(), so that Columns nested inside a sibling
// Columns's own Column.Elements shares one running depth count with its
// ancestor calls rather than each resetting to 0. See maxElementDepth's
// doc comment in registry.go for why this matters: without it, decoding a
// deeply nested columns-in-columns payload recurses on the Go call stack
// with no bound.
func (c *Columns) unmarshalJSON(data []byte, depth int) error {
	var wire struct {
		Columns []struct {
			Weight   int               `json:"weight,omitempty"`
			Elements []json.RawMessage `json:"elements"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return apperr.Wrap(apperr.KindValidation, "receipt.Columns.UnmarshalJSON", err)
	}

	cols := make([]Column, len(wire.Columns))
	for i, wc := range wire.Columns {
		var elements []Element
		if wc.Elements != nil {
			elements = make([]Element, len(wc.Elements))
			for j, raw := range wc.Elements {
				el, err := decodeElement(raw, depth+1)
				if err != nil {
					return err
				}
				elements[j] = el
			}
		}
		cols[i] = Column{Weight: wc.Weight, Elements: elements}
	}

	c.Columns = cols
	return nil
}

func init() {
	registerElement("columns", func(data []byte, depth int) (Element, error) {
		var c Columns
		if err := c.unmarshalJSON(data, depth); err != nil {
			return nil, err
		}
		return c, nil
	})
}
