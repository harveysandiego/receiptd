package queue

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Reconcile resolves every Job found JobRunning into Pending (another
// chance) or Failed (budget exhausted), treating the interruption itself
// as one consumed retry attempt. cmd/receiptd must run this to completion,
// synchronously, before any worker starts claiming Jobs — see
// docs/adr/0017-queue-lifecycle-crash-recovery.md, which this implements,
// for why every Job found Running at that point is unconditionally
// orphaned. It returns how many Jobs were retried vs. failed, and
// propagates the first Store error without reconciling past it.
func (q *Queue) Reconcile(ctx context.Context) (retried, failed int, err error) {
	jobs, err := q.store.List(ctx, Filter{})
	if err != nil {
		return 0, 0, err
	}

	now := time.Now()
	for _, j := range jobs {
		if j.State != JobRunning {
			continue
		}

		j.Attempts++
		j.LastError = fmt.Sprintf("interrupted: daemon restarted while this job was running (attempt %d of %d)", j.Attempts, q.maxAttempts)
		if j.Attempts < q.maxAttempts {
			j.State = JobPending
			retried++
		} else {
			j.State = JobFailed
			failed++
		}
		j.UpdatedAt = now

		if err := q.store.Save(ctx, j); err != nil {
			return retried, failed, err
		}
	}

	if retried+failed > 0 {
		log.Printf("queue: reconciled %d orphaned running job(s) at startup: %d retried, %d failed", retried+failed, retried, failed)
	}
	return retried, failed, nil
}
