package layout

import (
	"strings"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

// TableLine is one already-wrapped, column-aligned line of a receipt.Table's
// output — the header row or one data row, split across as many TableLine
// Blocks as wrapping produced (see tableLines). Build produces a distinct
// type, rather than a receipt.Text one, so a Table-derived Block keeps its
// own identity through layout like every other element type: each carries
// its receipt.Element type to render/canvas.Paint, the one place per type
// that decides how it paints (docs/ARCHITECTURE.md §4). Paint paints Content
// through the same glyph-by-glyph path as receipt.Text (canvas.textContent).
type TableLine struct {
	Content string
}

// Validate always succeeds. TableLine is never part of a client-supplied
// receipt.Receipt — it exists only as a Block.Element Build produces — so
// there is nothing to validate; the method exists only to satisfy
// receipt.Element.
func (TableLine) Validate() error { return nil }

// tableLines turns t into the plain-text lines Build lays out as TableLine
// Blocks — the header row then each data row, in order.
//
// widthDots <= 0 (Build's "no printer configured" sentinel, see wrapText)
// skips column derivation: each row's cells are joined with a single space,
// unwrapped.
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

// tableColumnWidths divides widthDots evenly across numCols budgets, each
// separated by a one-space gap: sum(colWidths) plus (numCols-1) gaps equals
// widthDots, so a composed row never exceeds printable width (see
// tableRowLines). Remainder goes to the last column. A non-positive budget
// floors to 1 dot rather than "unconstrained" — a narrow Profile still
// constrains, just wraps aggressively.
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

// tableRowLines wraps each of row's cells to its column budget (wrapText),
// then composes them line by line: non-last columns are right-padded to
// their width plus a one-space gap so the next column always starts at the
// same offset; the last is appended as-is. A cell wrapping to fewer lines
// than the row's tallest contributes blank padding, keeping columns aligned.
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

// padToWidth right-pads s with spaces until one more would exceed width —
// the greedy-fit test wrapParagraph uses, applied to blank space. s already
// wider than width (wrapText never splits a single word wider than its
// column) is returned unchanged.
func padToWidth(s string, width int, f Font) string {
	for f.Measure(s+" ") <= width {
		s += " "
	}
	return s
}
