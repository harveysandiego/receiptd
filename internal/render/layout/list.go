package layout

import (
	"fmt"
	"strings"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

// ListLine is one already-wrapped, marker-and-indent-composed line of a
// receipt.List's output — see TableLine for why Build produces a distinct
// Block-carrying type. render/canvas.Paint paints its Content through the
// same glyph-by-glyph path as receipt.Text (canvas.textContent).
type ListLine struct {
	Content string
}

// Validate always succeeds — see TableLine.Validate's identical doc
// comment: ListLine is never part of a client-supplied receipt.Receipt,
// it exists only as a Block.Element Build itself produces.
func (ListLine) Validate() error { return nil }

// listIndentSpaces is how many spaces one ListItem.Indent level shifts a
// line by — a rendering choice docs/adr/0014-list-elements.md leaves
// unfixed: Indent is a semantic nesting level, so this constant (and its
// unit) may change without being a schema or architectural change.
// Expressed as leading content measured via Font, not a Block coordinate —
// see ListLine and listLines.
const listIndentSpaces = 2

// listLines turns l into the plain-text lines Build lays out as ListLine
// Blocks: each item's marker (listMarker) and indent (listIndentSpaces per
// level) as leading content, then its word-wrapped Content, with
// continuation lines hang-indented under the content via listHangIndent —
// not under the marker, so it stays correct for variable-width markers
// (e.g. "1." vs "10.").
//
// widthDots <= 0 (Build's "no printer configured" sentinel, see wrapText)
// leaves content unwrapped, the same fallback tableLines and columnsLines
// use. An item's content width floors to 1 dot rather than going
// non-positive, so a deeply nested List degrades to narrow wrapping rather
// than failing.
func listLines(l receipt.List, widthDots int, f Font) []string {
	var lines []string
	for i, item := range l.Items {
		prefix := strings.Repeat(" ", item.Indent*listIndentSpaces) + listMarker(l.Kind, i+1, item)

		contentWidth := 0
		if widthDots > 0 {
			contentWidth = widthDots - f.Measure(prefix)
			if contentWidth < 1 {
				contentWidth = 1
			}
		}

		hang := listHangIndent(prefix, f)
		for j, line := range wrapText(item.Content, contentWidth, f, 1) {
			if j == 0 {
				lines = append(lines, prefix+line)
			} else {
				lines = append(lines, hang+line)
			}
		}
	}
	return lines
}

// listMarker returns the leading marker for the item at 1-based position
// number, per docs/adr/0014-list-elements.md: a hyphen for a bullet (kind
// "" or "bullet"), decimal digits for "number" (flat and sequential across
// Items, independent of Indent), or "[x]"/"[ ]" for a checkbox. Every
// marker is renderable by the embedded ASCII font and carries a trailing
// space.
func listMarker(kind string, number int, item receipt.ListItem) string {
	switch kind {
	case "number":
		return fmt.Sprintf("%d. ", number)
	case "checkbox":
		if item.Checked {
			return "[x] "
		}
		return "[ ] "
	default: // "" or "bullet"
		return "- "
	}
}

// listHangIndent returns spaces measuring the same width as prefix (per
// f.Measure), grown one space at a time as padToWidth does, so a wrapped
// continuation line lines up under the item's content regardless of the
// marker's width.
func listHangIndent(prefix string, f Font) string {
	budget := f.Measure(prefix)
	s := ""
	for f.Measure(s+" ") <= budget {
		s += " "
	}
	return s
}
