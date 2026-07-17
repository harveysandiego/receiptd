package receipt

import (
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
	Version  int
	Copies   int
	Elements []Element
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
