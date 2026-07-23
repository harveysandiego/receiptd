// This file exercises Store.EnqueueIdempotent — the atomic dedup
// operation docs/adr/0020-idempotent-print-requests.md introduces — against
// both storage backends, driven entirely through the exported Store
// interface (no backend-specific internals needed, unlike
// store_bolt_test.go's reopen test).
package queue_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// idempotentStoreConstructors lists both Store backends so every
// EnqueueIdempotent behavior test below runs against each — the ADR
// requires identical semantics from both.
func idempotentStoreConstructors(t *testing.T) map[string]func() queue.Store {
	t.Helper()
	return map[string]func() queue.Store{
		"memoryStore": queue.NewMemoryStore,
		"boltStore": func() queue.Store {
			s, err := queue.NewBoltStore(t.TempDir() + "/queue.db")
			if err != nil {
				t.Fatalf("NewBoltStore() error = %v, want nil", err)
			}
			return s
		},
	}
}

func testReceipt(content string) receipt.Receipt {
	return receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: content}}}
}

func forEachIdempotentStore(t *testing.T, run func(t *testing.T, newStore func() queue.Store)) {
	t.Helper()
	for name, newStore := range idempotentStoreConstructors(t) {
		t.Run(name, func(t *testing.T) {
			run(t, newStore)
		})
	}
}

func TestStore_EnqueueIdempotent_NoKey_AlwaysCreates(t *testing.T) {
	forEachIdempotentStore(t, func(t *testing.T, newStore func() queue.Store) {
		s := newStore()
		ctx := context.Background()
		now := time.Now()

		j1 := &queue.Job{PrinterName: "front-desk", Receipt: testReceipt("hello")}
		got1, created1, err := s.EnqueueIdempotent(ctx, j1, now)
		if err != nil {
			t.Fatalf("EnqueueIdempotent() (1st, no key) error = %v, want nil", err)
		}
		if !created1 {
			t.Error("created (1st, no key) = false, want true")
		}
		if got1.ID == "" {
			t.Error("got1.ID = \"\", want a generated ID")
		}
		if got1.State != queue.JobPending {
			t.Errorf("got1.State = %v, want %v", got1.State, queue.JobPending)
		}

		j2 := &queue.Job{PrinterName: "front-desk", Receipt: testReceipt("hello")}
		got2, created2, err := s.EnqueueIdempotent(ctx, j2, now)
		if err != nil {
			t.Fatalf("EnqueueIdempotent() (2nd, no key) error = %v, want nil", err)
		}
		if !created2 {
			t.Error("created (2nd, no key) = false, want true (no key means always create, even for an identical Receipt)")
		}
		if got2.ID == got1.ID {
			t.Errorf("got2.ID == got1.ID == %q, want distinct IDs for two no-key calls", got1.ID)
		}
	})
}

func TestStore_EnqueueIdempotent_NewKey_Creates(t *testing.T) {
	forEachIdempotentStore(t, func(t *testing.T, newStore func() queue.Store) {
		s := newStore()
		ctx := context.Background()

		j := &queue.Job{PrinterName: "front-desk", Receipt: testReceipt("hello"), IdempotencyKey: "key-1"}
		got, created, err := s.EnqueueIdempotent(ctx, j, time.Now())
		if err != nil {
			t.Fatalf("EnqueueIdempotent() error = %v, want nil", err)
		}
		if !created {
			t.Error("created = false, want true for a brand-new key")
		}
		if got.ID == "" {
			t.Error("got.ID = \"\", want a generated ID")
		}
		if got.IdempotencyKey != "key-1" {
			t.Errorf("got.IdempotencyKey = %q, want %q", got.IdempotencyKey, "key-1")
		}
		if got.State != queue.JobPending {
			t.Errorf("got.State = %v, want %v", got.State, queue.JobPending)
		}

		stored, err := s.Get(ctx, got.ID)
		if err != nil {
			t.Fatalf("store.Get(%q) error = %v, want nil", got.ID, err)
		}
		if stored.IdempotencyKey != "key-1" {
			t.Errorf("stored.IdempotencyKey = %q, want %q", stored.IdempotencyKey, "key-1")
		}
	})
}

