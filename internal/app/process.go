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

// Process renders j.Receipt against j.PrinterName's Profile, encodes the
// Canvas to ESC/POS against that same Profile, and sends the bytes to the
// Printer for that PrinterName — the Receipt -> Layout -> Canvas ->
// ESC/POS -> Printer pipeline (docs/ARCHITECTURE.md §4 step 8). It
// satisfies queue.Processor (docs/ARCHITECTURE.md §2) and is invoked by
// Queue.ProcessNext, which owns every Job state transition — Process
// reads only Receipt and PrinterName, never j.State, j.Attempts, or any
// other Job field.
//
// Process is orchestration only; each stage's error is returned exactly
// as received, so a caller can still branch on its original apperr.Kind.
func (s *Service) Process(ctx context.Context, j *queue.Job) error {
	profile, ok := s.Profiles[j.PrinterName]
	if !ok {
		return apperr.Wrap(apperr.KindNotFound, "app.Process", fmt.Errorf("printer profile %q not configured", j.PrinterName))
	}

	c, err := s.render(ctx, j.Receipt, profile)
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

// render composes the rendering pipeline (layout.Build then canvas.Paint)
// against profile — see printer.Profile.WidthDots for what a zero-value
// profile means for the Canvas width. layout.Build is the only stage that
// performs I/O, resolving any receipt.Asset r contains via s.Assets
// (docs/ARCHITECTURE.md §3 "Image vs. Asset").
func (s *Service) render(ctx context.Context, r receipt.Receipt, profile printer.Profile) (*canvas.Canvas, error) {
	doc, err := layout.Build(ctx, r, profile, layout.EmbeddedFont{}, s.Assets)
	if err != nil {
		return nil, err
	}
	return canvas.Paint(doc)
}
