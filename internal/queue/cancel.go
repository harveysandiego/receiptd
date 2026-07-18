package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// Cancel transitions the Job identified by id from JobPending to
// JobCancelled and persists that transition via the Queue's Store. Per
// docs/adr/0003-print-queue.md, pending -> cancelled is the only
// cancellation transition: a Running job can't be cancelled because it
// can't be un-printed, and Done, Failed, and Cancelled are already
// terminal. Cancel returns an apperr.KindValidation error, and leaves the
// Job untouched, if it is found but not Pending; it returns whatever
// error the Store returns, unwrapped, if the Job can't be found or
// persisted (e.g. apperr.KindNotFound for an unknown id).
func (q *Queue) Cancel(ctx context.Context, id string) error {
	j, err := q.store.Get(ctx, id)
	if err != nil {
		return err
	}

	if j.State != JobPending {
		return apperr.Wrap(apperr.KindValidation, "queue.Cancel", fmt.Errorf("job %q is %s, only a pending job can be cancelled", id, j.State))
	}

	j.State = JobCancelled
	j.UpdatedAt = time.Now()
	return q.store.Save(ctx, j)
}