func TestStore_EnqueueIdempotent_MatchingKey_ReturnsExistingWithoutCreating(t *testing.T) {
	forEachIdempotentStore(t, func(t *testing.T, newStore func() queue.Store) {
		for _, state := range []queue.JobState{queue.JobPending, queue.JobRunning, queue.JobDone} {
			t.Run(string(state), func(t *testing.T) {
				s := newStore()
				ctx := context.Background()
				now := time.Now()
				r := testReceipt("hello")

				seeded := &queue.Job{
					ID:             "existing-job",
					PrinterName:    "front-desk",
					Receipt:        r,
					State:          state,
					IdempotencyKey: "key-1",
					CreatedAt:      now,
					UpdatedAt:      now,
				}
				if err := s.Save(ctx, seeded); err != nil {
					t.Fatalf("Save() error = %v, want nil", err)
				}

				retry := &queue.Job{PrinterName: "front-desk", Receipt: r, IdempotencyKey: "key-1"}
				got, created, err := s.EnqueueIdempotent(ctx, retry, now)
				if err != nil {
					t.Fatalf("EnqueueIdempotent() error = %v, want nil", err)
				}
				if created {
					t.Error("created = true, want false for a matching, non-expired key")
				}
				if got.ID != seeded.ID {
					t.Errorf("got.ID = %q, want the existing Job's ID %q", got.ID, seeded.ID)
				}
				if got.State != state {
					t.Errorf("got.State = %v, want unchanged %v", got.State, state)
				}

				jobs, err := s.List(ctx, queue.Filter{})
				if err != nil {
					t.Fatalf("List() error = %v, want nil", err)
				}
				if len(jobs) != 1 {
					t.Errorf("len(List()) = %d, want 1 (a matching key must never create a second Job)", len(jobs))
				}
			})
		}
	})
}

func TestStore_EnqueueIdempotent_MatchingFailedJob_ReactivatesInPlace(t *testing.T) {
	forEachIdempotentStore(t, func(t *testing.T, newStore func() queue.Store) {
		s := newStore()
		ctx := context.Background()
		created := time.Now().Add(-time.Hour)
		r := testReceipt("hello")

		seeded := &queue.Job{
			ID:             "existing-job",
			PrinterName:    "front-desk",
			Receipt:        r,
			State:          queue.JobFailed,
			Attempts:       3,
			LastError:      "printer offline",
			IdempotencyKey: "key-1",
			CreatedAt:      created,
			UpdatedAt:      created,
		}
		if err := s.Save(ctx, seeded); err != nil {
			t.Fatalf("Save() error = %v, want nil", err)
		}

		now := time.Now()
		retry := &queue.Job{PrinterName: "front-desk", Receipt: r, IdempotencyKey: "key-1"}
		got, wasCreated, err := s.EnqueueIdempotent(ctx, retry, now)
		if err != nil {
			t.Fatalf("EnqueueIdempotent() error = %v, want nil", err)
		}
		if wasCreated {
			t.Error("created = true, want false (reactivation reuses the existing Job)")
		}
		if got.ID != seeded.ID {
			t.Errorf("got.ID = %q, want the reactivated Job's original ID %q", got.ID, seeded.ID)
		}
		if got.State != queue.JobPending {
			t.Errorf("got.State = %v, want %v", got.State, queue.JobPending)
		}
		if got.Attempts != 0 {
			t.Errorf("got.Attempts = %d, want 0", got.Attempts)
		}
		if got.LastError != "" {
			t.Errorf("got.LastError = %q, want \"\"", got.LastError)
		}
		if !got.CreatedAt.Equal(created) {
			t.Errorf("got.CreatedAt = %v, want unchanged %v", got.CreatedAt, created)
		}
		if !got.UpdatedAt.Equal(now) {
			t.Errorf("got.UpdatedAt = %v, want %v", got.UpdatedAt, now)
		}

		stored, err := s.Get(ctx, seeded.ID)
		if err != nil {
			t.Fatalf("store.Get() error = %v, want nil", err)
		}
		if stored.State != queue.JobPending || stored.Attempts != 0 || stored.LastError != "" {
			t.Errorf("stored = %+v, want reactivated (State=pending, Attempts=0, LastError=\"\")", *stored)
		}

		jobs, err := s.List(ctx, queue.Filter{})
		if err != nil {
			t.Fatalf("List() error = %v, want nil", err)
		}
		if len(jobs) != 1 {
			t.Errorf("len(List()) = %d, want 1 (reactivation must not create a second Job)", len(jobs))
		}
	})
}

