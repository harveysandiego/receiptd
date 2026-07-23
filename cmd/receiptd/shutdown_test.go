package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/queue"
)

// slowProcessor is a queue.Processor test double that ignores ctx and
// sleeps for delay before returning result — modeling
// printer.networkPrinter.Send's real behavior (never checks ctx.Done()
// past dialing): an in-progress call can't be cut short, only waited out.
// started closes the moment Process begins, so a test can be sure the
// call is genuinely in flight before triggering shutdown.
type slowProcessor struct {
	delay   time.Duration
	result  error
	started chan struct{}
	once    sync.Once
}

func (p *slowProcessor) Process(_ context.Context, _ *queue.Job) error {
	p.once.Do(func() { close(p.started) })
	time.Sleep(p.delay)
	return p.result
}

// alwaysTransientProcessor is a queue.Processor test double that always
// fails with apperr.KindTransient, so ProcessNext keeps retrying (and
// backing off between attempts) until something interrupts it.
type alwaysTransientProcessor struct{}

func (alwaysTransientProcessor) Process(context.Context, *queue.Job) error {
	return apperr.Wrap(apperr.KindTransient, "test", errors.New("printer offline"))
}

// newTestJob enqueues and returns the ID of a minimal pending Job against
// q, for tests that only care about ProcessNext's state machine, not the
// actual receipt/printer content.
func newTestJob(t *testing.T, q *queue.Queue) string {
	t.Helper()
	j := &queue.Job{PrinterName: "front-desk"}
	if err := q.Enqueue(context.Background(), j); err != nil {
		t.Fatalf("q.Enqueue() error = %v, want nil", err)
	}
	return j.ID
}

// startSingleProcessNextWorker simulates exactly what daemon.startWorker
// wires up (d.workerCancel, d.workers, runWorker's ctx passed into
// ProcessNext) but for a single ProcessNext call instead of runWorker's
// loop, so a test can deterministically control when that one call
// begins without also standing up a real HTTP listener. Named distinctly
// from the (*daemon).startWorker method it stands in for, to keep the two
// from being confused with each other.
func startSingleProcessNextWorker(d *daemon, q *queue.Queue) <-chan error {
	ctx, cancel := context.WithCancel(context.Background())
	d.workerCancel = cancel
	d.workers.Add(1)
	procErr := make(chan error, 1)
	go func() {
		defer d.workers.Done()
		procErr <- q.ProcessNext(ctx)
	}()
	return procErr
}

