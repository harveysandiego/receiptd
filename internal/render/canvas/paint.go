package canvas

import (
	"fmt"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// rasterBitmap resolves el's decoded/generated bitmap if el is a
// receipt.Image, receipt.QRCode, or receipt.Barcode — the element types
// Paint treats as "just another image" (docs/ARCHITECTURE.md §4):
// scaled/generated to fit maxWidth and converted to the same
// layout.GlyphBitmap representation glyphs already use, so all three
// paint through the exact same paintGlyph primitive with no
// element-specific drawing logic of their own. ok is false for any other
// Element, in which case bmp and err are both zero. A receipt.QRCode's or
// receipt.Barcode's bitmap is generated fresh here
// (render/layout.GenerateQRCodeBitmap, render/layout.GenerateBarcodeBitmap),
// the render-time analogue of decoding a receipt.Image's bytes — Paint
// never distinguishes a generated bitmap from a decoded one once it has
// one.
func rasterBitmap(el receipt.Element, maxWidth int) (bmp layout.GlyphBitmap, ok bool, err error) {
	switch e := el.(type) {
	case receipt.Image:
		bmp, err = layout.DecodeImageBitmap(e.Data, maxWidth)
		if err != nil {
			err = fmt.Errorf("image: %w", err)
		}
		return bmp, true, err
	case receipt.QRCode:
		bmp, err = layout.GenerateQRCodeBitmap(e, maxWidth)
		if err != nil {
			err = fmt.Errorf("qrcode: %w", err)
		}
		return bmp, true, err
	case receipt.Barcode:
		bmp, err = layout.GenerateBarcodeBitmap(e, maxWidth)
		if err != nil {
			err = fmt.Errorf("barcode: %w", err)
		}
		return bmp, true, err
	default:
		return layout.GlyphBitmap{}, false, nil
	}
}

// isRasterElement reports whether el is one of the element types
// rasterBitmap resolves a bitmap for — used by Paint's painting pass to
// decide whether a Block's already-resolved bitmap (see rasterBitmap)
// should be painted via paintGlyph, without re-running the (potentially
// expensive, in QRCode's and Barcode's case CPU-bound) resolution itself
// a second time.
func isRasterElement(el receipt.Element) bool {
	switch el.(type) {
	case receipt.Image, receipt.QRCode, receipt.Barcode:
		return true
	default:
		return false
	}
}

// Paint renders doc's Blocks onto a Canvas using doc.Font, in Block
// order. receipt.Text, receipt.Heading, and layout.TableLine (one already
// wrapped, column-aligned line of a receipt.Table's output — see
// render/layout.Build) each occupy one line of height f.LineHeight() *
// b.Style.Size, starting at their Y, and paint their Content as glyphs
// styled per b.Style (see styleGlyph), followed by any
// underline/strikethrough decoration (see paintDecorations) — decorations
// are drawn onto the Canvas after glyph painting, never folded into a
// glyph's own bitmap. receipt.Spacer occupies its own Height (dots) of
// blank space and paints nothing. receipt.Divider occupies
// layout.DividerThickness dots and paints one horizontal line spanning
// the Canvas's full width (paintHLine, the same primitive underline and
// strikethrough already reuse) — it is not part of the text-styling
// pipeline, so b.Style is not read for it. receipt.Image, receipt.QRCode,
// and receipt.Barcode are all resolved to a layout.GlyphBitmap (scaled to
// fit doc.WidthDots) by rasterBitmap — decoded via
// layout.DecodeImageBitmap for Image, generated via
// layout.GenerateQRCodeBitmap for QRCode and layout.GenerateBarcodeBitmap
// for Barcode — then painted with the same paintGlyph primitive text
// glyphs use: there is exactly one bitmap-painting path, not a parallel
// one per raster element type (docs/ARCHITECTURE.md §4); like Divider,
// b.Style is not read for any of them. receipt.Feed and receipt.Cut paint
// nothing and occupy zero height; each becomes one Canvas.Controls entry
// instead (docs/adr/0010-printer-control-elements-via-canvas-controls.md).
// Any other element type returns apperr.KindPermanent rather than being
// skipped or given placeholder pixels.
//
// Paint never inspects receipt.Text/receipt.Heading fields to decide how
// to style a Block — only Block.Style, already fully resolved by
// render/layout.Build (docs/ARCHITECTURE.md §3 "Text styling"). This is
// what makes a receipt.Heading or layout.TableLine Block render
// identically to a receipt.Text Block given the same Style: there is
// exactly one rendering pipeline, not a per-type one. The type switches
// below (textContent, blockHeight) exist only to read structural data the
// frozen receipt.Element interface doesn't expose generically — a
// Text/Heading/TableLine's Content, a Spacer's own Height — never to
// branch on styling.
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
// Paint assumes doc.Font is set and every Block's Style.Size is >= 1,
// same as it assumes doc came from Build rather than being hand-built: a
// Document is always produced with its Font and fully resolved Styles, so
// there's nothing to validate here (see docs/ARCHITECTURE.md §5 on not
// blurring Validate-style checks into stages that can trust their input).
func Paint(doc layout.Document) (*Canvas, error) {
	f := doc.Font
	width, height := doc.WidthDots, 0
	contentFit := width <= 0         // doc.WidthDots itself never changes; capture this once, before width becomes the running content-fit max below.
	var bitmaps []layout.GlyphBitmap // lazily allocated: most Documents paint no raster (Image/QRCode) Blocks at all
	for i, b := range doc.Blocks {
		var bh int
		if bmp, ok, err := rasterBitmap(b.Element, doc.WidthDots); ok {
			if err != nil {
				return nil, apperr.Wrap(apperr.KindPermanent, "canvas.Paint", err)
			}
			if bitmaps == nil {
				bitmaps = make([]layout.GlyphBitmap, len(doc.Blocks))
			}
			bitmaps[i] = bmp
			bh = bmp.Height
			if contentFit && bmp.Width > width {
				width = bmp.Width
			}
		} else {
			var ok bool
			bh, ok = blockHeight(b, f)
			if !ok {
				return nil, apperr.Wrap(apperr.KindPermanent, "canvas.Paint", fmt.Errorf("unsupported element type %T", b.Element))
			}
			if contentFit {
				if content, ok := textContent(b.Element); ok {
					if w := f.Measure(content) * b.Style.Size; w > width {
						width = w
					}
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

	for i, b := range doc.Blocks {
		switch b.Element.(type) {
		case receipt.Feed, receipt.Cut:
			c.Controls = append(c.Controls, Control{Y: b.Y, Element: b.Element, Terminal: i == len(doc.Blocks)-1})
			continue
		}
		if _, ok := b.Element.(receipt.Divider); ok {
			c.paintHLine(0, c.Width, b.Y, layout.DividerThickness)
			continue
		}
		if isRasterElement(b.Element) {
			c.paintGlyph(0, b.Y, bitmaps[i])
			continue
		}
		content, ok := textContent(b.Element)
		if !ok {
			continue // e.g. receipt.Spacer: blank space, no glyphs to paint
		}
		x := 0
		for _, r := range content {
			bmp, advance := f.Glyph(r)
			c.paintGlyph(x, b.Y, styleGlyph(bmp, b.Style))
			x += advance * b.Style.Size
		}
		c.paintDecorations(b, f, x)
	}

	return c, nil
}

// textContent returns el's text content if el is a receipt.Text,
// receipt.Heading, or layout.TableLine — the element types Paint paints
// glyphs for via the same code path (see Paint's painting loop). A
// TableLine is a receipt.Table's already-composed, already-wrapped output
// line (see render/layout.Build and layout.TableLine's own doc comment):
// Paint reads its Content exactly like any other line of text, never
// anything Table-specific.
func textContent(el receipt.Element) (string, bool) {
	switch e := el.(type) {
	case receipt.Text:
		return e.Content, true
	case receipt.Heading:
		return e.Content, true
	case layout.TableLine:
		return e.Content, true
	default:
		return "", false
	}
}

// blockHeight returns b's vertical extent in dots if its Element is a
// supported type: f.LineHeight() * b.Style.Size for receipt.Text,
// receipt.Heading, and layout.TableLine alike (the same Style.Size used to
// scale their glyphs — see Paint; a TableLine's Style is always
// layout.Build's normalStyle, Size 1), the Spacer's own Height (unaffected
// by Style), layout.DividerThickness for a receipt.Divider — the same
// constant layout.Build already advanced Y by, so the two stages can never
// disagree about how tall a Divider Block is — or 0 for a receipt.Feed or
// receipt.Cut, which layout.Build never advances Y for either.
func blockHeight(b layout.Block, f layout.Font) (int, bool) {
	switch e := b.Element.(type) {
	case receipt.Text, receipt.Heading, layout.TableLine:
		return f.LineHeight() * b.Style.Size, true
	case receipt.Spacer:
		return e.Height, true
	case receipt.Divider:
		return layout.DividerThickness, true
	case receipt.Feed, receipt.Cut:
		return 0, true
	default:
		return 0, false
	}
}

// paintDecorations draws b's underline and/or strikethrough, if styled,
// directly onto c across [0, textWidth) — the width of the content
// paintGlyph already painted for b. Unlike Bold/Italic (styleGlyph),
// these are decorations layered on after the glyph bitmaps are painted,
// never folded into the glyph bitmap itself (docs/ARCHITECTURE.md §3
// "Text styling"): a decoration's shape doesn't depend on what glyphs
// happen to be under it, only on b's line box, so there's nothing for
// styleGlyph's per-glyph pipeline to do here.
//
// Both lines are positioned and sized (thickness) from b's own resolved
// line height (blockHeight, the same f.LineHeight() * b.Style.Size
// every other placement decision in this file uses), so they stay
// correctly placed and scale naturally as Style.Size grows without Font
// needing to expose a baseline or x-height concept it doesn't have.
func (c *Canvas) paintDecorations(b layout.Block, f layout.Font, textWidth int) {
	if textWidth <= 0 || (!b.Style.Underline && !b.Style.Strikethrough) {
		return
	}
	lineHeight, _ := blockHeight(b, f)
	thickness := b.Style.Size
	if b.Style.Underline {
		c.paintHLine(0, textWidth, b.Y+lineHeight-thickness, thickness)
	}
	if b.Style.Strikethrough {
		c.paintHLine(0, textWidth, b.Y+lineHeight/2, thickness)
	}
}

// paintHLine sets every pixel in the horizontal band [x0, x1) x
// [y0, y0+thickness) on c, silently dropping anything outside c's
// bounds — the same clipping behaviour as paintGlyph, and for the same
// reason: a Document built against a printer.Profile can specify a
// width narrower than a decorated line's content.
func (c *Canvas) paintHLine(x0, x1, y0, thickness int) {
	rowBytes := (c.Width + 7) / 8
	for y := y0; y < y0+thickness; y++ {
		if y < 0 || y >= c.Height {
			continue
		}
		for x := x0; x < x1; x++ {
			if x < 0 || x >= c.Width {
				continue
			}
			c.Bits[y*rowBytes+x/8] |= 0x80 >> uint(x%8)
		}
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
