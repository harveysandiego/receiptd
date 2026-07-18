package printer

// Profile describes what a printer can do: paper geometry and cut
// capability. It carries no information about how to reach the printer —
// see Connection for that split, and docs/ARCHITECTURE.md §1 for why the
// two are never mixed in a single function signature. render/* only ever
// receives a Profile.
type Profile struct {
	// WidthDots is the printer's paper width, in dots, at DPI.
	WidthDots int
	// DPI is the printer's horizontal/vertical dot density.
	DPI int
	// MarginLeftDots and MarginRightDots are unprintable dots reserved on
	// each side, within WidthDots. Usable content width is derived from
	// WidthDots minus these margins by a later layout slice — Profile
	// itself has no width-arithmetic helpers (see docs/ARCHITECTURE.md
	// §1: capabilities and transport stay data here, not behavior).
	MarginLeftDots  int
	MarginRightDots int
	// SupportsCut reports whether the printer can cut paper at all.
	SupportsCut bool
	// SupportsPartialCut reports whether the printer can perform a
	// partial (as opposed to only full) cut.
	SupportsPartialCut bool
	// DefaultCut is the cut style to use when a job doesn't request one:
	// "full" or "partial".
	DefaultCut string
	// MaxImageHeightDots is the tallest raster image this printer accepts
	// in one command, or 0 if it needs no chunking.
	MaxImageHeightDots int
}
