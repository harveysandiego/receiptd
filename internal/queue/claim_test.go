// This file is package queue (not queue_test, unlike most of this
// package's tests) so it can reuse newTestBoltStore's t.Cleanup-based
// Close handling (store_bolt_test.go) — running the same table of
// assertions against both Store backends without leaking a bbolt file
// handle per subtest.
package queue

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// claimTestStores returns every Store backend ClaimNextPending's atomicity
// invariant must hold for — today's two, bolt and memory. docs/adr/
// 0016-queue-concurrency-per-printer-workers.md's Consequences section
// calls out exactly this kind of test (many concurrent claim attempts,
// same printer and different printers, against every storage backend) as
// the main defense against a subtly broken implementation.
func claimTestStores(t *testing.T) map[string]Store {
	t.Helper()
	return map[string]Store{
		"memory": NewMemoryStore(),
		"bolt":   newTestBoltStore(t),
	}
}

// mustSave saves j into s, using context.Background() like this package's
// existing mustEnqueue helper (queue_test.go) does.
func mustSave(t *testing.T, s Store, j *Job) {
	t.Helper()
	if err := s.Save(context.Background(), j); err != nil {
		t.Fatalf("Save(%q) error = %v, want nil", j.ID, err)
	}
}

func TestClaimNextPending_ReturnsLowestIDPendingJobScopedToPrinter(t *testing.T) {
	for name, s := range claimTestStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			mustSave(t, s, &Job{ID: "job-2", PrinterName: "front-desk", State: JobPending})
			mustSave(t, s, &Job{ID: "job-3", PrinterName: "front-desk", State: JobPending})
			mustSave(t, s, &Job{ID: "job-1", PrinterName: "front-desk", State: JobPending})
			// Lower ID than any front-desk Job, but a different printer —
			// must never be returned for a "front-desk" claim.
			mustSave(t, s, &Job{ID: "job-0", PrinterName: "kitchen", State: JobPending})

			got, err := s.ClaimNextPending(ctx, "front-desk")
			if err != nil {
				t.Fatalf("ClaimNextPending() error = %v, want nil", err)
			}
			if got == nil {
				t.Fatal("ClaimNextPending() = nil, want job-1")
			}
			if got.ID != "job-1" {
				t.Errorf("ClaimNextPending().ID = %q, want %q", got.ID, "job-1")
			}
		})
	}
}

func TestClaimNextPending_TransitionsToRunningAndPersists(t *testing.T) {
	for name, s := range claimTestStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			mustSave(t, s, &Job{ID: "job-1", PrinterName: "front-desk", State: JobPending})

			got, err := s.ClaimNextPending(ctx, "front-desk")
			if err != nil {
				t.Fatalf("ClaimNextPending() error = %v, want nil", err)
			}
			if got.State != JobRunning {
				t.Errorf("ClaimNextPending().State = %v, want %v", got.State, JobRunning)
			}
			if got.UpdatedAt.IsZero() {
				t.Error("ClaimNextPending().UpdatedAt is zero, want it stamped")
			}

			persisted, err := s.Get(ctx, "job-1")
			if err != nil {
				t.Fatalf("Get() error = %v, want nil", err)
			}
			if persisted.State != JobRunning {
				t.Errorf("Get().State = %v after ClaimNextPending, want %v (the transition must be persisted, not just returned)", persisted.State, JobRunning)
			}
		})
	}
}

func TestClaimNextPending_DoesNotClaimJobsForADifferentPrinter(t *testing.T) {
	for name, s := range claimTestStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			mustSave(t, s, &Job{ID: "job-1", PrinterName: "kitchen", State: JobPending})

			got, err := s.ClaimNextPending(ctx, "front-desk")
			if err != nil {
				t.Fatalf("ClaimNextPending() error = %v, want nil", err)
			}
			if got != nil {
				t.Errorf("ClaimNextPending() = %+v, want nil (job-1 belongs to a different printer)", *got)
			}

			// The job must still be exactly as it was: untouched, still
			// pending, still claimable by its own printer's worker.
			persisted, err := s.Get(ctx, "job-1")
			if err != nil {
				t.Fatalf("Get() error = %v, want nil", err)
			}
			if persisted.State != JobPending {
				t.Errorf("Get().State = %v, want unchanged %v", persisted.State, JobPending)
			}
		})
	}
}

