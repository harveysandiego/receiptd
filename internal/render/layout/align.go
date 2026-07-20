package layout

// alignLeft, alignCenter, and alignRight are the closed Align vocabulary
// receipt.Text.Align and receipt.Asset.Align both validate against
// (docs/adr/0013-text-and-asset-alignment.md) — shared by alignPad and
// alignBitmap, and by Build's own barcode-caption call site, so the
// three values are spelled once rather than repeated as string literals
// throughout render/layout.
const (
	alignLeft   = "left"
	alignCenter = "center"
	alignRight  = "right"
)

// alignPad left-pads content with as many leading spaces as fit within
// the horizontal space align implies, so that content — painted by the
// ordinary text-glyph path starting at x=0 — reads as positioned within
// width dots: the same "pad the content itself, since there is no
// horizontal-position primitive anywhere in Document/Block/Canvas"
// technique tableRowLines/padToWidth and columnsLines already use for
// their own trailing padding, generalized to also pad leading space and
// to a shared align vocabulary (docs/adr/0013-text-and-asset-alignment.md).
//
//   - align is "" or "left": content is returned unchanged — the fast
//     path, and today's implicit behavior for every existing Receipt.
//   - align is "center": left-padded with as many leading spaces as fit
//     within half of (width - f.Measure(content)*size).
//   - align is "right": left-padded with as many leading spaces as fit
//     within the full (width - f.Measure(content)*size).
//
// width <= 0 (Build's documented "no printer configured" sentinel, see
// wrapText) or content already as wide as or wider than width is returned
// unchanged — the same fallback padToWidth/the original
// centerBarcodeCaption already applied.
//
// This is space-padded alignment against the embedded font's own fixed
// glyph advance (every rune measures the same width — see
// docs/adr/0008-embedded-font-legibility.md), not a general,
// font-independent text-alignment primitive: the padding is only ever a
// whole number of space-glyph widths, so the result can be off by up to
// half a glyph's advance from true geometric alignment. A future
// proportional-font Font implementation would need this space-counting
// loop revisited (docs/ARCHITECTURE.md §8).
func alignPad(content, align string, width int, f Font, size int) string {
	if align != alignCenter && align != alignRight {
		return content
	}
	contentWidth := f.Measure(content) * size
	if width <= 0 || contentWidth >= width {
		return content
	}
	avail := width - contentWidth
	if align == alignCenter {
		avail /= 2
	}
	prefix := ""
	for f.Measure(prefix+" ")*size <= avail {
		prefix += " "
	}
	return prefix + content
}
