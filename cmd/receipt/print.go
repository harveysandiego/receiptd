package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/harveysandiego/receiptd/internal/config"
)

// newPrintCmd builds the "print" subcommand: POST a Receipt to a running
// receiptd's /api/v1/print for the named printer, and report the queued
// Job's ID. Printing always goes through the API — the CLI holds no
// printer or queue logic of its own.
func newPrintCmd() *cobra.Command {
	var printerName string

	cmd := &cobra.Command{
		Use:   "print <receipt.json>",
		Short: "Submit a Receipt to receiptd's print queue via its API",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrint(cmd, args[0], printerName)
		},
	}

	cmd.Flags().StringVar(&printerName, "printer", "", "name of the printer to print to (required)")
	if err := cmd.MarkFlagRequired("printer"); err != nil {
		panic(err)
	}

	return cmd
}

// runPrint reads the Receipt at inPath, POSTs it to receiptd's
// /api/v1/print for printerName, and reports the queued Job's ID.
func runPrint(cmd *cobra.Command, inPath, printerName string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	r, err := readReceiptFile(inPath)
	if err != nil {
		return err
	}

	client, err := newAPIClient(cfg)
	if err != nil {
		return err
	}

	jobID, err := client.print(context.Background(), r, printerName)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(cmd.OutOrStdout(), jobID)
	return err
}
