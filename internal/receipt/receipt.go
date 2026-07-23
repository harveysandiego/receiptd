package receipt

import (
	"encoding/json"
	"errors"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// Element is anything that can appear in a Receipt's ordered content —
// text, a heading, a divider, a spacer, and so on. Validate checks only
// the element's own local invariants: no I/O, no reference to rendering,
// layout, or printers. See docs/adr/0001-receipt-model.md.
type Element interface {
	Validate() error
}

// Receipt is a printer-agnostic document: an ordered list of Elements.
// It carries no paper width, DPI, or printer identity — those are
// resolved server-side, by printer name, at render time.
type Receipt struct {
	Version int `json:"version"`
	// Copies is decoded and round-tripped through the API and CLI, but
	// nothing in the render/print pipeline reads it yet — every Job
	// prints exactly once regardless of its value. Multi-copy printing is
	// unimplemented, not silently broken; see the README's REST API
	// examples section.
	Copies   int       `json:"copies"`
	Elements []Element `json:"elements"`
}

// Validate aggregates every Element's Validate() via errors.Join and
// wraps the result as apperr.KindValidation. A nil entry in Elements is
// itself a validation failure rather than a panic.
func (r Receipt) Validate() error {
	var errs []error
	for _, el := range r.Elements {
		if el == nil {
			errs = append(errs, errors.New("receipt: nil element"))
			continue
		}
		if err := el.Validate(); err != nil {
			errs = append(errs, err)
		}
	}

	if err := errors.Join(errs...); err != nil {
		return apperr.Wrap(apperr.KindValidation, "receipt.Validate", err)
	}
	return nil
}

// UnmarshalJSON decodes a Receipt from the discriminated-union JSON shape
// (docs/ARCHITECTURE.md §3): each "elements" entry's "type" string selects
// the concrete Go type, resolved through the registry. Marshaling needs no
// equivalent method — each Element type implements json.Marshaler itself,
// so default struct marshaling of Elements already produces the right shape.
func (r *Receipt) UnmarshalJSON(data []byte) error {
	var wire struct {
		Version  int               `json:"version"`
		Copies   int               `json:"copies"`
		Elements []json.RawMessage `json:"elements"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return apperr.Wrap(apperr.KindValidation, "receipt.UnmarshalJSON", err)
	}

	var elements []Element
	if wire.Elements != nil {
		elements = make([]Element, len(wire.Elements))
		for i, raw := range wire.Elements {
			el, err := decodeElement(raw, 0)
			if err != nil {
				return err
			}
			elements[i] = el
		}
	}

	r.Version = wire.Version
	r.Copies = wire.Copies
	r.Elements = elements
	return nil
}
