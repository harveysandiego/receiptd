package layout

import "github.com/harveysandiego/receiptd/internal/receipt"

// Block is a single receipt.Element positioned within a Document. Y is
// its vertical offset in dots from the top of the document; Build (not
// yet implemented) is what computes it — a zero-value Block simply has
// no position assigned yet.
//
// Wrapping receipt.Element as-is is deliberately provisional: once Build
// exists, a single long Text may need to become several Blocks (one per
// wrapped line), which this shape cannot express, since a Block's
// Element still carries the whole original content. That transformation
// depends on text measurement (Font), which this slice excludes on
// purpose — reshaping Block to anticipate it now would mean guessing at
// Build's output before Build exists. This is expected to change in the
// slice that implements Build, not a defect to fix here.
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
// silently come from different Font instances. WidthDots and HeightDots
// from the frozen Document (ARCHITECTURE.md §2) are not yet present:
// nothing in this codebase threads a printer.Profile through Build yet,
// so there's no value to put there and no second use to justify adding
// them ahead of that.
type Document struct {
	Blocks []Block
	Font   Font
}
