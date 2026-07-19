package layout

import (
	"strings"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

// TableLine is one already-wrapped, column-aligned line of a
// receipt.Table's rendered output — the header row, or one data row,
// split across as many TableLine Blocks as wrapping produced (see
// tableLines). Build produces a TableLine Block rather than a
// receipt.Text one so a Table-derived Block keeps its own identity
// through layout, the same as every other element type: Text, Heading,
// Image, QRCode, Barcode, Divider, Feed, and Cut Blocks all carry their
// original receipt.Element type to render/canvas.Paint, which is the one
// place, per element type, that decides how it paints
// (docs/ARCHITECTURE.md §4). render/canvas.Paint paints TableLine's
// Content through the exact same glyph-by-glyph path a receipt.Text
// Block already uses (see canvas.textContent) — this is not a second
// text-rendering primitive, just one more case recognized by the
// existing one, the same way receipt.Heading already is.
type TableLine struct {
	Content string
}

// Validate always succeeds. TableLine is never part of a client-supplied
// receipt.Receipt — it exists only as a Block.Element Build itself
// produces — so there is nothing to validate; this method exists solely
// so TableLine satisfies receipt.Element and can be placed in Block.Element
// like any other element type.
func (TableLine) Validate() error { return nil }

// tableLines turns t into the plain-text lines Build lays out as
// TableLine Blocks — the header row followed by each data row, in order.
//
// widthDots <= 0 (Build's documented "no printer configured" sentinel,
// see wrapText) skips column derivation entirely: each row's cells are
// simply joined with a single space, unwrapped — the same "no width to
// constrain to" fallback wrapText itself uses.
func tableLines(t receipt.Table, widthDots int, f Font) []string {
	rows := make([][]string, 0, len(t.Rows)+1)
	rows = append(rows, t.Headers)
	rows = append(rows, t.Rows...)

	if widthDots <= 0 {
		lines := make([]string, len(rows))
		for i, row := range rows {
			lines[i] = strings.Join(row, " ")
		}
		return lines
	}

	colWidths := tableColumnWidths(len(t.Headers), widthDots, f)
	var lines []string
	for _, row := range rows {
		lines = append(lines, tableRowLines(row, colWidths, f)...)
	}
	return lines
}

// tableColumnWidths divides widthDots evenly across numCols content
// budgets, each separated by a one-space gap: sum(colWidths) plus
// (numCols-1) gaps always equals exactly widthDots, so a composed row
// never exceeds the printable width (see tableRowLines). Any remainder
// from the division goes to the last column. A budget that would be
// non-positive floors to 1 dot rather than falling back to "unconstrained"
// — a narrow Profile still constrains, it just wraps aggressively.
func tableColumnWidths(numCols, widthDots int, f Font) []int {
	gap := f.Measure(" ")
	budget := widthDots - (numCols-1)*gap
	if budget < numCols {
		budget = numCols
	}

	widths := make([]int, numCols)
	base := budget / numCols
	for i := range widths {
		widths[i] = base
	}
	widths[numCols-1] += budget % numCols
	return widths
}

// tableRowLines wraps each of row's cells to its own column budget (via
// the same wrapText greedy word-wrap every receipt.Text uses), then
// composes them line by line: non-last columns are right-padded with
// spaces to their full column width plus a one-space gap, so the next
// column always starts at the same dot offset regardless of content
// length; the last column is appended as-is, with no trailing padding. A
// cell that wraps to fewer lines than the row's tallest cell contributes
// blank padding on its missing lines, so every column stays aligned for
// the whole row.
func tableRowLines(row []string, colWidths []int, f Font) []string {
	cellLines := make([][]string, len(row))
	maxLines := 0
	for i, cell := range row {
		cellLines[i] = wrapText(cell, colWidths[i], f, 1)
		if len(cellLines[i]) > maxLines {
			maxLines = len(cellLines[i])
		}
	}

	lines := make([]string, maxLines)
	for i, col := range cellLines {
		last := i == len(row)-1
		for j := range lines {
			var cell string
			if j < len(col) {
				cell = col[j]
			}
			if !last {
				cell = padToWidth(cell, colWidths[i], f) + " "
			}
			lines[j] += cell
		}
	}
	return lines
}

// padToWidth right-pads s with single spaces until one more would measure
// wider than width — the same greedy-fit test wrapParagraph already uses
// for deciding whether a word fits, applied here to deciding how much
// blank space fits instead. s already wider than width (wrapText never
// splits a single word wider than its column, see wrapText's doc comment)
// is returned unchanged.
func padToWidth(s string, width int, f Font) string {
	for f.Measure(s+" ") <= width {
		s += " "
	}
	return s
}
