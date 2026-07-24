package app

import (
	"context"

	"github.com/harveysandiego/receiptd/internal/queue"
)

// QueueSummary returns a point-in-time count of every Job the Queue
// knows about, grouped by state, for the Web UI's dashboard/status page
// (docs/adr/0025-dashboard-updates-via-polling.md). A Job in a state
// outside the closed set queue.JobState defines is impossible by
// construction (queue.Store only ever writes one of those values), so no
// "unknown state" bucket exists here.
func (s *Service) QueueSummary(ctx context.Context) (QueueSummary, error) {
	jobs, err := s.Queue.List(ctx)
	if err != nil {
		return QueueSummary{}, err
	}

	var summary QueueSummary
	for _, j := range jobs {
		switch j.State {
		case queue.JobPending:
			summary.Pending++
		case queue.JobRunning:
			summary.Running++
		case queue.JobDone:
			summary.Done++
		case queue.JobFailed:
			summary.Failed++
		case queue.JobCancelled:
			summary.Cancelled++
		}
	}
	return summary, nil
}
