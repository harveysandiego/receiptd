package app

import (
	"context"

	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// Process renders j.Receipt via the existing rendering pipeline. It
// satisfies queue.Processor (docs/ARCHITECTURE.md §2) and is invoked by
// Queue.ProcessNext, which owns every Job state transition — Process
// itself never reads or writes j.State, j.Attempts, or any other Job
// field besides Receipt, and never touches a printer.
//
// This slice ends at a rendered Canvas: no printer.Profile lookup,
// escpos encoding, or printer.Printer exists yet to carry the result
// further, so a successful render's Canvas is discarded here. See
// render for the actual rendering step, and docs/ARCHITECTURE.md §4 for
// the steps this Process will grow into.
func (s *Service) Process(ctx context.Context, j *queue.Job) error {
	_, err := s.render(j.Receipt)
	return err
}

// render turns r into a rendered Canvas by composing the existing
// rendering pipeline: layout.Build, then canvas.Paint. It uses
// layout.EmbeddedFont, the only Font implementation that exists, and
// performs no I/O — consistent with layout.Build and canvas.Paint
// themselves not yet accepting a context.Context.
func (s *Service) render(r receipt.Receipt) (*canvas.Canvas, error) {
	doc, err := layout.Build(r, layout.EmbeddedFont{})
	if err != nil {
		return nil, err
	}
	return canvas.Paint(doc)
}
