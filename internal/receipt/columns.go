package receipt

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// Column is one side-by-side segment of a Columns element. Weight controls
// its share of the printable width relative to its siblings (0 or omitted
// means "unscaled", the convention Text.Size and Divider.Size also use);
// Elements is its own ordered content, validated and laid out like
// Receipt.Elements (docs/ARCHITECTURE.md §3).
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
// negative Weight, and every column's own Elements recursively valid
// (docs/ARCHITECTURE.md §3).
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

// MarshalJSON encodes c with the "type":"columns" discriminator the
// registry polymorphism decodes it back through (docs/adr/0001-receipt-model.md).
// Each Column's Elements marshal via their own Element.MarshalJSON, so no
// second marshaling method is needed here.
func (c Columns) MarshalJSON() ([]byte, error) {
	type alias Columns
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "columns", alias: alias(c)})
}

// UnmarshalJSON decodes c from the discriminated-union JSON shape
// (docs/ARCHITECTURE.md §3), resolving each column element's "type" string
// through the same registry Receipt.UnmarshalJSON uses. It starts depth
// tracking at 0: a caller reaching Columns through the json.Unmarshaler
// interface (rather than the registry's "columns" decoder) is the top of
// its own decode — see unmarshalJSON.
func (c *Columns) UnmarshalJSON(data []byte) error {
	return c.unmarshalJSON(data, 0)
}

// unmarshalJSON is UnmarshalJSON's depth-aware implementation, called
// directly by the "columns" decoder registered in init() (the
// json.Unmarshaler interface cannot carry the extra depth argument) so
// that nested Columns share one running depth count rather than each
// resetting to 0. Without that bound, a deeply nested columns-in-columns
// payload recurses on the Go call stack unbounded — see maxElementDepth.
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
