package app

import (
	"context"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

// Preview renders r into PNG bytes for inspection, without enqueuing a Job
// or touching a printer. It shares render with Process (see process.go) so
// there is exactly one rendering path from a Receipt to pixels.
func (s *Service) Preview(ctx context.Context, r receipt.Receipt) ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}

	c, err := s.render(r)
	if err != nil {
		return nil, err
	}

	return c.EncodePNG()
}
