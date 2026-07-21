package config_test

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/harveysandiego/receiptd/internal/config"
	"github.com/harveysandiego/receiptd/internal/printer"
)

func TestPrinterConfigUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantPrinter config.PrinterConfig
	}{
		{
			name: "known model",
			yaml: `
name: front-desk
model: epson-tm-m30ii
transport: network
address: 192.168.1.50:9100
`,
			wantPrinter: config.PrinterConfig{
				Name:  "front-desk",
				Model: "epson-tm-m30ii",
				Connection: printer.Connection{
					Transport: "network",
					Address:   "192.168.1.50:9100",
				},
				Profile: printer.ModelProfiles["epson-tm-m30ii"],
			},
		},
		{
			name: "custom profile, full fields",
			yaml: `
name: default
transport: network
address: 192.168.1.50:9100
profile:
  printable_width_mm: 80
  dpi: 203
  margins_dots: { left: 2, right: 3 }
  supports_cut: true
  supports_partial_cut: true
  default_cut: partial
  max_image_height_dots: 512
`,
			wantPrinter: config.PrinterConfig{
				Name: "default",
				Connection: printer.Connection{
					Transport: "network",
					Address:   "192.168.1.50:9100",
				},
				Profile: printer.Profile{
					WidthDots:          639, // round(80mm / 25.4 * 203dpi)
					DPI:                203,
					MarginLeftDots:     2,
					MarginRightDots:    3,
					SupportsCut:        true,
					SupportsPartialCut: true,
					DefaultCut:         "partial",
					MaxImageHeightDots: 512,
				},
			},
		},
		{
			name: "custom profile, 58mm narrow paper",
			yaml: `
name: narrow
transport: network
address: 10.0.0.5:9100
profile:
  printable_width_mm: 58
  dpi: 203
  default_cut: full
`,
			wantPrinter: config.PrinterConfig{
				Name: "narrow",
				Connection: printer.Connection{
					Transport: "network",
					Address:   "10.0.0.5:9100",
				},
				Profile: printer.Profile{
					WidthDots:  464, // round(58 / 25.4 * 203) = round(463.543...)
					DPI:        203,
					DefaultCut: "full",
				},
			},
		},
		{
			name: "usb transport carries Device, not Address",
			yaml: `
name: usb-printer
model: epson-tm-m30ii
transport: usb
device: /dev/usb/lp0
`,
			wantPrinter: config.PrinterConfig{
				Name:  "usb-printer",
				Model: "epson-tm-m30ii",
				Connection: printer.Connection{
					Transport: "usb",
					Device:    "/dev/usb/lp0",
				},
				Profile: printer.ModelProfiles["epson-tm-m30ii"],
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got config.PrinterConfig
			if err := yaml.Unmarshal([]byte(tt.yaml), &got); err != nil {
				t.Fatalf("yaml.Unmarshal: unexpected error: %v", err)
			}
			if got != tt.wantPrinter {
				t.Errorf("got %+v, want %+v", got, tt.wantPrinter)
			}
		})
	}
}

func TestPrinterConfigUnmarshalYAML_KnownModel_MatchesEquivalentCustomProfile(t *testing.T) {
	// The ADR's central invariant: a "model:" lookup and an operator's own
	// "profile:" block describing the identical hardware must resolve to
	// exactly the same printer.Profile — the catalogue is a convenience
	// over the custom path, not a different mechanism with different
	// results.
	modelYAML := `
name: front-desk
model: epson-tm-m30ii
transport: network
address: 192.168.1.50:9100
`
	profileYAML := `
name: front-desk
transport: network
address: 192.168.1.50:9100
profile:
  printable_width_mm: 72.02
  dpi: 203
  margins_dots: { left: 0, right: 0 }
  supports_cut: true
  supports_partial_cut: true
  default_cut: partial
  max_image_height_dots: 0
`

	var viaModel, viaProfile config.PrinterConfig
	if err := yaml.Unmarshal([]byte(modelYAML), &viaModel); err != nil {
		t.Fatalf("yaml.Unmarshal(modelYAML): unexpected error: %v", err)
	}
	if err := yaml.Unmarshal([]byte(profileYAML), &viaProfile); err != nil {
		t.Fatalf("yaml.Unmarshal(profileYAML): unexpected error: %v", err)
	}

	if viaModel.Profile != viaProfile.Profile {
		t.Errorf("Profile via model = %+v, Profile via equivalent custom profile = %+v, want equal", viaModel.Profile, viaProfile.Profile)
	}
}

func TestPrinterConfigUnmarshalYAML_BothModelAndProfile_Errors(t *testing.T) {
	yamlSrc := `
name: default
model: epson-tm-m30ii
transport: network
address: 192.168.1.50:9100
profile:
  printable_width_mm: 80
  dpi: 203
  default_cut: partial
`
	var got config.PrinterConfig
	err := yaml.Unmarshal([]byte(yamlSrc), &got)
	if err == nil {
		t.Fatal("yaml.Unmarshal: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "model") || !strings.Contains(err.Error(), "profile") {
		t.Errorf("yaml.Unmarshal: err = %q, want it to mention both %q and %q", err.Error(), "model", "profile")
	}
}

func TestPrinterConfigUnmarshalYAML_NeitherModelNorProfile_Errors(t *testing.T) {
	yamlSrc := `
name: default
transport: network
address: 192.168.1.50:9100
`
	var got config.PrinterConfig
	err := yaml.Unmarshal([]byte(yamlSrc), &got)
	if err == nil {
		t.Fatal("yaml.Unmarshal: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "model") || !strings.Contains(err.Error(), "profile") {
		t.Errorf("yaml.Unmarshal: err = %q, want it to mention both %q and %q", err.Error(), "model", "profile")
	}
}

func TestPrinterConfigUnmarshalYAML_UnknownModel_Errors(t *testing.T) {
	yamlSrc := `
name: default
model: some-printer-nobody-has-heard-of
transport: network
address: 192.168.1.50:9100
`
	var got config.PrinterConfig
	err := yaml.Unmarshal([]byte(yamlSrc), &got)
	if err == nil {
		t.Fatal("yaml.Unmarshal: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "some-printer-nobody-has-heard-of") {
		t.Errorf("yaml.Unmarshal: err = %q, want it to mention the unrecognized model name", err.Error())
	}
	if !strings.Contains(err.Error(), "epson-tm-m30ii") {
		t.Errorf("yaml.Unmarshal: err = %q, want it to list the known models (epson-tm-m30ii)", err.Error())
	}
}
