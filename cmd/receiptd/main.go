// Command receiptd is Receiptd's composition root. It loads
// configuration, constructs the configured queue Store, wires up
// app.Service — including a printer.Printer and printer.Profile per
// configured printer.Connection — registers the versioned API routes,
// applies Bearer-token middleware when configured, and starts the
// background queue worker alongside the HTTP server. A SIGTERM or SIGINT
// begins a bounded graceful shutdown rather than killing the process
// outright — see docs/adr/0018-graceful-shutdown.md and (*daemon).run.
//
// It is the only place in the codebase that ever constructs a
// printer.Connection — see docs/ARCHITECTURE.md §1.
package main

import (
	"flag"
	"fmt"
	"os"
)

// version, commit, and date are overridden via -ldflags at build time by
// .goreleaser.yml (main.version, main.commit, main.date); they keep
// these placeholder values for a plain `go build`/`go run`.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	configPath := flag.String("config", "/etc/receiptd/config.yaml", "path to Receiptd's YAML configuration file")
	flag.Parse()

	fmt.Printf("receiptd %s (commit %s, built %s)\n", version, commit, date)

	d, err := loadAndBuild(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "receiptd: %v\n", err)
		os.Exit(1)
	}

	// run blocks until the HTTP server stops on its own or a SIGTERM/
	// SIGINT begins docs/adr/0018-graceful-shutdown.md's shutdown
	// sequence; see run's own doc comment (cmd/receiptd/shutdown.go) for
	// the full behavior.
	os.Exit(d.run())
}
