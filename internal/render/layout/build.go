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
// f.LineHeight(), except a Spacer, which advances Y by its own Height
// (dots), per its documented meaning in docs/ARCHITECTURE.md §3. The
// returned Document carries f and p.WidthDots (see Document.WidthDots), so
// every later stage (e.g. render/canvas.Paint) measures and paints against
// the same Font and target width Build used.
//
// Text and Heading Content is wrapped to p.WidthDots via wrapText before
// becoming Blocks — see wrapText's docstring for the wrapping rules. A
// Document's height is therefore never computed separately from its
// Blocks: it falls out of however many lines wrapping produced, the same
// way it always has for any other sequence of Blocks.
//
// This is an early, partial implementation of the Build described in
// docs/ARCHITECTURE.md §2 — it does not yet accept a context.Context or
// assets.Store, since this slice performs no I/O. Heading's documented
// "bold + large" styling (docs/ARCHITECTURE.md §3) is not applied here:
// Font has no notion of weight or size variation yet (the same gap that
// already leaves receipt.Text's own Bold/Size hints unapplied), so a
// Heading is positioned exactly like a Text with the same Content.
// Element types other than receipt.Text, receipt.Heading, and
// receipt.Spacer are not yet supported and are reported as an
// apperr.KindPermanent error rather than skipped or given placeholder
// positions.
func Build(r receipt.Receipt, p printer.Profile, f Font) (Document, error) {
	var blocks []Block
	y := 0
	for _, el := range r.Elements {
		switch e := el.(type) {
		case receipt.Text:
			for _, line := range wrapText(e.Content, p.WidthDots, f) {
				e.Content = line
				blocks = append(blocks, Block{Y: y, Element: e})
				y += f.LineHeight()
			}
		case receipt.Heading:
			for _, line := range wrapText(e.Content, p.WidthDots, f) {
				e.Content = line
				blocks = append(blocks, Block{Y: y, Element: e})
				y += f.LineHeight()
			}
		case receipt.Spacer:
			blocks = append(blocks, Block{Y: y, Element: el})
			y += e.Height
		default:
			return Document{}, apperr.Wrap(apperr.KindPermanent, "layout.Build", fmt.Errorf("unsupported element type %T", el))
		}
	}
	return Document{WidthDots: p.WidthDots, Blocks: blocks, Font: f}, nil
}

// wrapText splits content into the lines Build lays out as separate
// Blocks. It first splits on each explicit "\n", preserving blank lines,
// then — only when widthDots is positive — greedily word-wraps each of
// those lines to fit within widthDots, measured via f.Measure (the same
// measurement every other width decision in this package already uses).
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
func wrapText(content string, widthDots int, f Font) []string {
	paragraphs := strings.Split(content, "\n")
	if widthDots <= 0 {
		return paragraphs
	}

	var lines []string
	for _, p := range paragraphs {
		lines = append(lines, wrapParagraph(p, widthDots, f)...)
	}
	return lines
}

// wrapParagraph greedily packs p's whitespace-separated words into as few
// lines as possible, each at most widthDots wide per f.Measure, never
// splitting a word to do so. An empty paragraph (e.g. from consecutive
// "\n"s) yields a single empty line, so blank lines are preserved. Runs of
// interior whitespace are normalized to a single space between words,
// since wrapping already has to decide where each line breaks — this only
// changes output for content with irregular spacing to begin with.
func wrapParagraph(p string, widthDots int, f Font) []string {
	words := strings.Fields(p)
	if len(words) == 0 {
		return []string{""}
	}

	lines := make([]string, 0, len(words))
	line := words[0]
	for _, w := range words[1:] {
		candidate := line + " " + w
		if f.Measure(candidate) <= widthDots {
			line = candidate
			continue
		}
		lines = append(lines, line)
		line = w
	}
	return append(lines, line)
}
