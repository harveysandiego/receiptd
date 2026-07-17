// Package config loads and validates Receiptd's YAML configuration file.
// Each printers[] entry is a single flat YAML block for the user's
// convenience; config is the one place that splits it into the
// printer.Profile and printer.Connection values the rest of the system
// uses separately. See docs/ARCHITECTURE.md §7.
package config
