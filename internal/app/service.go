package app

import (
	"context"

	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// Service is Receiptd's service/use-case layer. See doc.go for its role in
// the architecture. Only the fields needed by the current slice are
// present; docs/ARCHITECTURE.md §2 specifies the full shape this struct
// grows into as later slices land.
type Service struct {
	Queue *queue.Queue
	// Printers maps a Job's PrinterName to the already-constructed Printer
	// instance Process sends encoded bytes to. cmd/receiptd builds each
	// entry once at startup from its Connection (docs/ARCHITECTURE.md §4
	// step 8f) — Service never sees a Connection itself. A nil map (the
	// zero value) is safe to read from: a PrinterName with no entry is
	// reported by Process as apperr.KindNotFound, not a panic.
	Printers map[string]printer.Printer
}

// New returns a Service that enqueues print work via queue.
func New(q *queue.Queue) *Service {
	return &Service{Queue: q}
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
