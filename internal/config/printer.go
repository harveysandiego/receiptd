package config

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/harveysandiego/receiptd/internal/printer"
)

// PrinterConfig is one configured printer, split from its YAML block
// (docs/ARCHITECTURE.md §7) into the two frozen printer types the rest of
// the system uses separately — config is the only place that performs
// this split (§1, §7).
//
// Model is the "model:" name Profile resolved from, or "" when Profile
// came from an explicit "profile:" block instead
// (docs/adr/0015-printer-model-catalogue.md) — informational only, nothing
// branches on it after UnmarshalYAML.
type PrinterConfig struct {
	Name       string
	Model      string
	Profile    printer.Profile
	Connection printer.Connection
}

// UnmarshalYAML decodes a PrinterConfig and splits it into Profile and
// Connection. Per docs/adr/0015-printer-model-catalogue.md, Profile comes
// from exactly one of two mutually exclusive sources — a "model:" lookup
// in printer.ModelProfiles or an explicit "profile:" block — and this
// rejects an entry giving both or neither rather than defining a
// precedence. A "profile:" block's PrintableWidthMM is converted to dots
// via widthDotsFromMM; a "model:" lookup carries its WidthDots through
// unchanged.
func (p *PrinterConfig) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Name  string `yaml:"name"`
		Model string `yaml:"model"`

		Transport string `yaml:"transport"`
		Address   string `yaml:"address"`
		Device    string `yaml:"device"`
		MAC       string `yaml:"mac"`

		Profile *struct {
			PrintableWidthMM float64 `yaml:"printable_width_mm"`
			DPI              int     `yaml:"dpi"`
			Margins          struct {
				Left  int `yaml:"left"`
				Right int `yaml:"right"`
			} `yaml:"margins_dots"`
			SupportsCut        bool   `yaml:"supports_cut"`
			SupportsPartialCut bool   `yaml:"supports_partial_cut"`
			DefaultCut         string `yaml:"default_cut"`
			MaxImageHeightDots int    `yaml:"max_image_height_dots"`
		} `yaml:"profile"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}

	p.Name = raw.Name
	p.Connection = printer.Connection{
		Transport: raw.Transport,
		Address:   raw.Address,
		Device:    raw.Device,
		MAC:       raw.MAC,
	}

	switch {
	case raw.Model != "" && raw.Profile != nil:
		return fmt.Errorf("printer %q: specify either %q or %q, not both", raw.Name, "model", "profile")
	case raw.Model == "" && raw.Profile == nil:
		return fmt.Errorf("printer %q: specify either %q or %q", raw.Name, "model", "profile")
	case raw.Model != "":
		prof, ok := printer.ModelProfiles[raw.Model]
		if !ok {
			return fmt.Errorf("printer %q: unknown model %q (known models: %s)", raw.Name, raw.Model, knownModelNames())
		}
		p.Model = raw.Model
		p.Profile = prof
	default: // raw.Profile != nil
		p.Profile = printer.Profile{
			WidthDots:          widthDotsFromMM(raw.Profile.PrintableWidthMM, raw.Profile.DPI),
			DPI:                raw.Profile.DPI,
			MarginLeftDots:     raw.Profile.Margins.Left,
			MarginRightDots:    raw.Profile.Margins.Right,
			SupportsCut:        raw.Profile.SupportsCut,
			SupportsPartialCut: raw.Profile.SupportsPartialCut,
			DefaultCut:         raw.Profile.DefaultCut,
			MaxImageHeightDots: raw.Profile.MaxImageHeightDots,
		}
	}
	return nil
}

// knownModelNames returns printer.ModelProfiles' keys, sorted for a
// deterministic error message (map iteration order is randomized).
func knownModelNames() string {
	names := make([]string, 0, len(printer.ModelProfiles))
	for name := range printer.ModelProfiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// widthDotsFromMM converts a printable width in mm to dots at dpi:
// dots = round(mm / 25.4 * dpi). mm must be the printhead's printable
// width, never a paper roll width — see
// docs/adr/0015-printer-model-catalogue.md for why one isn't derived from
// the other.
func widthDotsFromMM(widthMM float64, dpi int) int {
	return int(math.Round(widthMM / 25.4 * float64(dpi)))
}

// validate checks the local invariants the frozen schema requires of one
// printer entry's resolved Profile and Connection. Duplicate-name
// checking spans the whole printers[] list, so Config.Validate does that,
// not this method.
func (p PrinterConfig) validate() error {
	var errs []error

	if p.Name == "" {
		errs = append(errs, errors.New("name is required"))
	}
	if p.Profile.DPI <= 0 {
		errs = append(errs, fmt.Errorf("dpi must be positive, got %d", p.Profile.DPI))
	}
	if p.Profile.WidthDots <= 0 {
		errs = append(errs, fmt.Errorf("printable width must be positive, got %d dots", p.Profile.WidthDots))
	}
	if p.Profile.MarginLeftDots < 0 {
		errs = append(errs, fmt.Errorf("margins_dots.left must not be negative, got %d", p.Profile.MarginLeftDots))
	}
	if p.Profile.MarginRightDots < 0 {
		errs = append(errs, fmt.Errorf("margins_dots.right must not be negative, got %d", p.Profile.MarginRightDots))
	}
	if p.Profile.WidthDots > 0 && p.Profile.MarginLeftDots+p.Profile.MarginRightDots >= p.Profile.WidthDots {
		errs = append(errs, fmt.Errorf("margins_dots.left + margins_dots.right must leave a positive usable width, got %d + %d >= %d dots",
			p.Profile.MarginLeftDots, p.Profile.MarginRightDots, p.Profile.WidthDots))
	}
	if p.Profile.MaxImageHeightDots < 0 {
		errs = append(errs, fmt.Errorf("max_image_height_dots must not be negative, got %d", p.Profile.MaxImageHeightDots))
	}
	switch p.Profile.DefaultCut {
	case "full", "partial":
	default:
		errs = append(errs, fmt.Errorf("default_cut must be %q or %q, got %q", "full", "partial", p.Profile.DefaultCut))
	}
	// Only "network" is implemented in v0.1; usb/bluetooth/serial are
	// documented as future transports, not yet valid here
	// (docs/ARCHITECTURE.md §1).
	if p.Connection.Transport != "network" {
		errs = append(errs, fmt.Errorf("transport must be %q (usb, bluetooth, serial are not yet supported), got %q", "network", p.Connection.Transport))
	}

	return errors.Join(errs...)
}
