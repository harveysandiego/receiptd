package layout

// alignLeft, alignCenter, and alignRight are the closed Align vocabulary
// receipt.Text.Align and receipt.Asset.Align validate against
// (docs/adr/0013-text-and-asset-alignment.md), shared by alignPad,
// alignBitmap, and Build's barcode-caption call site so the values are
// spelled once.
const (
	alignLeft   = "left"
	alignCenter = "center"
	alignRight  = "right"
)

// alignPad left-pads content with as many leading spaces as fit within the
// space align implies, so content — painted from x=0 by the ordinary
// text-glyph path — reads as positioned within width dots: the "pad the
// content itself, since there is no horizontal-position primitive"
// technique tableRowLines/padToWidth use, generalized to leading space and
// a shared align vocabulary (docs/adr/0013-text-and-asset-alignment.md).
// "center" pads within half of (width - content width), "right" within all
// of it; "left"/"" returns content unchanged.
//
// width <= 0 (Build's "no printer configured" sentinel, see wrapText) or
// content already >= width returns content unchanged.
//
// This pads against the embedded font's fixed glyph advance (every rune the
// same width — docs/adr/0008-embedded-font-legibility.md), not a
// font-independent alignment primitive: padding is a whole number of space
// widths, so the result can be off by up to half a glyph's advance. A future
// proportional-font Font would need this loop revisited (docs/ARCHITECTURE.md §8).
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
