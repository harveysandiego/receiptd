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

// Build turns r into a Document: each receipt.Text or receipt.Heading
// becomes one Block per wrapped line, each receipt.Spacer becomes one
// Block, each receipt.Divider becomes one Block, and each receipt.Image,
// receipt.Asset, receipt.QRCode, or receipt.Barcode becomes one Block,
// stacked top to bottom in Receipt order. A receipt.Text's own Align is
// applied per wrapped line via alignPad before the Block is emitted (see
// alignPad's own doc comment and docs/adr/0013-text-and-asset-alignment.md);
// receipt.Heading has no Align field of its own (docs/adr/0007-bitmap-text-styling.md).
// Every Block advances Y by
// f.LineHeight() times its resolved Style.Size, except a Spacer (which
// advances Y by its own Height, dots), a Divider (which advances Y by
// DividerThickness times its own resolved Size — see ResolveSize), an
// Image (which advances Y by its decoded, printable-width-scaled height —
// see imageHeight), an Asset (resolved via a.Get to raw bytes, then
// advanced by its own resolved width and height — see assetHeight and
// the receipt.Asset case below), a QRCode (which advances Y by its
// generated, printable-width-scaled size — see qrCodeDimensions), a
// Barcode (which advances Y by its configured or default height,
// unaffected by any printable-width scaling of its own — see
// barcodeDimensions — plus one further line if its ShowText is set, for a
// BarcodeCaption Block space-padded to sit roughly centered under the
// barcode's own rendered width — see alignPad's own doc comment
// for why this is font-relative space-padding, not geometric centering),
// and a Feed or Cut (which advance Y by nothing at
// all: they are printer-control elements with no raster footprint — see
// render/escpos.Encode, the stage that turns their Document position into
// actual command bytes). A receipt.Table becomes one TableLine Block per
// composed output line (header row, then each data row, wrapped and
// column-aligned to p.WidthDots — see tableLines and TableLine), keeping
// its own identity through layout the same as every other element type —
// render/canvas.Paint paints a TableLine's Content through the exact same
// glyph-painting path a receipt.Text Block already uses, but still knows
// a Block came from a Table, not ordinary Text. A receipt.Columns becomes
// one ColumnsLine Block per composed output line, the same technique
// generalized from Table's plain-string cells to each Column's own
// receipt.Text content, proportioned across p.WidthDots by each column's
// own Weight — see columnsLines and ColumnsLine. Every
// element advances Y per its documented meaning in docs/ARCHITECTURE.md
// §3. The returned
// Document carries f and p.WidthDots (see Document.WidthDots), so every
// later stage (e.g. render/canvas.Paint) measures and paints against the
// same Font and target width Build used.
//
// receipt.Asset resolves via a.Get(ctx, e.Name) to raw pixel bytes, which
// Build carries forward as an AlignedAsset Block — a small,
// layout-local type distinct from receipt.Image, required specifically
// because assets.Store.Get is I/O and docs/ARCHITECTURE.md §4 reserves
// that resolution step to Build alone ("layout is the only stage that
// talks to the outside world"): receipt.Asset itself has no field to hold
// resolved bytes and must not gain one (see "Image vs. Asset",
// docs/ARCHITECTURE.md §3), so some Build-produced carrier is required
// regardless of where Width/Align end up living — see
// docs/adr/0013-text-and-asset-alignment.md's "Why a new type" for the
// full argument. An AlignedAsset with Width 0 and Align "" renders
// pixel-identically to the receipt.Image lowering this replaced. A
// missing asset surfaces whatever apperr.Kind a.Get itself chose
// (apperr.KindNotFound for assets.Store's own implementations) unchanged;
// invalid resolved image data is reported as apperr.KindPermanent, the
// same Kind an Image's own decode failure already uses.
//
// a may be nil: most callers know in advance their Receipt carries no
// receipt.Asset and have no assets.Store to construct, the same "a nil
// map/interface is a valid zero value until something actually needs it"
// convention Service.Printers and Service.Profiles already establish
// (docs/ARCHITECTURE.md §2). Build only ever calls a method on a once it
// has already matched a receipt.Asset in r.Elements — a Receipt with no
// Asset never touches a at all, nil or not. If r does contain a
// receipt.Asset and a is nil, that is reported as apperr.KindPermanent (a
// caller/wiring mistake, not something retrying fixes) rather than
// panicking on the nil interface method call.
//
// Every Block Build produces carries a fully resolved Style: a
// receipt.Text's own Bold/Italic/Underline/Strikethrough/Size fields (see
// textStyle), a receipt.Heading's fixed headingStyle (it is presentation
// sugar over Text, not a second styling system — docs/ARCHITECTURE.md
// §3), or normalStyle for anything else (e.g. receipt.Spacer,
// receipt.Divider). Style.Size is always >= 1 once resolved — see
// ResolveSize — so no downstream code ever special-cases a zero or
// invalid Size.
//
// Text and Heading Content is wrapped to p.WidthDots via wrapText before
// becoming Blocks, measured at the resolved Style's Size (see wrapText's
// docstring). A Document's height is therefore never computed separately
// from its Blocks: it falls out of however many lines wrapping produced,
// the same way it always has for any other sequence of Blocks.
//
// Element types other than receipt.Text, receipt.Heading, receipt.Spacer,
// receipt.Divider, receipt.Image, receipt.Asset, receipt.QRCode,
// receipt.Barcode, receipt.Table, receipt.Columns, receipt.Feed, and
// receipt.Cut are not yet supported and are reported as an
// apperr.KindPermanent error rather than skipped or given placeholder
// positions. Within a receipt.Columns, only receipt.Text is currently
// renderable — receipt.Heading is deliberately rejected too, not just
// silently downgraded to plain text (see columnLines for why); anything
// else nested in a column is reported the same way.
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
// Heading has no styling fields of its own by design, because it is
// presentation sugar over Text, not a second styling system
// (docs/ARCHITECTURE.md §3, docs/adr/0007-bitmap-text-styling.md) — this
// is the one place that equivalence is expressed, not render/canvas.Paint.
var headingStyle = Style{Bold: true, Size: 2}

