package printer

// ModelProfiles is the built-in catalogue config resolves a printer
// entry's "model:" field against (docs/adr/0015-printer-model-catalogue.md).
// Every entry is verified — preferably on real hardware — never guessed
// from a heuristic (e.g. paper roll width) or a spec sheet alone. It's a
// plain data table, not a registration mechanism: entries are added
// directly here, not via init()-time registration.
//
// The catalogue starts at one entry and grows only as new hardware is
// validated — see the ADR.
var ModelProfiles = map[string]Profile{
	// epson-tm-m30ii: Epson TM-m30II, 80mm paper roll, 72mm / 576-dot
	// printable width at 203dpi — the printer this project's List Element
	// (docs/adr/0014-list-elements.md) was hardware-validated against.
	// Supports both full and partial cuts; needs no raster chunking.
	"epson-tm-m30ii": {
		WidthDots:          576,
		DPI:                203,
		MarginLeftDots:     0,
		MarginRightDots:    0,
		SupportsCut:        true,
		SupportsPartialCut: true,
		DefaultCut:         "partial",
		MaxImageHeightDots: 0,
	},
}
