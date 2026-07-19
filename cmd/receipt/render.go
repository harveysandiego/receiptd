package main

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// newRenderCmd builds the "render" subcommand: an offline path from a
// Receipt JSON file to a PNG preview that needs no running daemon, using
// exactly the same layout.Build + canvas.Paint pipeline app.Service uses
// server-side (docs/ARCHITECTURE.md §4). It has no config or daemon to
// resolve a real printer.Profile from, so it renders against the
// zero-value Profile — printer.Profile.WidthDots's documented "no printer
// configured" case — producing the same content-fit PNG this command has
// always produced.
func newRenderCmd() *cobra.Command {
	var out string

	cmd := &cobra.Command{
		Use:   "render <receipt.json>",
		Short: "Render a Receipt to a PNG preview, offline",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRender(args[0], out)
		},
	}

	cmd.Flags().StringVar(&out, "out", "", "path to write the rendered PNG to (required)")
	if err := cmd.MarkFlagRequired("out"); err != nil {
		panic(err)
	}

	return cmd
}

// runRender reads and validates the Receipt at inPath, renders it via the
// existing local pipeline, and writes the resulting PNG to outPath. It
// writes nothing to outPath unless every prior step succeeds.
func runRender(inPath, outPath string) error {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}

	var r receipt.Receipt
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}

	if err := r.Validate(); err != nil {
		return err
	}

	doc, err := layout.Build(r, printer.Profile{}, layout.EmbeddedFont{})
	if err != nil {
		return err
	}

	c, err := canvas.Paint(doc)
	if err != nil {
		return err
	}

	png, err := c.EncodePNG()
	if err != nil {
		return err
	}

	return os.WriteFile(outPath, png, 0o644)
}
