package app

import (
	"context"

	"github.com/harveysandiego/receiptd/internal/queue"
)

// JobStatus returns the current state of the Job identified by id, as
// held by the Service's Queue. It is a read-only lookup: it never
// transitions Job state, never persists a change, and never invokes the
// Processor.
func (s *Service) JobStatus(ctx context.Context, id string) (*queue.Job, error) {
	return s.Queue.Get(ctx, id)
}
