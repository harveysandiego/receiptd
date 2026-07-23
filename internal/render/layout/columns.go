package layout

import (
	"fmt"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

// ColumnsLine is one already-wrapped, column-aligned line of a
// receipt.Columns's output — see TableLine for why Build produces a
// distinct Block-carrying type rather than reusing receipt.Text.
// render/canvas.Paint paints its Content through the same glyph-by-glyph
// path as receipt.Text (canvas.textContent).
type ColumnsLine struct {
	Content string
}

// Validate always succeeds — see TableLine.Validate's identical doc
// comment: ColumnsLine is never part of a client-supplied receipt.Receipt,
// it exists only as a Block.Element Build itself produces.
func (ColumnsLine) Validate() error { return nil }

// columnsLines turns c into the plain-text lines Build lays out as
// ColumnsLine Blocks: each column's content word-wrapped to its share of
// widthDots (columnWidths) and composed side by side — the same technique
// tableLines uses, generalized from a Table's string cells to a Column's
// []receipt.Element.
//
// Only receipt.Text is supported inside a column (see columnLines): a Block
// carries no horizontal position and no per-run styling
// (docs/ARCHITECTURE.md §2), so side-by-side layout works only by composing
// already-wrapped plain text. Any other element type returns an error,
// which Build reports as apperr.KindPermanent — accepted and validated by
// receipt.Columns.Validate per the frozen schema (§3) but not yet
// renderable, the same "ahead of implementation" position Text.Align and
// Asset.Align hold.
func columnsLines(c receipt.Columns, widthDots int, f Font) ([]string, error) {
	widths := columnWidths(c.Columns, widthDots, f)

	cellLines := make([][]string, len(c.Columns))
	maxLines := 0
	for i, col := range c.Columns {
		lines, err := columnLines(col, widths[i], f)
		if err != nil {
			return nil, err
		}
		cellLines[i] = lines
		if len(lines) > maxLines {
			maxLines = len(lines)
		}
	}

	lines := make([]string, maxLines)
	for i, col := range cellLines {
		last := i == len(c.Columns)-1
		for j := range lines {
			var cell string
			if j < len(col) {
				cell = col[j]
			}
			if !last {
				cell = padToWidth(cell, widths[i], f) + " "
			}
			lines[j] += cell
		}
	}
	return lines, nil
}

// columnWidths divides widthDots across len(cols) content budgets in
// proportion to each Column's Weight (0 floors to 1 via ResolveSize),
// separated by one-space gaps — sum(widths) plus (len(cols)-1) gaps always
// equals widthDots, the invariant tableColumnWidths establishes. Remainder
// goes to the last column. A non-positive budget floors to len(cols) dots
// rather than "unconstrained": a narrow Profile still constrains, it just
// wraps aggressively.
//
// widthDots <= 0 (Build's "no printer configured" sentinel, see wrapText)
// returns all-zero widths, so columns compose as a single unwrapped line
// per row separated by the gap — columnLines and padToWidth already no-op
// at width <= 0.
func columnWidths(cols []receipt.Column, widthDots int, f Font) []int {
	widths := make([]int, len(cols))
	if widthDots <= 0 {
		return widths
	}

	gap := f.Measure(" ")
	numCols := len(cols)
	budget := widthDots - (numCols-1)*gap
	if budget < numCols {
		budget = numCols
	}

	totalWeight := 0
	weight := make([]int, numCols)
	for i, col := range cols {
		weight[i] = ResolveSize(col.Weight)
		totalWeight += weight[i]
	}

	assigned := 0
	for i, w := range weight {
		widths[i] = budget * w / totalWeight
		assigned += widths[i]
	}
	widths[numCols-1] += budget - assigned
	return widths
}

// columnLines returns col's content as already-wrapped lines, each wrapped
// to width via wrapText, in order. Any non-Text element is reported as an
// error (see columnsLines).
//
// receipt.Heading is rejected rather than downgraded: a composed ColumnsLine
// Block carries one Style for its whole line, so a Heading's Bold/Size
// styling cannot survive being merged with a sibling column's content
// without per-run styling or per-column positioning, both out of scope.
// Painting it at normalStyle would silently change what Heading means.
func columnLines(col receipt.Column, width int, f Font) ([]string, error) {
	var lines []string
	for _, el := range col.Elements {
		text, ok := el.(receipt.Text)
		if !ok {
			return nil, fmt.Errorf("column: unsupported element type %T inside a column", el)
		}
		lines = append(lines, wrapText(text.Content, width, f, 1)...)
	}
	return lines, nil
}