func TestClaimNextPending_IgnoresNonPendingJobs(t *testing.T) {
	for name, s := range claimTestStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			mustSave(t, s, &Job{ID: "job-1", PrinterName: "front-desk", State: JobDone})
			mustSave(t, s, &Job{ID: "job-2", PrinterName: "front-desk", State: JobRunning})
			mustSave(t, s, &Job{ID: "job-3", PrinterName: "front-desk", State: JobFailed})

			got, err := s.ClaimNextPending(ctx, "front-desk")
			if err != nil {
				t.Fatalf("ClaimNextPending() error = %v, want nil", err)
			}
			if got != nil {
				t.Errorf("ClaimNextPending() = %+v, want nil (no Job is Pending)", *got)
			}
		})
	}
}

func TestClaimNextPending_EmptyStore_ReturnsNilNil(t *testing.T) {
	for name, s := range claimTestStores(t) {
		t.Run(name, func(t *testing.T) {
			got, err := s.ClaimNextPending(context.Background(), "front-desk")
			if err != nil {
				t.Fatalf("ClaimNextPending() error = %v, want nil", err)
			}
			if got != nil {
				t.Errorf("ClaimNextPending() = %+v, want nil", *got)
			}
		})
	}
}

func TestClaimNextPending_DoesNotAliasStoredJob(t *testing.T) {
	for name, s := range claimTestStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			mustSave(t, s, &Job{ID: "job-1", PrinterName: "front-desk", State: JobPending})

			got, err := s.ClaimNextPending(ctx, "front-desk")
			if err != nil {
				t.Fatalf("ClaimNextPending() error = %v, want nil", err)
			}
			got.LastError = "mutated by caller"

			persisted, err := s.Get(ctx, "job-1")
			if err != nil {
				t.Fatalf("Get() error = %v, want nil", err)
			}
			if persisted.LastError != "" {
				t.Errorf("Get().LastError = %q after mutating a ClaimNextPending() result, want unchanged \"\"", persisted.LastError)
			}
		})
	}
}

// TestClaimNextPending_ConcurrentCallers_NeverDoubleClaim is the main
// defense docs/adr/0016-queue-concurrency-per-printer-workers.md's
// Consequences section calls for: many goroutines claim concurrently,
// same and different printer names, against a Store pre-seeded with a
// known Job count per printer; no Job may ever be claimed twice. Run with
// -race so a broken atomicity split (e.g. two db.Update calls, or
// separate RLock/Lock sections) is caught even if the assertion isn't.
func TestClaimNextPending_ConcurrentCallers_NeverDoubleClaim(t *testing.T) {
	const jobsPerPrinter = 10
	const callersPerPrinter = 25 // > jobsPerPrinter, so contention is real: some callers get (nil, nil).

	for name, s := range claimTestStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			printers := []string{"front-desk", "kitchen"}
			for _, p := range printers {
				for i := 0; i < jobsPerPrinter; i++ {
					mustSave(t, s, &Job{ID: fmt.Sprintf("%s-job-%03d", p, i), PrinterName: p, State: JobPending})
				}
			}

			var (
				mu      sync.Mutex
				claimed = make(map[string]int) // Job ID -> number of callers it was returned to.
				wg      sync.WaitGroup
			)
			for _, p := range printers {
				for i := 0; i < callersPerPrinter; i++ {
					wg.Add(1)
					go func(printerName string) {
						defer wg.Done()
						got, err := s.ClaimNextPending(ctx, printerName)
						if err != nil {
							t.Errorf("ClaimNextPending(%q) error = %v, want nil", printerName, err)
							return
						}
						if got == nil {
							return
						}
						mu.Lock()
						claimed[got.ID]++
						mu.Unlock()
					}(p)
				}
			}
			wg.Wait()

			for id, n := range claimed {
				if n > 1 {
					t.Errorf("job %q was claimed by %d concurrent callers, want at most 1", id, n)
				}
			}
			if len(claimed) != len(printers)*jobsPerPrinter {
				t.Errorf("len(claimed) = %d, want %d (every seeded Job claimed exactly once across all callers)", len(claimed), len(printers)*jobsPerPrinter)
			}
		})
	}
}
