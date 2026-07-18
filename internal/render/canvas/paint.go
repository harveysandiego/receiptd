package canvas

import (
	"fmt"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// Paint renders doc's Blocks onto a Canvas using doc.Font, in Block
// order. Each Block occupies one line of height f.LineHeight(), starting
// at its Y; the Canvas is sized to exactly fit the painted content. Only
// receipt.Text and receipt.Heading elements are supported — any other
// element type returns apperr.KindPermanent rather than being skipped or
// given placeholder pixels. A Heading paints identically to a Text with
// the same Content: see render/layout.Build's docstring for why its
// documented "bold + large" styling isn't applied yet.
//
// Paint assumes doc.Font is set, same as it assumes doc came from Build
// rather than being hand-built: a Document is always produced with its
// Font, so there's nothing to validate here (see docs/ARCHITECTURE.md §5
// on not blurring Validate-style checks into stages that can trust their
// input).
//
// This is an early, partial implementation of the Paint described in
// docs/ARCHITECTURE.md §2 — it does not yet accept a printer.Profile, and
// sizes the Canvas to fit the painted content rather than to
// Document.WidthDots x HeightDots, since Document does not yet carry
// those fields (the same gap noted in render/layout.Build's docstring).
func Paint(doc layout.Document) (*Canvas, error) {
	f := doc.Font
	width, height := 0, 0
	for _, b := range doc.Blocks {
		content, ok := textContent(b.Element)
		if !ok {
			return nil, apperr.Wrap(apperr.KindPermanent, "canvas.Paint", fmt.Errorf("unsupported element type %T", b.Element))
		}
		if w := f.Measure(content); w > width {
			width = w
		}
		if bottom := b.Y + f.LineHeight(); bottom > height {
			height = bottom
		}
	}

	c := &Canvas{
		Width:  width,
		Height: height,
		Bits:   make([]byte, ((width+7)/8)*height),
	}

	for _, b := range doc.Blocks {
		content, _ := textContent(b.Element) // already validated above
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
// receipt.Heading, the only two element types Paint currently supports.
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

// paintGlyph copies bmp's set pixels into c, offset by (x, y).
func (c *Canvas) paintGlyph(x, y int, bmp layout.GlyphBitmap) {
	rowBytes := (c.Width + 7) / 8
	srcRowBytes := (bmp.Width + 7) / 8
	for row := 0; row < bmp.Height; row++ {
		for col := 0; col < bmp.Width; col++ {
			if bmp.Bits[row*srcRowBytes+col/8]&(0x80>>uint(col%8)) == 0 {
				continue
			}
			px, py := x+col, y+row
			c.Bits[py*rowBytes+px/8] |= 0x80 >> uint(px%8)
		}
	}
}
