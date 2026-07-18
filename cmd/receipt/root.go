package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newRootCmd builds the receipt command tree. It performs no I/O itself and
// never calls os.Exit, so it can be executed directly in tests via
// cmd.SetArgs + cmd.Execute().
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "receipt",
		Short:   "Receiptd's CLI client",
		Version: fmt.Sprintf("%s (commit %s, built %s)", version, commit, date),
	}

	cmd.AddCommand(newRenderCmd())

	return cmd
}
