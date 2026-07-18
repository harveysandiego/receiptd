package printer

import "context"

// Printer sends already-encoded bytes (produced by render/escpos) to a
// physical printer over whatever transport a Connection was resolved to.
// It never sees a Profile or a Connection itself — those are consumed by
// whatever constructs a Printer (cmd/receiptd), not by Printer's callers.
type Printer interface {
	// Send transmits data, already-encoded by render/escpos, to the
	// printer.
	Send(ctx context.Context, data []byte) error

	// Status reports whether the printer is currently reachable.
	Status(ctx context.Context) (Status, error)

	// Close releases any resources held by the underlying transport.
	Close() error
}

// Status is a point-in-time report of a printer's reachability.
type Status struct {
	// Online reports whether the printer responded.
	Online bool
	// Detail is a human-readable elaboration, e.g. the underlying
	// transport error when Online is false.
	Detail string
}
