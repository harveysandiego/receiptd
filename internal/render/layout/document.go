package layout

import "github.com/harveysandiego/receiptd/internal/receipt"

// Block is a single receipt.Element positioned within a Document. Y is
// its vertical offset in dots from the top of the document, computed by
// Build.
//
// A single receipt.Text or receipt.Heading may become several Blocks, one
// per wrapped line (see Build's wrapText): each such Block's Element is a
// copy of the original with Content replaced by just that line, so a
// Block's Element always carries exactly what should be painted on its
// own line — render/canvas.Paint never needs to know wrapping happened.
type Block struct {
	Y       int
	Element receipt.Element
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
