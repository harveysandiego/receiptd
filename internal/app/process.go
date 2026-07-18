package app

import (
	"context"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// Process renders j.Receipt via the existing rendering pipeline and writes
// the result to LogSink. It satisfies queue.Processor (docs/ARCHITECTURE.md
// §2) and is invoked by Queue.ProcessNext, which owns every Job state
// transition — Process itself never reads or writes j.State, j.Attempts,
// or any other Job field besides Receipt.
//
// This slice ends at LogSink, not a real printer: no printer.Profile
// lookup, escpos encoding, or printer.Printer exists yet to carry the
// result further (docs/ARCHITECTURE.md §10 — that's Milestone 3). LogSink
// is Milestone 2's stand-in, written as PNG bytes via the same
// canvas.EncodePNG call Preview uses, so there is exactly one encoding
// path from a Canvas to bytes as well as one rendering path from a
// Receipt to a Canvas.
func (s *Service) Process(ctx context.Context, j *queue.Job) error {
	c, err := s.render(j.Receipt)
	if err != nil {
		return err
	}

	out, err := c.EncodePNG()
	if err != nil {
		return err
	}

	if _, err := s.LogSink.Write(out); err != nil {
		return apperr.Wrap(apperr.KindPermanent, "app.Process", err)
	}
	return nil
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
