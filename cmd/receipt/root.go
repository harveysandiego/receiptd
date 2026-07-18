package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// configPath is bound to the root command's persistent --config flag. The
// API-backed subcommands (preview, print, jobs) read it to load the same
// YAML configuration receiptd itself loads, matching cmd/receiptd's own
// --config flag and default. "render" ignores it — it never talks to a
// daemon.
var configPath string

// newRootCmd builds the receipt command tree. It performs no I/O itself and
// never calls os.Exit, so it can be executed directly in tests via
// cmd.SetArgs + cmd.Execute().
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "receipt",
		Short:   "Receiptd's CLI client",
		Version: fmt.Sprintf("%s (commit %s, built %s)", version, commit, date),
	}

	cmd.PersistentFlags().StringVar(&configPath, "config", "/etc/receiptd/config.yaml", "path to Receiptd's YAML configuration file")

	cmd.AddCommand(newRenderCmd())
	cmd.AddCommand(newPreviewCmd())
	cmd.AddCommand(newPrintCmd())
	cmd.AddCommand(newJobsCmd())

	return cmd
}
