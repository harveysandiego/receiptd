package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/config"
	"github.com/harveysandiego/receiptd/internal/printer"
)

const validYAML = `
server:
  address: ":8080"

auth:
  enabled: true
  token_file: /etc/receiptd/token

logging:
  level: info
  format: auto

assets:
  path: /var/lib/receiptd/assets

queue:
  store: bbolt
  path: /var/lib/receiptd/queue.db
  max_attempts: 3
  retry_backoff: 5s

printers:
  - name: default
    transport: network
    address: 192.168.1.50:9100
    width_mm: 80
    dpi: 203
    margins_dots: { left: 0, right: 0 }
    supports_cut: true
    supports_partial_cut: true
    default_cut: partial
    max_image_height_dots: 0

providers:
  weather:
    driver: openweather
    api_key_env: OPENWEATHER_API_KEY

web:
  enabled: true
`

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "receiptd.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	return path
}

func TestLoad_Success(t *testing.T) {
	path := writeConfig(t, validYAML)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	if got, want := cfg.Server.Address, ":8080"; got != want {
		t.Errorf("Server.Address = %q, want %q", got, want)
	}
	if !cfg.Auth.Enabled {
		t.Errorf("Auth.Enabled = false, want true")
	}
	if got, want := cfg.Auth.TokenFile, "/etc/receiptd/token"; got != want {
		t.Errorf("Auth.TokenFile = %q, want %q", got, want)
	}
	if got, want := cfg.Logging.Level, "info"; got != want {
		t.Errorf("Logging.Level = %q, want %q", got, want)
	}
	if got, want := cfg.Logging.Format, "auto"; got != want {
		t.Errorf("Logging.Format = %q, want %q", got, want)
	}
	if got, want := cfg.Assets.Path, "/var/lib/receiptd/assets"; got != want {
		t.Errorf("Assets.Path = %q, want %q", got, want)
	}
	if got, want := cfg.Queue.Store, "bbolt"; got != want {
		t.Errorf("Queue.Store = %q, want %q", got, want)
	}
	if got, want := cfg.Queue.Path, "/var/lib/receiptd/queue.db"; got != want {
		t.Errorf("Queue.Path = %q, want %q", got, want)
	}
	if got, want := cfg.Queue.MaxAttempts, 3; got != want {
		t.Errorf("Queue.MaxAttempts = %d, want %d", got, want)
	}
	if got, want := cfg.Queue.RetryBackoff, 5*time.Second; got != want {
		t.Errorf("Queue.RetryBackoff = %v, want %v", got, want)
	}
	if got, want := cfg.Providers.Weather.Driver, "openweather"; got != want {
		t.Errorf("Providers.Weather.Driver = %q, want %q", got, want)
	}
	if got, want := cfg.Providers.Weather.APIKeyEnv, "OPENWEATHER_API_KEY"; got != want {
		t.Errorf("Providers.Weather.APIKeyEnv = %q, want %q", got, want)
	}
	if !cfg.Web.Enabled {
		t.Errorf("Web.Enabled = false, want true")
	}

	if len(cfg.Printers) != 1 {
		t.Fatalf("len(Printers) = %d, want 1", len(cfg.Printers))
	}
	p := cfg.Printers[0]
	if got, want := p.Name, "default"; got != want {
		t.Errorf("Printers[0].Name = %q, want %q", got, want)
	}

	wantConn := printer.Connection{Transport: "network", Address: "192.168.1.50:9100"}
	if p.Connection != wantConn {
		t.Errorf("Printers[0].Connection = %+v, want %+v", p.Connection, wantConn)
	}

	wantProfile := printer.Profile{
		WidthDots:          639, // round(80mm / 25.4 * 203dpi)
		DPI:                203,
		MarginLeftDots:     0,
		MarginRightDots:    0,
		SupportsCut:        true,
		SupportsPartialCut: true,
		DefaultCut:         "partial",
		MaxImageHeightDots: 0,
	}
	if p.Profile != wantProfile {
		t.Errorf("Printers[0].Profile = %+v, want %+v", p.Profile, wantProfile)
	}
}

func TestLoad_AuthSectionOmitted_DefaultsEnabledTrue(t *testing.T) {
	yaml := `
queue:
  store: bbolt
  max_attempts: 3
  retry_backoff: 5s

printers:
  - name: default
    transport: network
    address: 192.168.1.50:9100
    width_mm: 80
    dpi: 203
    default_cut: partial
`
	path := writeConfig(t, yaml)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if !cfg.Auth.Enabled {
		t.Error("Auth.Enabled = false, want true (auth must default on when the auth: section is omitted)")
	}
}

