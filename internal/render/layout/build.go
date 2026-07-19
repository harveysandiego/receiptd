package layout

import (
	"fmt"
	"strings"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// Build turns r into a Document: each receipt.Text or receipt.Heading
// becomes one Block per wrapped line, and each receipt.Spacer becomes one
// Block, stacked top to bottom in Receipt order. Every Block advances Y by
// f.LineHeight() times its resolved Style.Size, except a Spacer, which
// advances Y by its own Height (dots), per its documented meaning in
// docs/ARCHITECTURE.md §3. The returned Document carries f and
// p.WidthDots (see Document.WidthDots), so every later stage (e.g.
// render/canvas.Paint) measures and paints against the same Font and
// target width Build used.
//
// Every Block Build produces carries a fully resolved Style: a
// receipt.Text's own Bold/Italic/Underline/Strikethrough/Size fields (see
// textStyle), a receipt.Heading's fixed headingStyle (it is presentation
// sugar over Text, not a second styling system — docs/ARCHITECTURE.md
// §3), or normalStyle for anything else (e.g. receipt.Spacer).
// Style.Size is always >= 1 once resolved — see resolveSize — so no
// downstream code ever special-cases a zero or invalid Size.
//
// Text and Heading Content is wrapped to p.WidthDots via wrapText before
// becoming Blocks, measured at the resolved Style's Size (see wrapText's
// docstring). A Document's height is therefore never computed separately
// from its Blocks: it falls out of however many lines wrapping produced,
// the same way it always has for any other sequence of Blocks.
//
// This is an early, partial implementation of the Build described in
// docs/ARCHITECTURE.md §2 — it does not yet accept a context.Context or
// assets.Store, since this slice performs no I/O. Element types other
// than receipt.Text, receipt.Heading, and receipt.Spacer are not yet
// supported and are reported as an apperr.KindPermanent error rather than
// skipped or given placeholder positions.
func Build(r receipt.Receipt, p printer.Profile, f Font) (Document, error) {
	var blocks []Block
	y := 0
	for _, el := range r.Elements {
		switch e := el.(type) {
		case receipt.Text:
			style := textStyle(e)
			for _, line := range wrapText(e.Content, p.WidthDots, f, style.Size) {
				e.Content = line
				blocks = append(blocks, Block{Y: y, Element: e, Style: style})
				y += f.LineHeight() * style.Size
			}
		case receipt.Heading:
			for _, line := range wrapText(e.Content, p.WidthDots, f, headingStyle.Size) {
				e.Content = line
				blocks = append(blocks, Block{Y: y, Element: e, Style: headingStyle})
				y += f.LineHeight() * headingStyle.Size
			}
		case receipt.Spacer:
			blocks = append(blocks, Block{Y: y, Element: el, Style: normalStyle})
			y += e.Height
		default:
			return Document{}, apperr.Wrap(apperr.KindPermanent, "layout.Build", fmt.Errorf("unsupported element type %T", el))
		}
	}
	return Document{WidthDots: p.WidthDots, Blocks: blocks, Font: f}, nil
}

// headingStyle is the fixed Style every receipt.Heading resolves to:
// Heading has no styling fields of its own by design, because it is
// presentation sugar over Text, not a second styling system
// (docs/ARCHITECTURE.md §3, docs/adr/0007-bitmap-text-styling.md) — this
// is the one place that equivalence is expressed, not render/canvas.Paint.
var headingStyle = Style{Bold: true, Size: 2}

// normalStyle is the resolved Style for element types with no styling
// concept of their own (e.g. receipt.Spacer): unstyled, at the
// normalized Size — see Block's doc comment for why every Block Build
// produces has Style.Size >= 1, not just Text/Heading ones.
var normalStyle = Style{Size: 1}

// textStyle resolves t's own styling fields into a Style, normalizing
// Size via resolveSize so the result always has Size >= 1.
func textStyle(t receipt.Text) Style {
	return Style{
		Bold:          t.Bold,
		Italic:        t.Italic,
		Underline:     t.Underline,
		Strikethrough: t.Strikethrough,
		Size:          resolveSize(t.Size),
	}
}

// resolveSize normalizes a receipt.Text.Size value into the >= 1 scale
// factor Style.Size always holds: 0 (an omitted field) becomes 1, per
// docs/adr/0007-bitmap-text-styling.md's "0 or omitted means unscaled"
// convention. receipt.Text.Validate() already rejects negative Size
// before a Receipt reaches Build; resolveSize also floors it to 1 rather
// than propagating it, so the >= 1 invariant holds unconditionally.
func resolveSize(size int) int {
	if size < 1 {
		return 1
	}
	return size
}

// wrapText splits content into the lines Build lays out as separate
// Blocks. It first splits on each explicit "\n", preserving blank lines,
// then — only when widthDots is positive — greedily word-wraps each of
// those lines to fit within widthDots, measured as f.Measure(candidate) *
// size (the same f.Measure every other width decision in this package
// uses, scaled by the resolved Style's Size — docs/ARCHITECTURE.md §3
// "Text styling": nearest-neighbour integer scaling is exact and uniform,
// so this is the effective width of the scaled string, not an
// approximation, and there is no separate scaled-measurement
// implementation).
//
// A word that alone exceeds widthDots is never split: it is emitted as its
// own line and left for render/canvas.Paint's existing clipping to handle
// (see paintGlyph), rather than introducing character-level wrapping —
// consistent with this codebase's raster-first "reuse what Paint already
// does" approach rather than a new one.
//
// widthDots <= 0 is Build's "no printer configured" case
// (printer.Profile.WidthDots's documented sentinel): word-wrapping is
// skipped entirely, so a caller with no printer width still gets exactly
// the lines its explicit "\n"s specify, unchanged.
//
// margins (printer.Profile.MarginLeftDots/MarginRightDots) are
// deliberately not subtracted from widthDots here — Profile's own doc
// comment defers usable-width arithmetic to "a later layout slice".
func wrapText(content string, widthDots int, f Font, size int) []string {
	paragraphs := strings.Split(content, "\n")
	if widthDots <= 0 {
		return paragraphs
	}

	var lines []string
	for _, p := range paragraphs {
		lines = append(lines, wrapParagraph(p, widthDots, f, size)...)
	}
	return lines
}

// wrapParagraph greedily packs p's whitespace-separated words into as few
// lines as possible, each at most widthDots wide per f.Measure(candidate)
// * size, never splitting a word to do so. An empty paragraph (e.g. from
// consecutive "\n"s) yields a single empty line, so blank lines are
// preserved. Runs of interior whitespace are normalized to a single space
// between words, since wrapping already has to decide where each line
// breaks — this only changes output for content with irregular spacing to
// begin with.
func wrapParagraph(p string, widthDots int, f Font, size int) []string {
	words := strings.Fields(p)
	if len(words) == 0 {
		return []string{""}
	}

	lines := make([]string, 0, len(words))
	line := words[0]
	for _, w := range words[1:] {
		candidate := line + " " + w
		if f.Measure(candidate)*size <= widthDots {
			line = candidate
			continue
		}
		lines = append(lines, line)
		line = w
	}
	return append(lines, line)
}
