package canvas

import (
	"fmt"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// rasterBitmap resolves el's bitmap (scaled/generated to fit maxWidth) if
// el is a receipt.Image, receipt.QRCode, receipt.Barcode, or
// layout.AlignedAsset — the element types Paint treats as "just another
// image" (docs/ARCHITECTURE.md §4), all painted through the one paintGlyph
// primitive with no element-specific drawing. ok is false for any other
// Element, with bmp and err both zero. An AlignedAsset arrives already
// left-padded for "center"/"right" (docs/adr/0013-text-and-asset-alignment.md),
// so Paint blits it at x=0 like any other raster Block.
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
	case layout.AlignedAsset:
		bmp, err = layout.DecodeAlignedAssetBitmap(e, maxWidth)
		if err != nil {
			err = fmt.Errorf("asset: %w", err)
		}
		return bmp, true, err
	default:
		return layout.GlyphBitmap{}, false, nil
	}
}

// isRasterElement reports whether el is one of the element types
// rasterBitmap resolves a bitmap for. Paint's painting pass uses it to
// reuse the already-resolved bitmap rather than re-running the
// (CPU-bound, for QRCode/Barcode) resolution a second time.
func isRasterElement(el receipt.Element) bool {
	switch el.(type) {
	case receipt.Image, receipt.QRCode, receipt.Barcode, layout.AlignedAsset:
		return true
	default:
		return false
	}
}

// Paint renders doc's Blocks onto a Canvas using doc.Font, in Block order.
// There is exactly one rendering pipeline, not a per-type one
// (docs/ARCHITECTURE.md §4):
//
//   - Text-bearing Blocks (receipt.Text, receipt.Heading, layout.TableLine,
//     layout.ColumnsLine, layout.ListLine, layout.BarcodeCaption) occupy one
//     f.LineHeight()*b.Style.Size line and paint their Content as glyphs
//     styled per b.Style (styleGlyph), then any underline/strikethrough
//     (paintDecorations, layered on after glyphs, never folded into a glyph).
//   - Raster Blocks (receipt.Image, receipt.QRCode, receipt.Barcode,
//     layout.AlignedAsset) are resolved by rasterBitmap and painted through
//     the same paintGlyph primitive.
//   - receipt.Spacer occupies its own Height and paints nothing;
//     receipt.Divider paints via paintDivider.
//   - receipt.Feed and receipt.Cut paint nothing and become one
//     Canvas.Controls entry each
//     (docs/adr/0010-printer-control-elements-via-canvas-controls.md).
//   - Any other element type returns apperr.KindPermanent.
//
// Divider and raster Blocks don't read b.Style; the type switches below
// (textContent, blockHeight) read only structural data the frozen
// receipt.Element interface doesn't expose generically, never styling.
//
// The Canvas is sized to doc.WidthDots when positive: narrower content
// still produces a full-width Canvas, wider content is clipped rather than
// expanding or wrapping (see paintGlyph). When doc.WidthDots is zero (a
// Document built with no printer.Profile, or hand-built) the Canvas falls
// back to fitting its content. Height is always computed from content.
//
// Paint assumes doc.Font is set and every Style.Size is >= 1: a Document
// always comes from Build fully resolved, so there is nothing to validate
// here (docs/ARCHITECTURE.md §5).
func Paint(doc layout.Document) (*Canvas, error) {
	f := doc.Font
	width, height := doc.WidthDots, 0
	contentFit := width <= 0         // capture before width becomes the running content-fit max below.
	var bitmaps []layout.GlyphBitmap // lazily allocated: most Documents paint no raster Blocks
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
		if d, ok := b.Element.(receipt.Divider); ok {
			c.paintDivider(d, b.Y)
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

// textContent returns el's text content if el is one of the element types
// Paint paints glyphs for (receipt.Text, receipt.Heading, and the
// already-composed, already-wrapped output lines layout.TableLine,
// layout.ColumnsLine, layout.ListLine, layout.BarcodeCaption). Paint reads
// their Content exactly like any other line of text.
func textContent(el receipt.Element) (string, bool) {
	switch e := el.(type) {
	case receipt.Text:
		return e.Content, true
	case receipt.Heading:
		return e.Content, true
	case layout.TableLine:
		return e.Content, true
	case layout.ColumnsLine:
		return e.Content, true
	case layout.ListLine:
		return e.Content, true
	case layout.BarcodeCaption:
		return e.Content, true
	default:
		return "", false
	}
}

// blockHeight returns b's vertical extent in dots if its Element is a
// supported type: f.LineHeight()*b.Style.Size for the text-bearing types,
// the Spacer's own Height, layout.DividerThickness*resolved Size for a
// Divider, or 0 for Feed/Cut. The Divider computation deliberately reuses
// the same layout.ResolveSize and DividerThickness layout.Build advanced Y
// by, so the two stages can never disagree about a Divider's height.
func blockHeight(b layout.Block, f layout.Font) (int, bool) {
	switch e := b.Element.(type) {
	case receipt.Text, receipt.Heading, layout.TableLine, layout.ColumnsLine, layout.ListLine, layout.BarcodeCaption:
		return f.LineHeight() * b.Style.Size, true
	case receipt.Spacer:
		return e.Height, true
	case receipt.Divider:
		return layout.DividerThickness * layout.ResolveSize(e.Size), true
	case receipt.Feed, receipt.Cut:
		return 0, true
	default:
		return 0, false
	}
}

// paintDecorations draws b's underline and/or strikethrough, if styled,
// across [0, textWidth). Unlike Bold/Italic (styleGlyph), these are
// layered on after glyphs rather than folded into a glyph's bitmap
// (docs/ARCHITECTURE.md §3): a decoration's shape depends only on b's line
// box, not on the glyphs under it. Both are positioned and sized from b's
// resolved line height (blockHeight), so they scale with Style.Size
// without Font needing a baseline or x-height concept it doesn't have.
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

// dashDots and dashGapDots are a dashed receipt.Divider's repeating
// on/off pattern, in dots: wide enough to read as distinct segments at
// typical thermal-printer resolution rather than blurring into a solid or
// near-invisible line, the "legible by default" goal of
// docs/adr/0011-divider-thickness-legibility.md.
const (
	dashDots    = 16
	dashGapDots = 8
)

// paintDivider paints one receipt.Divider Block at y: a solid full-width
// line for Style "" or "solid", or a dashDots-on/dashGapDots-off pattern
// for "dashed" — the only values Validate accepts, so no default case is
// needed. Both share the same thickness (layout.DividerThickness times d's
// resolved Size); Style changes only the horizontal pattern.
func (c *Canvas) paintDivider(d receipt.Divider, y int) {
	thickness := layout.DividerThickness * layout.ResolveSize(d.Size)
	if d.Style != "dashed" {
		c.paintHLine(0, c.Width, y, thickness)
		return
	}
	for x := 0; x < c.Width; x += dashDots + dashGapDots {
		end := x + dashDots
		if end > c.Width {
			end = c.Width
		}
		c.paintHLine(x, end, y, thickness)
	}
}

// paintHLine sets every pixel in the horizontal band [x0, x1) x
// [y0, y0+thickness) on c, silently dropping anything out of bounds — same
// clipping as paintGlyph, and for the same reason.
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
// dropping any pixel outside c's bounds rather than growing c or wrapping.
// The bounds guard is load-bearing, not defensive filler: a Document built
// against a printer.Profile can specify a width narrower than a painted
// line's content, so without it px/py could index past the end of c.Bits.
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
