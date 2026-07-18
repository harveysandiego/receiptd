package config_test

import (
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
			name: "network transport, full fields",
			yaml: `
name: default
transport: network
address: 192.168.1.50:9100
width_mm: 80
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
					WidthDots:          639,
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
			name: "58mm narrow paper",
			yaml: `
name: narrow
transport: network
address: 10.0.0.5:9100
width_mm: 58
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
transport: usb
device: /dev/usb/lp0
width_mm: 80
dpi: 203
default_cut: full
`,
			wantPrinter: config.PrinterConfig{
				Name: "usb-printer",
				Connection: printer.Connection{
					Transport: "usb",
					Device:    "/dev/usb/lp0",
				},
				Profile: printer.Profile{
					WidthDots:  639,
					DPI:        203,
					DefaultCut: "full",
				},
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
