package layout

import (
	"fmt"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// Build turns r into a Document: each receipt.Text becomes one Block,
// stacked top to bottom in Receipt order, each Y advancing by f.LineHeight().
//
// This is an early, partial implementation of the Build described in
// docs/ARCHITECTURE.md §2 — it does not yet accept a context.Context,
// printer.Profile, or assets.Store, since this slice performs no I/O and
// has nothing printer-width- or asset-dependent to do with them. Element
// types other than receipt.Text are not yet supported and are reported as
// an apperr.KindPermanent error rather than skipped or given placeholder
// positions.
func Build(r receipt.Receipt, f Font) (Document, error) {
	var blocks []Block
	y := 0
	for _, el := range r.Elements {
		text, ok := el.(receipt.Text)
		if !ok {
			return Document{}, apperr.Wrap(apperr.KindPermanent, "layout.Build", fmt.Errorf("unsupported element type %T", el))
		}
		blocks = append(blocks, Block{Y: y, Element: text})
		y += f.LineHeight()
	}
	return Document{Blocks: blocks}, nil
}
