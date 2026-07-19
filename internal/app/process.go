package app

import (
	"context"
	"fmt"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
	"github.com/harveysandiego/receiptd/internal/render/escpos"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// Process resolves the printer.Profile for j.PrinterName, renders
// j.Receipt against it, encodes the rendered Canvas to ESC/POS bytes
// against the same Profile, and sends those bytes to the Printer
// configured for the same PrinterName — the complete Receipt -> Layout ->
// Canvas -> ESC/POS -> Printer pipeline (docs/ARCHITECTURE.md §4 step 8).
// It satisfies queue.Processor (docs/ARCHITECTURE.md §2) and is invoked by
// Queue.ProcessNext, which owns every Job state transition — Process
// itself never reads or writes j.State, j.Attempts, or any other Job
// field besides Receipt and PrinterName.
//
// The Profile is resolved once and reused for both render and
// escpos.Encode, rather than looked up separately for each — both stages
// need the same printer's Profile for the same Job, so there is exactly
// one printer-name lookup per Process call, not two that could in
// principle disagree.
//
// Process is orchestration only: rendering lives in render, encoding in
// escpos.Encode, delivery in the resolved Printer's Send. Each stage's
// error is returned exactly as received, so a caller can still branch on
// its original apperr.Kind.
func (s *Service) Process(ctx context.Context, j *queue.Job) error {
	profile, ok := s.Profiles[j.PrinterName]
	if !ok {
		return apperr.Wrap(apperr.KindNotFound, "app.Process", fmt.Errorf("printer profile %q not configured", j.PrinterName))
	}

	c, err := s.render(j.Receipt, profile)
	if err != nil {
		return err
	}

	data, err := escpos.Encode(c, profile)
	if err != nil {
		return err
	}

	p, ok := s.Printers[j.PrinterName]
	if !ok {
		return apperr.Wrap(apperr.KindNotFound, "app.Process", fmt.Errorf("printer %q not configured", j.PrinterName))
	}

	return p.Send(ctx, data)
}

// render turns r into a rendered Canvas by composing the existing
// rendering pipeline: layout.Build, then canvas.Paint, against profile —
// see printer.Profile.WidthDots's doc comment for what a zero-value
// profile means for the resulting Canvas's width. It uses
// layout.EmbeddedFont, the only Font implementation that exists, and
// performs no I/O — consistent with layout.Build and canvas.Paint
// themselves not yet accepting a context.Context.
func (s *Service) render(r receipt.Receipt, profile printer.Profile) (*canvas.Canvas, error) {
	doc, err := layout.Build(r, profile, layout.EmbeddedFont{})
	if err != nil {
		return nil, err
	}
	return canvas.Paint(doc)
}
