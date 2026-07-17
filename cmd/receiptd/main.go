// Command receiptd is Receiptd's composition root: it loads
// configuration, constructs printer.Connections and turns them into
// printer.Printer instances, registers templates and providers via blank
// imports, wires up app.Service, and starts the queue worker and HTTP
// server.
//
// It is the only place in the entire codebase that ever constructs a
// printer.Connection — see docs/ARCHITECTURE.md §1.
//
// No functionality is implemented yet; this is a build/test skeleton
// ahead of Milestone 1. See docs/ARCHITECTURE.md for the roadmap.
package main

import "fmt"

// version, commit, and date are overridden via -ldflags at build time by
// .goreleaser.yml (main.version, main.commit, main.date); they keep
// these placeholder values for a plain `go build`/`go run`.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	fmt.Printf("receiptd %s (commit %s, built %s) — not yet implemented\n", version, commit, date)
}
