package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"github.com/harveysandiego/receiptd/internal/config"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// newPreviewCmd builds the "preview" subcommand: POST a Receipt to a
// running receiptd's /api/v1/preview and write the PNG it returns to
// --out. Unlike "render" (render.go), this needs a running daemon and
// does no rendering itself — all rendering happens server-side. --printer
// is required, matching "print": Preview renders against a specific
// printer's Profile, so there is no printer-agnostic preview to produce —
// see docs/adr/0006-preview-requires-printer-profile.md.
func newPreviewCmd() *cobra.Command {
	var out string
	var printerName string

	cmd := &cobra.Command{
		Use:   "preview <receipt.json>",
		Short: "Render a Receipt to a PNG preview via receiptd's API",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPreview(args[0], out, printerName)
		},
	}

	cmd.Flags().StringVar(&out, "out", "", "path to write the returned PNG to (required)")
	if err := cmd.MarkFlagRequired("out"); err != nil {
		panic(err)
	}
	cmd.Flags().StringVar(&printerName, "printer", "", "name of the printer to preview against (required)")
	if err := cmd.MarkFlagRequired("printer"); err != nil {
		panic(err)
	}

	return cmd
}

// runPreview reads the Receipt at inPath, POSTs it and printerName to
// receiptd's /api/v1/preview, and writes the resulting PNG to outPath. It
// writes nothing to outPath unless every prior step succeeds.
func runPreview(inPath, outPath, printerName string) error {
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

	png, err := client.preview(context.Background(), r, printerName)
	if err != nil {
		return err
	}

	return os.WriteFile(outPath, png, 0o644)
}

// readReceiptFile reads and JSON-decodes the Receipt at path. It performs
// no validation — Receipt validation happens server-side, inside
// app.Service, for every API-backed command.
func readReceiptFile(path string) (receipt.Receipt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return receipt.Receipt{}, err
	}

	var r receipt.Receipt
	if err := json.Unmarshal(data, &r); err != nil {
		return receipt.Receipt{}, err
	}
	return r, nil
}