// normalStyle is the resolved Style for element types with no styling
// concept of their own (e.g. receipt.Spacer, receipt.Divider): unstyled,
// at the normalized Size — see Block's doc comment for why every Block
// Build produces has Style.Size >= 1, not just Text/Heading ones.
var normalStyle = Style{Size: 1}

// DividerThickness is the height, in dots, a receipt.Divider occupies at
// Size 1 — Build and Paint share this one constant (see blockHeight) so
// the two stages can't disagree, and both scale it by
// ResolveSize(receipt.Divider.Size) the same way. See
// docs/adr/0012-divider-thickness-default-and-scaling.md (which supersedes
// docs/adr/0011-divider-thickness-legibility.md's now-changed default).
// Style ("solid"/"dashed") is not read here — Build advances Y by the same
// thickness regardless of Style, since the dashed pattern only changes
// which pixels along that line are painted (render/canvas.Paint's
// paintDivider), never the line's own vertical extent.
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

// ResolveSize normalizes a receipt.Text.Size, receipt.Divider.Size, or
// receipt.Column.Weight value into the >= 1 scale factor each field's "0
// or omitted means unscaled" convention promises (docs/adr/0007-bitmap-text-styling.md,
// docs/adr/0012-divider-thickness-default-and-scaling.md). All three
// types' Validate() already reject a negative value before a Receipt
// reaches Build; ResolveSize also floors it to 1 rather than propagating
// it, so the >= 1 invariant holds unconditionally. Exported so
// render/canvas can resolve a receipt.Divider's Size the same way Build
// itself does (see canvas.blockHeight), without a second copy of this
// rule.
func ResolveSize(size int) int {
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
