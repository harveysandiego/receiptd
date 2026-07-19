package canvas

import (
	"fmt"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// Paint renders doc's Blocks onto a Canvas using doc.Font, in Block
// order. receipt.Text and receipt.Heading each occupy one line of height
// f.LineHeight(), starting at their Y, and paint their Content as glyphs.
// receipt.Spacer occupies its own Height (dots) of blank space and paints
// nothing. Any other element type returns apperr.KindPermanent rather
// than being skipped or given placeholder pixels. A Heading paints
// identically to a Text with the same Content: see render/layout.Build's
// docstring for why its documented "bold + large" styling isn't applied
// yet.
//
// The Canvas is sized to doc.WidthDots when it's positive (the printer
// width render/layout.Build resolved the Document against) — content
// narrower than that still produces a full-width Canvas, and content
// wider than it is clipped rather than expanding the Canvas or wrapping
// onto another line (see paintGlyph). When doc.WidthDots is zero — a
// Document Build produced with no printer.Profile, or one hand-built by
// a caller that never set it — the Canvas falls back to exactly fitting
// its painted content, this package's original behavior. Height is
// always computed from content: a printer.Profile carries no notion of
// paper length for Paint to target instead (see Document's own doc
// comment).
//
// Paint assumes doc.Font is set, same as it assumes doc came from Build
// rather than being hand-built: a Document is always produced with its
// Font, so there's nothing to validate here (see docs/ARCHITECTURE.md §5
// on not blurring Validate-style checks into stages that can trust their
// input).
func Paint(doc layout.Document) (*Canvas, error) {
	f := doc.Font
	width, height := doc.WidthDots, 0
	for _, b := range doc.Blocks {
		bh, ok := blockHeight(b.Element, f)
		if !ok {
			return nil, apperr.Wrap(apperr.KindPermanent, "canvas.Paint", fmt.Errorf("unsupported element type %T", b.Element))
		}
		if width <= 0 {
			if content, ok := textContent(b.Element); ok {
				if w := f.Measure(content); w > width {
					width = w
				}
			}
		}
		if bottom := b.Y + bh; bottom > height {
			height = bottom
		}
	}

	c := &Canvas{
		Width:  width,
		Height: height,
		Bits:   make([]byte, ((width+7)/8)*height),
	}

	for _, b := range doc.Blocks {
		content, ok := textContent(b.Element)
		if !ok {
			continue // e.g. receipt.Spacer: blank space, no glyphs to paint
		}
		x := 0
		for _, r := range content {
			bmp, advance := f.Glyph(r)
			c.paintGlyph(x, b.Y, bmp)
			x += advance
		}
	}

	return c, nil
}

// textContent returns el's text content if el is a receipt.Text or
// receipt.Heading, the only two element types Paint paints glyphs for.
func textContent(el receipt.Element) (string, bool) {
	switch e := el.(type) {
	case receipt.Text:
		return e.Content, true
	case receipt.Heading:
		return e.Content, true
	default:
		return "", false
	}
}

// blockHeight returns el's vertical extent in dots if el is a supported
// element type: f.LineHeight() for receipt.Text and receipt.Heading, or
// its own Height for receipt.Spacer.
func blockHeight(el receipt.Element, f layout.Font) (int, bool) {
	switch e := el.(type) {
	case receipt.Text, receipt.Heading:
		return f.LineHeight(), true
	case receipt.Spacer:
		return e.Height, true
	default:
		return 0, false
	}
}

// paintGlyph copies bmp's set pixels into c, offset by (x, y), silently
// dropping any pixel that falls outside c's bounds rather than growing c
// or wrapping onto another line. Before doc.WidthDots existed (Paint
// always sized the Canvas to fit its content), that could never happen;
// a Document built against a printer.Profile can now specify a width
// narrower than some painted line's content, so this guard is load-bearing,
// not defensive filler — without it, x or y could index past the end of
// c.Bits.
func (c *Canvas) paintGlyph(x, y int, bmp layout.GlyphBitmap) {
	rowBytes := (c.Width + 7) / 8
	srcRowBytes := (bmp.Width + 7) / 8
	for row := 0; row < bmp.Height; row++ {
		for col := 0; col < bmp.Width; col++ {
			if bmp.Bits[row*srcRowBytes+col/8]&(0x80>>uint(col%8)) == 0 {
				continue
			}
			px, py := x+col, y+row
			if px >= c.Width || py >= c.Height {
				continue
			}
			c.Bits[py*rowBytes+px/8] |= 0x80 >> uint(px%8)
		}
	}
}
