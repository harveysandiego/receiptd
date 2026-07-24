package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/queue"
)

// neverCalledProcessor is a queue.Processor spy that fails the test if ever
// asked to Process forbidID — used to prove a Job that reconciliation must
// resolve to Failed on its own is never handed to a worker.
type neverCalledProcessor struct {
	t        *testing.T
	forbidID string
}

func (p neverCalledProcessor) Process(_ context.Context, j *queue.Job) error {
	if j.ID == p.forbidID {
		p.t.Errorf("Process called for job %q, which reconciliation should have already resolved to Failed before any worker could claim it", p.forbidID)
	}
	return errors.New("neverCalledProcessor: unexpected call")
}

// TestDaemon_Run_ReconcilesOrphanedRunningJobBeforeAnyWorkerStarts pins
// docs/adr/0017-queue-lifecycle-crash-recovery.md's ordering requirement:
// run() must reconcile to completion before startWorker claims anything.
// Seeding a Job Running with Attempts one short of maxAttempts makes the
// outcome unambiguous: reconciliation fails it outright (Attempts reaches
// maxAttempts), an outcome no worker could ever produce from Running
// (ClaimNextPending only ever claims Pending) — so a regression that skips
// or reorders reconciliation would instead leave this Job stuck in
// Running, not Failed.
func TestDaemon_Run_ReconcilesOrphanedRunningJobBeforeAnyWorkerStarts(t *testing.T) {
	const maxAttempts = 3
	store := queue.NewMemoryStore()
	orphan := &queue.Job{ID: "orphan-job", PrinterName: "front-desk", State: queue.JobRunning, Attempts: maxAttempts - 1}
	if err := store.Save(context.Background(), orphan); err != nil {
		t.Fatalf("store.Save() error = %v", err)
	}

	proc := neverCalledProcessor{t: t, forbidID: orphan.ID}
	q := queue.NewWithRetry(store, proc, maxAttempts, time.Millisecond)

	d := &daemon{
		queue:        q,
		srv:          &http.Server{Addr: "127.0.0.1:0"},
		printerNames: []string{"front-desk"},
	}

	runDone := make(chan int, 1)
	go func() { runDone <- d.run() }()

	// run() reconciles synchronously, before it even opens a listener, so
	// the Job is already resolved the instant run() begins — this deadline
	// is generous scheduling headroom, not something correctness depends on.
	deadline := time.Now().Add(time.Second)
	var got *queue.Job
	for {
		var err error
		got, err = q.Get(context.Background(), orphan.ID)
		if err != nil {
			t.Fatalf("q.Get() error = %v", err)
		}
		if got.State != queue.JobRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job stayed JobRunning for %v — reconciliation did not run", time.Second)
		}
		time.Sleep(2 * time.Millisecond)
	}

	if got.State != queue.JobFailed {
		t.Errorf("State = %v, want %v", got.State, queue.JobFailed)
	}
	if got.Attempts != maxAttempts {
		t.Errorf("Attempts = %d, want %d", got.Attempts, maxAttempts)
	}
	if !strings.Contains(got.LastError, "interrupted") {
		t.Errorf("LastError = %q, want it to mention the interruption", got.LastError)
	}

	// Force run()'s ListenAndServe to return so it exits cleanly instead of
	// leaking past this test — Close (unlike Shutdown) is safe to call
	// regardless of whether Serve has started listening yet. run()'s
	// "server stopped on its own" path returns without waiting on
	// d.workers, so runDone fires as soon as ListenAndServe does — this
	// receive is what the test needs anyway, and, as a channel receive, it
	// also establishes a happens-before edge back to everything run() did
	// in its own goroutine (including startWorker's write to
	// d.workerCancel). Reading d.workerCancel beforehand, from this
	// goroutine while run()'s goroutine could still be inside startWorker,
	// would be a genuine data race — the polling loop above only
	// synchronizes on the Job's state (set during Reconcile, before
	// startWorker runs), not on startWorker having completed.
	_ = d.srv.Close()
	var runCode int
	select {
	case runCode = <-runDone:
	case <-time.After(time.Second):
		t.Fatal("run() never returned after srv.Close()")
	}
	if runCode != 0 {
		t.Errorf("run() = %d, want 0", runCode)
	}
	if d.workerCancel != nil {
		d.workerCancel()
	}
}
