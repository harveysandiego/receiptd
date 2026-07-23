package layout

import (
	"context"
	"fmt"
	"strings"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/assets"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// Build turns r into a Document: each element becomes one or more Blocks
// stacked top to bottom in Receipt order (Text/Heading wrap to one Block
// per line via wrapText), each advancing Y per its documented meaning in
// docs/ARCHITECTURE.md §3. The returned Document carries f and p.WidthDots
// so every later stage (e.g. render/canvas.Paint) measures and paints
// against the same Font and target width Build used.
//
// Build is the only stage that resolves a receipt.Asset, via a.Get — the
// I/O that docs/ARCHITECTURE.md §4 reserves to layout. Because
// receipt.Asset holds no resolved bytes, Build carries them forward in a
// layout-local AlignedAsset Block (see docs/adr/0013-text-and-asset-alignment.md).
//
// a may be nil: it is touched only once a receipt.Asset is actually
// matched, so a Receipt with no Asset never needs one. A receipt.Asset
// with a nil a is a wiring mistake, reported as apperr.KindPermanent
// rather than panicking. A missing asset surfaces a.Get's own Kind
// unchanged; invalid image data and any unsupported element type are
// apperr.KindPermanent (never skipped or given a placeholder position).
func Build(ctx context.Context, r receipt.Receipt, p printer.Profile, f Font, a assets.Store) (Document, error) {
	var blocks []Block
	y := 0
	for _, el := range r.Elements {
		switch e := el.(type) {
		case receipt.Text:
			style := textStyle(e)
			for _, line := range wrapText(e.Content, p.WidthDots, f, style.Size) {
				e.Content = alignPad(line, e.Align, p.WidthDots, f, style.Size)
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
		case receipt.Divider:
			blocks = append(blocks, Block{Y: y, Element: el, Style: normalStyle})
			y += DividerThickness * ResolveSize(e.Size)
		case receipt.Image:
			h, err := imageHeight(e.Data, p.WidthDots)
			if err != nil {
				return Document{}, apperr.Wrap(apperr.KindPermanent, "layout.Build", fmt.Errorf("image: %w", err))
			}
			blocks = append(blocks, Block{Y: y, Element: el, Style: normalStyle})
			y += h
		case receipt.Asset:
			if a == nil {
				return Document{}, apperr.Wrap(apperr.KindPermanent, "layout.Build", fmt.Errorf("asset %q: no assets.Store configured to resolve it", e.Name))
			}
			data, err := a.Get(ctx, e.Name)
			if err != nil {
				return Document{}, err
			}
			h, err := assetHeight(data, e.Width, p.WidthDots)
			if err != nil {
				return Document{}, apperr.Wrap(apperr.KindPermanent, "layout.Build", fmt.Errorf("asset: %w", err))
			}
			blocks = append(blocks, Block{Y: y, Element: AlignedAsset{Data: data, Width: e.Width, Align: e.Align}, Style: normalStyle})
			y += h
		case receipt.QRCode:
			_, h, err := qrCodeDimensions(e, p.WidthDots)
			if err != nil {
				return Document{}, apperr.Wrap(apperr.KindPermanent, "layout.Build", fmt.Errorf("qrcode: %w", err))
			}
			blocks = append(blocks, Block{Y: y, Element: el, Style: normalStyle})
			y += h
		case receipt.Barcode:
			w, h, err := barcodeDimensions(e, p.WidthDots)
			if err != nil {
				return Document{}, apperr.Wrap(apperr.KindPermanent, "layout.Build", fmt.Errorf("barcode: %w", err))
			}
			blocks = append(blocks, Block{Y: y, Element: el, Style: normalStyle})
			y += h
			if e.ShowText {
				caption := alignPad(e.Content, alignCenter, w, f, normalStyle.Size)
				blocks = append(blocks, Block{Y: y, Element: BarcodeCaption{Content: caption}, Style: normalStyle})
				y += f.LineHeight() * normalStyle.Size
			}
		case receipt.Table:
			for _, line := range tableLines(e, p.WidthDots, f) {
				blocks = append(blocks, Block{Y: y, Element: TableLine{Content: line}, Style: normalStyle})
				y += f.LineHeight() * normalStyle.Size
			}
		case receipt.Columns:
			lines, err := columnsLines(e, p.WidthDots, f)
			if err != nil {
				return Document{}, apperr.Wrap(apperr.KindPermanent, "layout.Build", fmt.Errorf("columns: %w", err))
			}
			for _, line := range lines {
				blocks = append(blocks, Block{Y: y, Element: ColumnsLine{Content: line}, Style: normalStyle})
				y += f.LineHeight() * normalStyle.Size
			}
		case receipt.List:
			for _, line := range listLines(e, p.WidthDots, f) {
				blocks = append(blocks, Block{Y: y, Element: ListLine{Content: line}, Style: normalStyle})
				y += f.LineHeight() * normalStyle.Size
			}
		case receipt.Feed, receipt.Cut:
			// Printer-control elements: positioned but weightless — unlike
			// every other case here, y is never advanced. See
			// docs/adr/0010-printer-control-elements-via-canvas-controls.md.
			blocks = append(blocks, Block{Y: y, Element: el, Style: normalStyle})
		default:
			return Document{}, apperr.Wrap(apperr.KindPermanent, "layout.Build", fmt.Errorf("unsupported element type %T", el))
		}
	}
	return Document{WidthDots: p.WidthDots, Blocks: blocks, Font: f}, nil
}

// headingStyle is the fixed Style every receipt.Heading resolves to:
// Heading has no styling fields of its own because it is presentation
// sugar over Text (docs/adr/0007-bitmap-text-styling.md). This is the one
// place that equivalence is expressed, not render/canvas.Paint.
var headingStyle = Style{Bold: true, Size: 2}

// normalStyle is the resolved Style for element types with no styling
// concept of their own (e.g. receipt.Spacer, receipt.Divider): unstyled,
// at the normalized Size >= 1 (see Block).
var normalStyle = Style{Size: 1}

// DividerThickness is the height in dots a receipt.Divider occupies at
// Size 1. Build and Paint share this one constant (see blockHeight) so the
// two stages can't disagree on divider height. Style ("solid"/"dashed") is
// not read here: it changes which pixels are painted, not the line's
// vertical extent. See docs/adr/0012-divider-thickness-default-and-scaling.md.
const DividerThickness = 2

// textStyle resolves t's own styling fields into a Style, normalizing
// Size via ResolveSize so the result always has Size >= 1.
func textStyle(t receipt.Text) Style {
	return Style{
		Bold:          t.Bold,
		Italic:        t.Italic,
		Underline:     t.Underline,
		Strikethrough: t.Strikethrough,
		Size:          ResolveSize(t.Size),
	}
}

// ResolveSize floors a Size/Weight value to the >= 1 scale factor each
// field's "0 or omitted means unscaled" convention promises. Exported so
// render/canvas resolves a Divider's Size the same way Build does (see
// canvas.blockHeight), without a second copy of the rule.
func ResolveSize(size int) int {
	if size < 1 {
		return 1
	}
	return size
}

// wrapText splits content into the lines Build lays out as separate
// Blocks: first on each explicit "\n" (preserving blank lines), then, only
// when widthDots is positive, word-wrapping each to fit widthDots measured
// as f.Measure(candidate)*size. A word wider than widthDots is never split
// — it becomes its own line and is left to Paint's clipping (see
// paintGlyph). widthDots <= 0 is the "no printer configured" sentinel
// (printer.Profile.WidthDots): wrapping is skipped, so the caller gets
// exactly the lines its "\n"s specify. Margins are deliberately not
// subtracted here — Profile defers usable-width arithmetic to a later slice.
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

// wrapParagraph greedily packs p's words into as few lines as possible,
// each at most widthDots wide per f.Measure(candidate)*size, never
// splitting a word. An empty paragraph yields a single empty line (so
// blank lines survive); interior whitespace runs collapse to one space.
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
