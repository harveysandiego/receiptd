package app

import (
	"context"

	"github.com/harveysandiego/receiptd/internal/assets"
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
	// Printers maps a Job's PrinterName to its already-constructed Printer.
	// cmd/receiptd builds each entry at startup from its Connection
	// (docs/ARCHITECTURE.md §4 step 8f); Service never sees a Connection. A
	// nil map is safe to read from: an unknown PrinterName is reported by
	// Process as apperr.KindNotFound, not a panic.
	Printers map[string]printer.Printer
	// Profiles maps a Job's PrinterName to the Profile Process passes to
	// escpos.Encode (docs/ARCHITECTURE.md §4 step 8a), the same key space
	// as Printers. A nil map is safe to read from: an unknown PrinterName
	// is reported as apperr.KindNotFound, not a panic.
	Profiles map[string]printer.Profile
	// Assets resolves a receipt.Asset's Name to its bytes
	// (docs/ARCHITECTURE.md §3 "Image vs. Asset"), passed through to
	// layout.Build by render. A nil Assets is safe unless a Receipt
	// actually contains an Asset element; cmd/receiptd always supplies one,
	// and only a test that doesn't need it may leave this nil.
	Assets assets.Store
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
