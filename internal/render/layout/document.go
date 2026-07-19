package layout

import "github.com/harveysandiego/receiptd/internal/receipt"

// Style is a Block's fully resolved styling, produced by Build from
// whatever styling fields the source receipt.Text or receipt.Heading
// carries (see docs/ARCHITECTURE.md §3 "Text styling" and
// docs/adr/0007-bitmap-text-styling.md). Style is deliberately not part
// of Font: Font remains the sole source of a glyph's unscaled, unstyled
// base pixels, and Style is the separate, element-type-agnostic concern
// render/canvas.Paint layers on top of it — Paint reads a Block's Style,
// never receipt.Text/receipt.Heading fields directly, which is what
// keeps Heading from needing a second rendering path.
//
// Size is always >= 1 on a Style Build produced: 0 (an omitted
// receipt.Text.Size) is normalized to 1 during Build, so no downstream
// code — wrapping, measurement, or painting — ever needs to special-case
// a zero or otherwise invalid Size.
type Style struct {
	Bold          bool
	Italic        bool
	Underline     bool
	Strikethrough bool
	Size          int
}

// Block is a single receipt.Element positioned within a Document, styled
// per Style. Y is its vertical offset in dots from the top of the
// document, computed by Build.
//
// A single receipt.Text or receipt.Heading may become several Blocks, one
// per wrapped line (see Build's wrapText): each such Block's Element is a
// copy of the original with Content replaced by just that line, so a
// Block's Element always carries exactly what should be painted on its
// own line — render/canvas.Paint never needs to know wrapping happened.
// Every Block produced by Build carries a fully resolved Style, including
// ones for element types (e.g. receipt.Spacer) with no styling of their
// own — Style{Size: 1} there, so Style.Size >= 1 is a universal
// invariant, not one downstream code must check per element type.
type Block struct {
	Y       int
	Element receipt.Element
	Style   Style
}

// Document is the fully positioned, printer-agnostic intermediate
// representation between a Receipt and a Canvas: an ordered list of
// Blocks plus the Font they were measured against. render/canvas.Paint
// consumes a Document without ever touching a receipt.Receipt again —
// see docs/ARCHITECTURE.md §2, §4.
//
// Font is carried here, rather than passed separately to each stage that
// needs it, so Build's measurements and Paint's painted glyphs can never
// silently come from different Font instances.
//
// WidthDots is the frozen Document's (ARCHITECTURE.md §2) printer-driven
// canvas width, now present: Build sets it from the printer.Profile it
// was given. A zero value means Build had no positive width to constrain
// to (see printer.Profile.WidthDots's doc comment), and Paint falls back
// to sizing the Canvas to its painted content, exactly as this codebase
// has always done. HeightDots from the frozen struct is not yet present —
// unlike width, a printer.Profile carries no notion of paper length (a
// continuous roll has none to declare), so there is no second value to
// thread through yet, and Paint continues to compute the Canvas's height
// from content itself.
type Document struct {
	WidthDots int
	Blocks    []Block
	Font      Font
}
