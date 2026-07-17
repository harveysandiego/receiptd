// Command receipt is Receiptd's CLI client: it talks to a running
// receiptd's REST API for most operations, and can also render a Receipt
// to a local PNG preview offline via render/layout and render/canvas
// without a running daemon.
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
	fmt.Printf("receipt %s (commit %s, built %s) — not yet implemented\n", version, commit, date)
}
