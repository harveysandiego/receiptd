package config

import (
	"errors"
	"fmt"
	"math"

	"gopkg.in/yaml.v3"

	"github.com/harveysandiego/receiptd/internal/printer"
)

// PrinterConfig is one configured printer, split from its flat YAML block
// (docs/ARCHITECTURE.md §7) into the two frozen printer package types the
// rest of the system uses separately. config is the only place that
// performs this split (see docs/ARCHITECTURE.md §1, §7).
type PrinterConfig struct {
	Name       string
	Profile    printer.Profile
	Connection printer.Connection
}

// UnmarshalYAML decodes a PrinterConfig from its flat YAML block and
// splits it into Profile and Connection. WidthDots is derived from
// width_mm and dpi as dots = round(width_mm / 25.4 * dpi) — the standard
// dots-per-inch definition — since the frozen Profile type stores width
// in dots, not millimeters.
func (p *PrinterConfig) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Name string `yaml:"name"`

		Transport string `yaml:"transport"`
		Address   string `yaml:"address"`
		Device    string `yaml:"device"`
		MAC       string `yaml:"mac"`

		WidthMM float64 `yaml:"width_mm"`
		DPI     int     `yaml:"dpi"`
		Margins struct {
			Left  int `yaml:"left"`
			Right int `yaml:"right"`
		} `yaml:"margins_dots"`
		SupportsCut        bool   `yaml:"supports_cut"`
		SupportsPartialCut bool   `yaml:"supports_partial_cut"`
		DefaultCut         string `yaml:"default_cut"`
		MaxImageHeightDots int    `yaml:"max_image_height_dots"`
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
	p.Profile = printer.Profile{
		WidthDots:          widthDotsFromMM(raw.WidthMM, raw.DPI),
		DPI:                raw.DPI,
		MarginLeftDots:     raw.Margins.Left,
		MarginRightDots:    raw.Margins.Right,
		SupportsCut:        raw.SupportsCut,
		SupportsPartialCut: raw.SupportsPartialCut,
		DefaultCut:         raw.DefaultCut,
		MaxImageHeightDots: raw.MaxImageHeightDots,
	}
	return nil
}

// widthDotsFromMM converts a paper width in millimeters to dots at the
// given dots-per-inch density: dots = round(mm / 25.4 * dpi).
func widthDotsFromMM(widthMM float64, dpi int) int {
	return int(math.Round(widthMM / 25.4 * float64(dpi)))
}

// validate checks the local invariants the frozen schema requires of a
// single printer entry. Checking for duplicate names spans the whole
// printers[] list, so Config.Validate does that, not this method.
func (p PrinterConfig) validate() error {
	var errs []error

	if p.Name == "" {
		errs = append(errs, errors.New("name is required"))
	}
	if p.Profile.DPI <= 0 {
		errs = append(errs, fmt.Errorf("dpi must be positive, got %d", p.Profile.DPI))
	}
	if p.Profile.WidthDots <= 0 {
		errs = append(errs, fmt.Errorf("width_mm and dpi must produce a positive width, got %d dots", p.Profile.WidthDots))
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
	// documented on printer.Connection.Transport as future transports,
	// not yet valid here (docs/ARCHITECTURE.md §1).
	if p.Connection.Transport != "network" {
		errs = append(errs, fmt.Errorf("transport must be %q (usb, bluetooth, serial are not yet supported), got %q", "network", p.Connection.Transport))
	}

	return errors.Join(errs...)
}
