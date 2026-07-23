package receipt

import (
	"encoding/json"
	"errors"
	"fmt"

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
	// Copies is how many physical copies app.Service.Process sends to the
	// printer for one queued Job. Must be within [0, maxCopies]; negative
	// or over that bound is rejected by Validate below. See
	// EffectiveCopies for how zero is interpreted.
	Copies   int       `json:"copies"`
	Elements []Element `json:"elements"`
}

// maxCopies bounds Receipt.Copies. app.Service.Process sends the same
// encoded bytes to the printer once per copy, so an unbounded value would
// let a single authenticated request occupy one printer's worker
// (docs/adr/0016-queue-concurrency-per-printer-workers.md) indefinitely.
// 100 is far beyond any legitimate multi-copy scenario (a handful of
// copies at most — e.g. merchant and customer) but finite.
const maxCopies = 100

// EffectiveCopies is how many times app.Service.Process should send a
// rendered Receipt to the printer. A Copies below 1 (omitted, or a Receipt
// predating the field) means one. maxCopies is clamped here, not only
// rejected by Validate, so the send loop stays bounded for a Job that
// reached Process without Validate — one requeued by reconciliation, or
// built in-process by a future template.
func (r Receipt) EffectiveCopies() int {
	if r.Copies < 1 {
		return 1
	}
	if r.Copies > maxCopies {
		return maxCopies
	}
	return r.Copies
}

// Validate aggregates every Element's Validate() via errors.Join and
// wraps the result as apperr.KindValidation. A nil entry in Elements is
// itself a validation failure rather than a panic.
func (r Receipt) Validate() error {
	var errs []error
	if r.Copies < 0 {
		errs = append(errs, fmt.Errorf("copies must not be negative, got %d", r.Copies))
	}
	if r.Copies > maxCopies {
		errs = append(errs, fmt.Errorf("copies must not exceed %d, got %d", maxCopies, r.Copies))
	}
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