func TestStore_EnqueueIdempotent_MismatchedReceipt_ReturnsValidationErrorWithoutMutating(t *testing.T) {
	forEachIdempotentStore(t, func(t *testing.T, newStore func() queue.Store) {
		s := newStore()
		ctx := context.Background()
		now := time.Now()

		seeded := &queue.Job{
			ID:             "existing-job",
			PrinterName:    "front-desk",
			Receipt:        testReceipt("hello"),
			State:          queue.JobPending,
			IdempotencyKey: "key-1",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := s.Save(ctx, seeded); err != nil {
			t.Fatalf("Save() error = %v, want nil", err)
		}

		mismatched := &queue.Job{PrinterName: "front-desk", Receipt: testReceipt("goodbye"), IdempotencyKey: "key-1"}
		got, created, err := s.EnqueueIdempotent(ctx, mismatched, now)
		if !apperr.Is(err, apperr.KindValidation) {
			t.Fatalf("EnqueueIdempotent() error = %v, want apperr.KindValidation", err)
		}
		if got != nil {
			t.Errorf("EnqueueIdempotent() job = %+v, want nil on rejection", *got)
		}
		if created {
			t.Error("created = true, want false on rejection")
		}

		stored, err := s.Get(ctx, seeded.ID)
		if err != nil {
			t.Fatalf("store.Get() error = %v, want nil", err)
		}
		if stored.State != queue.JobPending {
			t.Errorf("stored.State = %v after a rejected reused key, want unchanged %v", stored.State, queue.JobPending)
		}

		jobs, err := s.List(ctx, queue.Filter{})
		if err != nil {
			t.Fatalf("List() error = %v, want nil", err)
		}
		if len(jobs) != 1 {
			t.Errorf("len(List()) = %d, want 1 (a rejected reused key must not create a Job)", len(jobs))
		}
	})
}

func TestStore_EnqueueIdempotent_MismatchedPrinterName_ReturnsValidationError(t *testing.T) {
	forEachIdempotentStore(t, func(t *testing.T, newStore func() queue.Store) {
		s := newStore()
		ctx := context.Background()
		now := time.Now()
		r := testReceipt("hello")

		seeded := &queue.Job{
			ID:             "existing-job",
			PrinterName:    "front-desk",
			Receipt:        r,
			State:          queue.JobPending,
			IdempotencyKey: "key-1",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := s.Save(ctx, seeded); err != nil {
			t.Fatalf("Save() error = %v, want nil", err)
		}

		mismatched := &queue.Job{PrinterName: "back-office", Receipt: r, IdempotencyKey: "key-1"}
		_, _, err := s.EnqueueIdempotent(ctx, mismatched, now)
		if !apperr.Is(err, apperr.KindValidation) {
			t.Fatalf("EnqueueIdempotent() error = %v, want apperr.KindValidation", err)
		}
	})
}

func TestStore_EnqueueIdempotent_ExpiredKey_TreatedAsNoMatch(t *testing.T) {
	forEachIdempotentStore(t, func(t *testing.T, newStore func() queue.Store) {
		s := newStore()
		ctx := context.Background()
		r := testReceipt("hello")
		expiredCreatedAt := time.Now().Add(-queue.IdempotencyKeyTTL - time.Minute)

		seeded := &queue.Job{
			ID:             "existing-job",
			PrinterName:    "front-desk",
			Receipt:        r,
			State:          queue.JobPending,
			IdempotencyKey: "key-1",
			CreatedAt:      expiredCreatedAt,
			UpdatedAt:      expiredCreatedAt,
		}
		if err := s.Save(ctx, seeded); err != nil {
			t.Fatalf("Save() error = %v, want nil", err)
		}

		now := time.Now()
		fresh := &queue.Job{PrinterName: "front-desk", Receipt: r, IdempotencyKey: "key-1"}
		got, created, err := s.EnqueueIdempotent(ctx, fresh, now)
		if err != nil {
			t.Fatalf("EnqueueIdempotent() error = %v, want nil", err)
		}
		if !created {
			t.Error("created = false, want true (an expired match must be treated as no match)")
		}
		if got.ID == seeded.ID {
			t.Errorf("got.ID == the expired Job's ID (%q), want a freshly generated ID", seeded.ID)
		}
		if got.IdempotencyKey != "key-1" {
			t.Errorf("got.IdempotencyKey = %q, want %q (the new Job keeps the key)", got.IdempotencyKey, "key-1")
		}

		jobs, err := s.List(ctx, queue.Filter{})
		if err != nil {
			t.Fatalf("List() error = %v, want nil", err)
		}
		if len(jobs) != 2 {
			t.Errorf("len(List()) = %d, want 2 (the stale expired Job plus the newly created one)", len(jobs))
		}
	})
}

// TestStore_EnqueueIdempotent_ConcurrentSameNewKey_CreatesExactlyOneJob
// fires many concurrent EnqueueIdempotent calls at the same brand-new key
// and asserts exactly one Job is ever created and every caller gets back
// the same Job ID — the atomicity guarantee
// docs/adr/0020-idempotent-print-requests.md requires, mirroring the same
// concurrent-claim race docs/adr/0016-queue-concurrency-per-printer-workers.md
// closes for NextPending-style claiming. Run with -race.
func TestStore_EnqueueIdempotent_ConcurrentSameNewKey_CreatesExactlyOneJob(t *testing.T) {
	forEachIdempotentStore(t, func(t *testing.T, newStore func() queue.Store) {
		s := newStore()
		ctx := context.Background()
		r := testReceipt("hello")
		const n = 50

		ids := make([]string, n)
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				j := &queue.Job{PrinterName: "front-desk", Receipt: r, IdempotencyKey: "race-key"}
				got, _, err := s.EnqueueIdempotent(ctx, j, time.Now())
				if err != nil {
					t.Errorf("EnqueueIdempotent() (goroutine %d) error = %v, want nil", i, err)
					return
				}
				ids[i] = got.ID
			}(i)
		}
		wg.Wait()

		want := ids[0]
		if want == "" {
			t.Fatal("ids[0] = \"\", want a generated ID")
		}
		for i, id := range ids {
			if id != want {
				t.Errorf("ids[%d] = %q, want every caller to observe the same ID %q", i, id, want)
			}
		}

		jobs, err := s.List(ctx, queue.Filter{})
		if err != nil {
			t.Fatalf("List() error = %v, want nil", err)
		}
		if len(jobs) != 1 {
			t.Errorf("len(List()) = %d, want exactly 1 Job created across %d concurrent callers racing the same new key", len(jobs), n)
		}
	})
}

