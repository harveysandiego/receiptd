// Command receipt is Receiptd's CLI client: it talks to a running
// receiptd's REST API for most operations, and can also render a Receipt
// to a local PNG preview offline via render/layout and render/canvas
// without a running daemon.
package main

import "os"

// version, commit, and date are overridden via -ldflags at build time by
// .goreleaser.yml (main.version, main.commit, main.date); they surface via
// the root command's --version flag.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
