package app

import (
	"context"
	"fmt"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// Preview renders r into PNG bytes for inspection, without enqueuing a Job
// or touching a printer. It shares render with Process (see process.go) so
// there is exactly one rendering path from a Receipt to pixels — resolving
// printerName's Profile the same way Process resolves Job.PrinterName's,
// per docs/adr/0006-preview-requires-printer-profile.md. r is validated
// before printerName is resolved, so an invalid Receipt is reported as
// such even against an unconfigured or misspelled printer name.
func (s *Service) Preview(ctx context.Context, r receipt.Receipt, printerName string) ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}

	profile, ok := s.Profiles[printerName]
	if !ok {
		return nil, apperr.Wrap(apperr.KindNotFound, "app.Preview", fmt.Errorf("printer profile %q not configured", printerName))
	}

	c, err := s.render(r, profile)
	if err != nil {
		return nil, err
	}

	return c.EncodePNG()
}
