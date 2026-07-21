package layout

import (
	"fmt"
	"strings"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

// ListLine is one already-wrapped, marker-and-indent-composed line of a
// receipt.List's rendered output — analogous to TableLine and ColumnsLine
// (see TableLine's own doc comment for why Build produces a distinct
// Block-carrying type here rather than reusing receipt.Text: a
// List-derived Block keeps its own identity through layout, the same as
// every other element type). render/canvas.Paint paints a ListLine's
// Content through the exact same glyph-by-glyph path a receipt.Text Block
// already uses (see canvas.textContent) — this is not a second
// text-rendering primitive, just one more case recognized by the existing
// one.
type ListLine struct {
	Content string
}

// Validate always succeeds — see TableLine.Validate's identical doc
// comment: ListLine is never part of a client-supplied receipt.Receipt,
// it exists only as a Block.Element Build itself produces.
func (ListLine) Validate() error { return nil }

// listIndentSpaces is how many literal space characters one
// ListItem.Indent level shifts a line's marker and content by — the
// rendering choice docs/adr/0014-list-elements.md deliberately leaves
// unfixed: Indent is a semantic nesting level, not a count of characters,
// so this constant (and its unit, spaces) may change without that being a
// schema-visible or architectural change. Expressed as literal leading
// content composed onto the line itself, measured via Font like every
// other width decision in this package, rather than as a coordinate on
// Block — see ListLine and listLines.
const listIndentSpaces = 2

// listLines turns l into the plain-text lines Build lays out as ListLine
// Blocks: each item's marker (listMarker) and semantic indent
// (listIndentSpaces per level) composed as leading content, followed by
// its own word-wrapped Content — the same greedy wrapText word-wrap every
// other element in this schema uses — with continuation lines hang-indented
// under the item's content via listHangIndent, not under its marker,
// correctly regardless of a marker's own width (a numbered list's markers
// are not all the same width, e.g. "1." vs. "10.").
//
// widthDots <= 0 (Build's documented "no printer configured" sentinel,
// see wrapText) leaves each item's content unwrapped, the same "no width
// to constrain to" fallback tableLines and columnsLines already use for
// their own content.
//
// An item's available content width narrows with its own marker and
// indent width and floors to 1 dot rather than going non-positive — a
// List nested past the point where any content width remains still
// degrades to the narrowest wrapping this schema already falls back to
// elsewhere (tableColumnWidths, columnWidths), rather than failing.
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

// listMarker returns the leading marker for an item at 1-based position
// number within its List's Items slice, per docs/adr/0014-list-elements.md:
// a hyphen for a bullet item (kind "" or "bullet"), number's own decimal
// digits for a numbered item — flat and sequential across the whole
// Items slice, independent of ListItem.Indent — or "[x]"/"[ ]" for a
// checked/unchecked checkbox item. Every marker is composed entirely from
// glyphs the embedded ASCII font can render
// (docs/adr/0008-embedded-font-legibility.md) and carries its own
// trailing space separating it from the item's content.
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

// listHangIndent returns a blank-space string measuring the same width as
// prefix (per f.Measure), the same "grow one space at a time while within
// budget" technique padToWidth and alignPad already use — so a wrapped
// continuation line lines up under an item's content rather than under
// its marker, exactly regardless of how wide that item's own marker
// happens to be.
func listHangIndent(prefix string, f Font) string {
	budget := f.Measure(prefix)
	s := ""
	for f.Measure(s+" ") <= budget {
		s += " "
	}
	return s
}
