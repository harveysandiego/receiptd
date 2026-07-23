package layout

import "github.com/harveysandiego/receiptd/internal/receipt"

// Style is a Block's fully resolved styling, produced by Build from the
// source receipt.Text or receipt.Heading (docs/ARCHITECTURE.md §3,
// docs/adr/0007-bitmap-text-styling.md). It is deliberately separate from
// Font: Font is the sole source of a glyph's unstyled base pixels, and
// render/canvas.Paint reads a Block's Style — never receipt fields directly
// — which is what keeps Heading from needing a second rendering path.
//
// Size is always >= 1 on a Style Build produces (an omitted Size normalizes
// to 1), so no downstream code special-cases a zero Size.
type Style struct {
	Bold          bool
	Italic        bool
	Underline     bool
	Strikethrough bool
	Size          int
}

// Block is a single receipt.Element positioned within a Document, styled per
// Style. Y is its vertical offset in dots from the top, computed by Build.
//
// A single receipt.Text or receipt.Heading may become several Blocks, one
// per wrapped line (see Build's wrapText): each carries a copy with just its
// line's Content, so render/canvas.Paint never needs to know wrapping
// happened. Every Block carries a resolved Style, including element types
// with no styling (e.g. receipt.Spacer gets Style{Size: 1}), so Style.Size
// >= 1 is a universal invariant.
type Block struct {
	Y       int
	Element receipt.Element
	Style   Style
}

// Document is the fully positioned, printer-agnostic intermediate
// representation between a Receipt and a Canvas: an ordered list of Blocks
// plus the Font they were measured against. render/canvas.Paint consumes it
// without touching a receipt.Receipt again — see docs/ARCHITECTURE.md §2, §4.
//
// Font is carried here, rather than passed separately to each stage, so
// Build's measurements and Paint's glyphs can never come from different Font
// instances.
//
// WidthDots is the printer-driven canvas width, set by Build from its
// printer.Profile. Zero means Build had no positive width to constrain to
// (see printer.Profile.WidthDots), and Paint falls back to sizing the Canvas
// to its content. There is no HeightDots: a printer.Profile declares no
// paper length (a continuous roll has none), so Paint computes height from
// content.
type Document struct {
	WidthDots int
	Blocks    []Block
	Font      Font
}
