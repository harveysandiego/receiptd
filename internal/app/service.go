package app

import (
	"context"
	"io"

	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// Service is Receiptd's service/use-case layer. See doc.go for its role in
// the architecture. Only the fields needed by the current slice are
// present; docs/ARCHITECTURE.md §2 specifies the full shape this struct
// grows into as later slices land.
type Service struct {
	Queue *queue.Queue
	// LogSink is where Process writes each Job's rendered output, standing
	// in for a real printer until Milestone 3 wires up printer.Printer and
	// render/escpos (docs/ARCHITECTURE.md §10: "Process writes to a log
	// file instead of a real printer"). New defaults it to io.Discard; the
	// composition root is expected to replace it with the configured fake
	// printer log file.
	LogSink io.Writer
}

// New returns a Service that enqueues print work via queue. LogSink
// defaults to io.Discard, so Process is safe to call before the
// composition root configures a real log destination.
func New(q *queue.Queue) *Service {
	return &Service{Queue: q, LogSink: io.Discard}
}

// Print validates r, constructs a Job for it targeting printerName, and
// enqueues it via the Service's Queue. It returns the enqueued Job's ID.
// Print only enqueues work — it never processes it.
func (s *Service) Print(ctx context.Context, r receipt.Receipt, printerName string) (jobID string, err error) {
	if err := r.Validate(); err != nil {
		return "", err
	}

	j := &queue.Job{PrinterName: printerName, Receipt: r}
	if err := s.Queue.Enqueue(ctx, j); err != nil {
		return "", err
	}

	return j.ID, nil
}
