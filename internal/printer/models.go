package printer

// ModelProfiles is the built-in catalogue config resolves a printer
// config entry's "model:" field against (docs/adr/0015-printer-model-catalogue.md).
// Every entry is a Profile whose characteristics have been independently
// verified — preferably through real hardware testing — never derived
// from a heuristic (e.g. guessed from paper roll width) or transcribed
// speculatively from a spec sheet alone. This is a plain data table, not
// a registration mechanism: entries are added directly here, not via
// init()-time registration (docs/adr/0004-extension-model.md's one
// extension mechanism is unrelated to this table).
//
// The catalogue starts at exactly one entry and is expected to grow only
// as new hardware is actually validated — see the ADR for why this
// project won't populate it ahead of that.
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
