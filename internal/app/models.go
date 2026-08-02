package app

import "time"

// ConnectionSummary is a configured printer's connection metadata, exposed
// as plain display data — never printer.Connection itself
// (docs/ARCHITECTURE.md §1: capabilities and transport are different
// concepts and never mix; this is that rule applied to what Service
// exposes upward, not just to render/*). cmd/receiptd is still the only
// thing that constructs a printer.Connection; it copies these two fields
// out of one into Service.Connections at startup.
type ConnectionSummary struct {
	Transport string
	Address   string
}

// PrinterSummary is a read-only, presentation-oriented view of one
// configured printer — name, connection metadata, capability profile,
// and current reachability — for the Web UI's printer status and
// settings pages (docs/adr/0024-printer-settings-screen-is-read-only.md).
// It never carries a printer.Connection, printer.Profile, or
// config.PrinterConfig; Service.ListPrinters is the only thing that
// constructs one.
type PrinterSummary struct {
	Name string

	// Transport and Address mirror ConnectionSummary, flattened onto the
	// summary rather than nested: nothing in the Web UI has a use for a
	// PrinterSummary without also knowing how to reach the printer it
	// describes.
	Transport string
	Address   string

	// WidthDots, DPI, SupportsCut, and SupportsPartialCut are the
	// capability fields docs/adr/0024 lists for the read-only settings
	// screen, taken from printer.Profile.
	WidthDots          int
	DPI                int
	SupportsCut        bool
	SupportsPartialCut bool

	// Online and StatusDetail are a point-in-time snapshot from
	// printer.Printer.Status, taken when ListPrinters was called — not
	// cached, and not refreshed except by calling ListPrinters again
	// (docs/adr/0025-dashboard-updates-via-polling.md).
	Online       bool
	StatusDetail string
}

// AssetSummary is a read-only view of one stored asset for the Web UI's
// asset management page. It carries no content type: which types are safe
// to render inline in a browser is a webui concern
// (docs/adr/0029-asset-content-endpoint-inline-type-allowlist.md), not a
// property of the stored asset.
type AssetSummary struct {
	Name    string
	Size    int64
	ModTime time.Time
}

// QueueSummary is a point-in-time count of every Job the Queue knows
// about, grouped by state, for the Web UI's dashboard/status page
// (docs/adr/0025-dashboard-updates-via-polling.md). It deliberately
// carries only counts, never individual Jobs: a Job's Receipt or
// LastError can carry content this project's trust boundary doesn't
// return verbatim to a client (see internal/api/job_status.go's
// jobFailureMessage sanitization) — a count sidesteps that question
// rather than re-deciding it here for a second surface.
type QueueSummary struct {
	Pending   int
	Running   int
	Done      int
	Failed    int
	Cancelled int
}

// Total is every Job QueueSummary accounts for, across all states.
func (s QueueSummary) Total() int {
	return s.Pending + s.Running + s.Done + s.Failed + s.Cancelled
}
