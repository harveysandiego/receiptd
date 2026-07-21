// Package config loads and validates Receiptd's YAML configuration file.
// Each printers[] entry is a single block for the user's convenience —
// either a known "model:" name or an explicit "profile:", never both
// (docs/adr/0015-printer-model-catalogue.md); config is the one place
// that splits it into the printer.Profile and printer.Connection values
// the rest of the system uses separately. See docs/ARCHITECTURE.md §7.
package config
