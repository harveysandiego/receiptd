// Command receiptd is Receiptd's composition root. As of Milestone 2 it
// loads configuration, constructs the configured queue Store, wires up
// app.Service (with a log-file stand-in for a real printer), registers
// the versioned API routes, applies Bearer-token middleware when
// configured, and starts the background queue worker alongside the HTTP
// server.
//
// It will also become the only place in the codebase that ever
// constructs a printer.Connection, once Milestone 3 adds a real printer
// transport — see docs/ARCHITECTURE.md §1.
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

	if err := d.serve(); err != nil {
		fmt.Fprintf(os.Stderr, "receiptd: %v\n", err)
		os.Exit(1)
	}
}
