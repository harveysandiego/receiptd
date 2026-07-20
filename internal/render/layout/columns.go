package layout

import (
	"fmt"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

// ColumnsLine is one already-wrapped, column-aligned line of a
// receipt.Columns's rendered output — analogous to TableLine for
// receipt.Table (see TableLine's own doc comment for why Build produces a
// distinct Block-carrying type here rather than reusing receipt.Text: a
// Columns-derived Block keeps its own identity through layout, the same
// as every other element type). render/canvas.Paint paints a
// ColumnsLine's Content through the exact same glyph-by-glyph path a
// receipt.Text Block already uses (see canvas.textContent) — this is not
// a second text-rendering primitive, just one more case recognized by the
// existing one.
type ColumnsLine struct {
	Content string
}

// Validate always succeeds — see TableLine.Validate's identical doc
// comment: ColumnsLine is never part of a client-supplied receipt.Receipt,
// it exists only as a Block.Element Build itself produces.
func (ColumnsLine) Validate() error { return nil }

// columnsLines turns c into the plain-text lines Build lays out as
// ColumnsLine Blocks: each column's own content, word-wrapped to its share
// of widthDots (columnWidths) and composed side by side into full-width
// lines — the same technique receipt.Table's own column layout already
// uses (see tableLines, tableRowLines), generalized from a Table's
// plain-string cells to a Column's own []receipt.Element content.
//
// Only receipt.Text is supported inside a column (see columnLines):
// side-by-side layout in this architecture works by composing
// already-wrapped plain text into a single Block per row, since
// render/layout.Block carries no horizontal position and no per-run
// styling of its own (docs/ARCHITECTURE.md §2) — the same constraint that
// already limits receipt.Table's own cells to plain strings. Any other
// element type inside a column, including receipt.Heading (see
// columnLines's own doc comment for why Heading specifically is rejected
// rather than downgraded), returns an error, which Build reports as
// apperr.KindPermanent: accepted and recursively validated by
// receipt.Columns.Validate() per the frozen schema (docs/ARCHITECTURE.md
// §3), but not yet renderable inside a column — the same "ahead of
// implementation" position Text.Italic/Underline/Strikethrough and
// Asset.Width/Align already hold.
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
// proportion to each Column's own Weight (0 or omitted floors to 1 via
// ResolveSize, the same "unscaled means 1" convention receipt.Text.Size
// and receipt.Divider.Size already use), each separated by a one-space
// gap — sum(widths) plus (len(cols)-1) gaps always equals exactly
// widthDots, the same invariant tableColumnWidths already establishes for
// receipt.Table. Any remainder from the division goes to the last column.
// A budget that would be non-positive floors to len(cols) dots rather than
// falling back to "unconstrained" — a narrow Profile still constrains, it
// just wraps aggressively, the same behaviour tableColumnWidths already
// has.
//
// widthDots <= 0 (Build's documented "no printer configured" sentinel,
// see wrapText) returns all-zero widths: columnLines (via wrapText) and
// padToWidth both already no-op at width <= 0, so columns compose as a
// single unwrapped, unpadded line per row, separated only by the one-space
// gap — the same "no width to constrain to" fallback receipt.Table's own
// tableLines uses.
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

// columnLines returns col's own content as already-wrapped lines, each
// wrapped to width via wrapText — the same greedy word-wrap every
// receipt.Text uses — in the order col.Elements appears, one or more
// output lines per receipt.Text element. Any other element type is
// reported as an error rather than silently skipped or given placeholder
// content (see columnsLines).
//
// receipt.Heading is deliberately not supported here, even though it is
// content Build otherwise knows how to lay out: a composed ColumnsLine
// Block carries exactly one Style for its whole line (columnsLines merges
// one line from every column into a single Block), so a Heading's
// Bold/Size styling (headingStyle) cannot be preserved once its line is
// merged with a sibling column's own — possibly plain-Text — content on
// the same row, without either per-run styling within a Block or per-
// column horizontal positioning, both new rendering primitives out of
// scope here. Silently painting a Heading at normalStyle instead would
// change what Heading means without saying so, so Build rejects it the
// same way any other unsupported nested element type already is, rather
// than downgrading it.
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
