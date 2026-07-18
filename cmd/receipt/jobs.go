package main

import (
	"context"
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/harveysandiego/receiptd/internal/config"
)

// newJobsCmd builds the "jobs" subcommand: GET a print Job's current
// status from a running receiptd via its API and print it as JSON. This
// is a single point-in-time query — no polling or waiting for completion,
// which is out of scope for this slice.
func newJobsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jobs <job-id>",
		Short: "Show a print Job's current status via receiptd's API",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJobs(cmd, args[0])
		},
	}
	return cmd
}

// runJobs fetches the current status of the Job identified by id and
// prints it as indented JSON.
func runJobs(cmd *cobra.Command, id string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	client, err := newAPIClient(cfg)
	if err != nil {
		return err
	}

	job, err := client.jobStatus(context.Background(), id)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(job)
}
