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
	// Printers maps a Job's PrinterName to the already-constructed Printer
	// instance Process sends encoded bytes to. cmd/receiptd builds each
	// entry once at startup from its Connection (docs/ARCHITECTURE.md §4
	// step 8f) — Service never sees a Connection itself. A nil map (the
	// zero value) is safe to read from: a PrinterName with no entry is
	// reported by Process as apperr.KindNotFound, not a panic.
	Printers map[string]printer.Printer
	// Profiles maps a Job's PrinterName to the printer.Profile Process
	// resolves before encoding (docs/ARCHITECTURE.md §4 step 8a) and passes
	// to escpos.Encode, the same key space as Printers. A nil map is safe
	// to read from: a PrinterName with no entry is reported by Process as
	// apperr.KindNotFound, not a panic.
	Profiles map[string]printer.Profile
	// Assets resolves a receipt.Asset's Name to its bytes (docs/ARCHITECTURE.md
	// §3 "Image vs. Asset"), passed straight through to layout.Build by
	// render. A nil Assets is safe unless a Receipt actually contains an
	// Asset element — cmd/receiptd's composition root always supplies one
	// (assets.NewFilesystemStore(cfg.Assets.Path)); only a Service built
	// directly by a test, the same way Printers/Profiles are already left
	// unset in tests that don't need them, may leave this nil.
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