// TestQueue_EnqueueIdempotent_ReturnsStoreDecidedID proves Queue.EnqueueIdempotent
// is a thin wrapper: it stamps the key onto j and returns whatever ID the
// Store decided is authoritative, including on a matching retry where
// that ID differs from any ID the caller might have pre-populated on j.
func TestQueue_EnqueueIdempotent_ReturnsStoreDecidedID(t *testing.T) {
	store := queue.NewMemoryStore()
	q := queue.New(store, &stubProcessor{})
	ctx := context.Background()
	r := testReceipt("hello")

	first, err := q.EnqueueIdempotent(ctx, &queue.Job{PrinterName: "front-desk", Receipt: r}, "key-1")
	if err != nil {
		t.Fatalf("EnqueueIdempotent() (first) error = %v, want nil", err)
	}
	if first == "" {
		t.Fatal("EnqueueIdempotent() (first) jobID = \"\", want a generated ID")
	}

	second, err := q.EnqueueIdempotent(ctx, &queue.Job{PrinterName: "front-desk", Receipt: r}, "key-1")
	if err != nil {
		t.Fatalf("EnqueueIdempotent() (retry) error = %v, want nil", err)
	}
	if second != first {
		t.Errorf("EnqueueIdempotent() (retry) jobID = %q, want the same ID as the first call, %q", second, first)
	}

	jobs, err := store.List(ctx, queue.Filter{})
	if err != nil {
		t.Fatalf("store.List() error = %v, want nil", err)
	}
	if len(jobs) != 1 {
		t.Errorf("len(store.List()) = %d, want 1", len(jobs))
	}
}

func TestQueue_EnqueueIdempotent_MismatchPropagatesValidationError(t *testing.T) {
	store := queue.NewMemoryStore()
	q := queue.New(store, &stubProcessor{})
	ctx := context.Background()

	if _, err := q.EnqueueIdempotent(ctx, &queue.Job{PrinterName: "front-desk", Receipt: testReceipt("hello")}, "key-1"); err != nil {
		t.Fatalf("EnqueueIdempotent() (first) error = %v, want nil", err)
	}

	_, err := q.EnqueueIdempotent(ctx, &queue.Job{PrinterName: "front-desk", Receipt: testReceipt("goodbye")}, "key-1")
	if !apperr.Is(err, apperr.KindValidation) {
		t.Fatalf("EnqueueIdempotent() (mismatched Receipt) error = %v, want apperr.KindValidation", err)
	}
}