// TestDaemon_Shutdown_DoesNotCancelInFlightProcess pins ADR-0018's central
// safety argument: a worker actively inside a Process call when shutdown
// begins is not cancelled — its ctx is left alone and it runs to
// completion — even though shutdown's own deadline is far shorter than
// that call takes and shutdown forcibly returns before it's done.
func TestDaemon_Shutdown_DoesNotCancelInFlightProcess(t *testing.T) {
	const processDelay = 150 * time.Millisecond
	store := queue.NewMemoryStore()
	proc := &slowProcessor{delay: processDelay, started: make(chan struct{})}
	q := queue.NewWithRetry(store, proc, 3, time.Second)
	jobID := newTestJob(t, q)

	d := &daemon{queue: q, srv: &http.Server{Addr: "127.0.0.1:0"}}
	procErr := startSingleProcessNextWorker(d, q)

	select {
	case <-proc.started:
	case <-time.After(time.Second):
		t.Fatal("proc.Process never started")
	}

	// A deadline much shorter than processDelay: shutdown must return
	// (forced) well before the in-flight Process call finishes on its
	// own, proving shutdown doesn't block synchronously waiting for it —
	// but, per the ADR, that call itself must still be left running, not
	// aborted.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := d.shutdown(shutdownCtx); err == nil {
		t.Fatal("shutdown() error = nil, want a deadline-exceeded error (the in-flight Process call was still running)")
	}

	// Now wait for the (never cancelled) Process call to actually finish
	// on its own and confirm it succeeded — if shutdown had cancelled its
	// ctx, a real Printer.Send would have no way to know to stop (it
	// doesn't check ctx.Done()), so this only proves shutdown never
	// interfered with the call itself, not that ctx was left uncancelled.
	select {
	case err := <-procErr:
		if err != nil {
			t.Fatalf("ProcessNext() error = %v, want nil (Process completed successfully once let run to completion)", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ProcessNext never returned after the in-flight Process call's own delay elapsed")
	}

	job, err := q.Get(context.Background(), jobID)
	if err != nil {
		t.Fatalf("q.Get() error = %v, want nil", err)
	}
	if job.State != queue.JobDone {
		t.Errorf("job.State = %v, want %v (an in-flight Process call must run to completion, not be cut short by shutdown)", job.State, queue.JobDone)
	}
}

// TestDaemon_Shutdown_InterruptsBackoffAndLeavesJobRunning covers ADR-0018
// Phase 2's other half at the daemon-integration level (queue's own
// retry_test.go covers it directly): a worker asleep in backoff is
// interrupted immediately by shutdown, and its Job is left JobRunning,
// not marked JobFailed.
func TestDaemon_Shutdown_InterruptsBackoffAndLeavesJobRunning(t *testing.T) {
	store := queue.NewMemoryStore()
	// A long base backoff: without interruption, ProcessNext would still
	// be asleep in its first backoff wait long after this test's own
	// generous shutdown deadline below.
	const baseBackoff = 2 * time.Second
	q := queue.NewWithRetry(store, alwaysTransientProcessor{}, 5, baseBackoff)
	jobID := newTestJob(t, q)

	d := &daemon{queue: q, srv: &http.Server{Addr: "127.0.0.1:0"}}
	procErr := startSingleProcessNextWorker(d, q)

	// Give ProcessNext a moment to claim the Job, fail its first attempt,
	// and enter its backoff sleep, before shutdown begins.
	time.Sleep(20 * time.Millisecond)

	start := time.Now()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown() error = %v, want nil (the backoff wait should be interrupted well within the deadline)", err)
	}
	if elapsed := time.Since(start); elapsed >= baseBackoff {
		t.Errorf("shutdown() took %v, want well under the %v backoff — cancellation should have cut the wait short", elapsed, baseBackoff)
	}

	select {
	case err := <-procErr:
		if err != nil {
			t.Fatalf("ProcessNext() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ProcessNext never returned after shutdown interrupted its backoff wait")
	}

	job, err := q.Get(context.Background(), jobID)
	if err != nil {
		t.Fatalf("q.Get() error = %v, want nil", err)
	}
	if job.State != queue.JobRunning {
		t.Errorf("job.State = %v, want %v (ADR-0018: interrupted mid-backoff must stay non-terminal, not Failed)", job.State, queue.JobRunning)
	}
}

// TestDaemon_ShutdownOnSignal_DeadlineExceeded_ForcesExit covers Phase 3:
// if the drain hasn't finished by d's deadline (a genuinely in-flight
// Process call taking longer than the deadline, here), shutdownOnSignal
// returns the forced-exit code rather than waiting any longer.
func TestDaemon_ShutdownOnSignal_DeadlineExceeded_ForcesExit(t *testing.T) {
	store := queue.NewMemoryStore()
	proc := &slowProcessor{delay: 500 * time.Millisecond, started: make(chan struct{})}
	q := queue.NewWithRetry(store, proc, 3, time.Second)
	newTestJob(t, q)

	d := &daemon{queue: q, srv: &http.Server{Addr: "127.0.0.1:0"}, deadline: 20 * time.Millisecond}
	startSingleProcessNextWorker(d, q)

	select {
	case <-proc.started:
	case <-time.After(time.Second):
		t.Fatal("proc.Process never started")
	}

	sigCh := make(chan os.Signal, 2)
	start := time.Now()
	code := d.shutdownOnSignal(sigCh)
	elapsed := time.Since(start)

	if code != 1 {
		t.Errorf("shutdownOnSignal() = %d, want 1 (deadline exceeded, forced exit)", code)
	}
	if elapsed >= proc.delay {
		t.Errorf("shutdownOnSignal() took %v, want well under the in-flight call's %v delay — the deadline should have forced an exit first", elapsed, proc.delay)
	}
}

// TestDaemon_ShutdownOnSignal_RepeatSignal_ExitsImmediately covers Phase
// 3's other forced-exit path: a second SIGTERM/SIGINT received while
// already draining is treated the same as the deadline expiring — logged
// and exited immediately, without waiting out the rest of a much longer
// deadline.
func TestDaemon_ShutdownOnSignal_RepeatSignal_ExitsImmediately(t *testing.T) {
	store := queue.NewMemoryStore()
	proc := &slowProcessor{delay: 5 * time.Second, started: make(chan struct{})}
	q := queue.NewWithRetry(store, proc, 3, time.Second)
	newTestJob(t, q)

	// A deadline far longer than this test should ever take: if the
	// repeat-signal path didn't work, shutdownOnSignal would block on
	// this instead and the test would time out.
	d := &daemon{queue: q, srv: &http.Server{Addr: "127.0.0.1:0"}, deadline: time.Minute}
	startSingleProcessNextWorker(d, q)

	select {
	case <-proc.started:
	case <-time.After(time.Second):
		t.Fatal("proc.Process never started")
	}

	sigCh := make(chan os.Signal, 2)
	go func() {
		time.Sleep(20 * time.Millisecond)
		sigCh <- syscall.SIGTERM
	}()

	start := time.Now()
	code := d.shutdownOnSignal(sigCh)
	elapsed := time.Since(start)

	if code != 1 {
		t.Errorf("shutdownOnSignal() = %d, want 1 (repeat signal, forced exit)", code)
	}
	if elapsed >= 500*time.Millisecond {
		t.Errorf("shutdownOnSignal() took %v, want well under the deadline — a repeat signal should exit immediately", elapsed)
	}
}

// TestDaemon_Run_ShutdownStopsServerAndWorker exercises startWorker/
// shutdown's actual wiring end to end: calling shutdown after the worker
// has started and ListenAndServe is running stops it with
// http.ErrServerClosed and lets the worker goroutine run to completion.
func TestDaemon_Run_ShutdownStopsServerAndWorker(t *testing.T) {
	d := buildDaemon(t, validConfig(t))
	d.startWorker()

	serveErr := make(chan error, 1)
	go func() { serveErr <- d.srv.ListenAndServe() }()

	// Give ListenAndServe a moment to actually start listening.
	time.Sleep(20 * time.Millisecond)

	if err := d.shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v, want nil (nothing in flight, should drain immediately)", err)
	}

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("serve() error = %v, want http.ErrServerClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serve() never returned after shutdown()")
	}
}
