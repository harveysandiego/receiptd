package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/harveysandiego/receiptd/internal/config"
)

// newVersionCmd builds the "version" subcommand: this CLI's own build
// identity plus the daemon's, so a bug report can name both.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show this CLI's version and the daemon's",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVersion(cmd)
		},
	}
}

// runVersion never fails on an unreachable daemon: reporting this binary's
// version is the command's primary job, and a daemon that won't start is
// exactly when someone runs it.
func runVersion(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "receipt %s (commit %s, built %s)\n", version, commit, date); err != nil {
		return err
	}

	server, err := serverVersion()
	if err != nil {
		_, writeErr := fmt.Fprintf(out, "receiptd unavailable (%v)\n", err)
		return writeErr
	}

	_, err = fmt.Fprintf(out, "receiptd %s (commit %s, built %s)\n", server.Version, server.Commit, server.Date)
	return err
}

func serverVersion() (*versionResponse, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	client, err := newAPIClient(cfg)
	if err != nil {
		return nil, err
	}

	return client.serverVersion(context.Background())
}
