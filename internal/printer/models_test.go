package printer_test

import (
	"testing"

	"github.com/harveysandiego/receiptd/internal/printer"
)

func TestModelProfiles_EpsonTMm30II_MatchesValidatedProfile(t *testing.T) {
	// Values per docs/adr/0015-printer-model-catalogue.md: 576 dots / 72mm
	// printable width at 203dpi, no margins, full+partial cut, no raster
	// chunking — validated against real hardware, not derived here.
	want := printer.Profile{
		WidthDots:          576,
		DPI:                203,
		MarginLeftDots:     0,
		MarginRightDots:    0,
		SupportsCut:        true,
		SupportsPartialCut: true,
		DefaultCut:         "partial",
		MaxImageHeightDots: 0,
	}

	got, ok := printer.ModelProfiles["epson-tm-m30ii"]
	if !ok {
		t.Fatal(`ModelProfiles["epson-tm-m30ii"] not found, want an entry`)
	}
	if got != want {
		t.Errorf(`ModelProfiles["epson-tm-m30ii"] = %+v, want %+v`, got, want)
	}
}

func TestModelProfiles_EveryEntryIsInternallyValid(t *testing.T) {
	// A guard against a typo'd future entry: every catalogued Profile must
	// itself be a usable Profile (positive width/dpi, a recognized
	// DefaultCut) — the same invariants config.PrinterConfig.validate()
	// enforces for an operator-supplied profile, since a model entry
	// bypasses that path entirely (see config.PrinterConfig.UnmarshalYAML).
	for name, p := range printer.ModelProfiles {
		if p.WidthDots <= 0 {
			t.Errorf("ModelProfiles[%q].WidthDots = %d, want positive", name, p.WidthDots)
		}
		if p.DPI <= 0 {
			t.Errorf("ModelProfiles[%q].DPI = %d, want positive", name, p.DPI)
		}
		if p.MarginLeftDots < 0 || p.MarginRightDots < 0 {
			t.Errorf("ModelProfiles[%q] margins = (%d, %d), want non-negative", name, p.MarginLeftDots, p.MarginRightDots)
		}
		if p.MarginLeftDots+p.MarginRightDots >= p.WidthDots {
			t.Errorf("ModelProfiles[%q] margins (%d + %d) leave no usable width within %d dots", name, p.MarginLeftDots, p.MarginRightDots, p.WidthDots)
		}
		if p.DefaultCut != "full" && p.DefaultCut != "partial" {
			t.Errorf("ModelProfiles[%q].DefaultCut = %q, want %q or %q", name, p.DefaultCut, "full", "partial")
		}
		if p.MaxImageHeightDots < 0 {
			t.Errorf("ModelProfiles[%q].MaxImageHeightDots = %d, want non-negative", name, p.MaxImageHeightDots)
		}
	}
}

func TestModelProfiles_HasExactlyOneValidatedEntry(t *testing.T) {
	// docs/adr/0015-printer-model-catalogue.md is explicit: the catalogue
	// starts at exactly the one printer this project has hardware-validated,
	// grown only as further entries are independently verified — never
	// populated speculatively. This test is a deliberate tripwire: growing
	// the table is fine, but should never happen by accident.
	want := []string{"epson-tm-m30ii"}

	if len(printer.ModelProfiles) != len(want) {
		t.Fatalf("len(ModelProfiles) = %d, want %d (%v)", len(printer.ModelProfiles), len(want), want)
	}
	for _, name := range want {
		if _, ok := printer.ModelProfiles[name]; !ok {
			t.Errorf("ModelProfiles missing expected entry %q", name)
		}
	}
}
