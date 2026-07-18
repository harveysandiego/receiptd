package layout

import (
	"fmt"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// Build turns r into a Document: each receipt.Text, receipt.Heading, or
// receipt.Spacer becomes one Block, stacked top to bottom in Receipt
// order. Text and Heading blocks advance Y by f.LineHeight(); a Spacer
// block advances Y by its own Height (dots), per its documented meaning
// in docs/ARCHITECTURE.md §3. The returned Document carries f, so every
// later stage (e.g. render/canvas.Paint) measures and paints against the
// same Font Build used.
//
// This is an early, partial implementation of the Build described in
// docs/ARCHITECTURE.md §2 — it does not yet accept a context.Context,
// printer.Profile, or assets.Store, since this slice performs no I/O and
// has nothing printer-width- or asset-dependent to do with them. Heading's
// documented "bold + large" styling (docs/ARCHITECTURE.md §3) is not
// applied here: Font has no notion of weight or size variation yet (the
// same gap that already leaves receipt.Text's own Bold/Size hints
// unapplied), so a Heading is positioned exactly like a Text with the same
// Content. Element types other than receipt.Text, receipt.Heading, and
// receipt.Spacer are not yet supported and are reported as an
// apperr.KindPermanent error rather than skipped or given placeholder
// positions.
func Build(r receipt.Receipt, f Font) (Document, error) {
	var blocks []Block
	y := 0
	for _, el := range r.Elements {
		switch e := el.(type) {
		case receipt.Text, receipt.Heading:
			blocks = append(blocks, Block{Y: y, Element: el})
			y += f.LineHeight()
		case receipt.Spacer:
			blocks = append(blocks, Block{Y: y, Element: el})
			y += e.Height
		default:
			return Document{}, apperr.Wrap(apperr.KindPermanent, "layout.Build", fmt.Errorf("unsupported element type %T", el))
		}
	}
	return Document{Blocks: blocks, Font: f}, nil
}