func TestLoad_AuthExplicitlyDisabled_StaysDisabled(t *testing.T) {
	yaml := `
auth:
  enabled: false

queue:
  store: bbolt
  max_attempts: 3
  retry_backoff: 5s

printers:
  - name: default
    transport: network
    address: 192.168.1.50:9100
    width_mm: 80
    dpi: 203
    default_cut: partial
`
	path := writeConfig(t, yaml)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if cfg.Auth.Enabled {
		t.Error("Auth.Enabled = true, want false (an explicit auth.enabled: false must still be honored)")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("Load: expected error, got nil")
	}
	if !apperr.Is(err, apperr.KindNotFound) {
		t.Errorf("Load: err = %v, want apperr.KindNotFound", err)
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	path := writeConfig(t, "server: [unterminated")

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("Load: expected error, got nil")
	}
	if !apperr.Is(err, apperr.KindValidation) {
		t.Errorf("Load: err = %v, want apperr.KindValidation", err)
	}
}

func TestLoad_InvalidConfig(t *testing.T) {
	base := func(printersYAML string) string {
		return `
queue:
  store: bbolt
  max_attempts: 3
  retry_backoff: 5s

printers:
` + printersYAML
	}

	validPrinter := `  - name: default
    transport: network
    address: 192.168.1.50:9100
    width_mm: 80
    dpi: 203
    default_cut: partial
`

	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "unsupported queue store",
			yaml: `
queue:
  store: redis
  max_attempts: 3
  retry_backoff: 5s

printers:
` + validPrinter,
		},
		{
			name: "non-positive max_attempts",
			yaml: `
queue:
  store: bbolt
  max_attempts: 0
  retry_backoff: 5s

printers:
` + validPrinter,
		},
		{
			name: "non-positive retry_backoff",
			yaml: `
queue:
  store: bbolt
  max_attempts: 3
  retry_backoff: -5s

printers:
` + validPrinter,
		},
		{
			name: "empty printer name",
			yaml: base(`  - name: ""
    transport: network
    address: 192.168.1.50:9100
    width_mm: 80
    dpi: 203
    default_cut: partial
`),
		},
		{
			name: "non-positive dpi",
			yaml: base(`  - name: default
    transport: network
    address: 192.168.1.50:9100
    width_mm: 80
    dpi: 0
    default_cut: partial
`),
		},
		{
			name: "non-positive width",
			yaml: base(`  - name: default
    transport: network
    address: 192.168.1.50:9100
    width_mm: 0
    dpi: 203
    default_cut: partial
`),
		},
		{
			name: "negative left margin",
			yaml: base(`  - name: default
    transport: network
    address: 192.168.1.50:9100
    width_mm: 80
    dpi: 203
    margins_dots: { left: -1, right: 0 }
    default_cut: partial
`),
		},
		{
			name: "margins sum to at least the width",
			yaml: base(`  - name: default
    transport: network
    address: 192.168.1.50:9100
    width_mm: 80
    dpi: 203
    margins_dots: { left: 639, right: 0 }
    default_cut: partial
`),
		},
		{
			name: "negative max_image_height_dots",
			yaml: base(`  - name: default
    transport: network
    address: 192.168.1.50:9100
    width_mm: 80
    dpi: 203
    default_cut: partial
    max_image_height_dots: -1
`),
		},
		{
			name: "invalid default_cut",
			yaml: base(`  - name: default
    transport: network
    address: 192.168.1.50:9100
    width_mm: 80
    dpi: 203
    default_cut: sideways
`),
		},
		{
			name: "unknown transport",
			yaml: base(`  - name: default
    transport: carrier-pigeon
    address: 192.168.1.50:9100
    width_mm: 80
    dpi: 203
    default_cut: partial
`),
		},
		{
			name: "usb transport not yet supported in v0.1",
			yaml: base(`  - name: default
    transport: usb
    device: /dev/usb/lp0
    width_mm: 80
    dpi: 203
    default_cut: partial
`),
		},
		{
			name: "bluetooth transport not yet supported in v0.1",
			yaml: base(`  - name: default
    transport: bluetooth
    mac: 00:11:22:33:44:55
    width_mm: 80
    dpi: 203
    default_cut: partial
`),
		},
		{
			name: "serial transport not yet supported in v0.1",
			yaml: base(`  - name: default
    transport: serial
    device: /dev/ttyUSB0
    width_mm: 80
    dpi: 203
    default_cut: partial
`),
		},
		{
			name: "duplicate printer names",
			yaml: base(validPrinter + `  - name: default
    transport: network
    address: 192.168.1.51:9100
    width_mm: 80
    dpi: 203
    default_cut: partial
`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.yaml)

			_, err := config.Load(path)
			if err == nil {
				t.Fatal("Load: expected error, got nil")
			}
			if !apperr.Is(err, apperr.KindValidation) {
				t.Errorf("Load: err = %v, want apperr.KindValidation", err)
			}
		})
	}
}
