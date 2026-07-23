package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// shutdownDeadline bounds docs/adr/0018-graceful-shutdown.md's Phase 3:
// the whole drain (every in-flight HTTP request and every printer's
// worker, together, since they drain concurrently) must finish within
// this long, or the process exits anyway. Sized off the queue side: a
// typical Job's render+encode+Send finishes in low single digits of
// seconds on target hardware, and a Job caught mid-retry has at most one
// already-elapsed backoff wait left — 30s is a generous multiple of that,
// not a value borrowed from writeTimeout (which happens to match it by
// coincidence). A constant, not a config field: docs/ARCHITECTURE.md §7's
// schema is frozen; tune later from real hardware experience.
const shutdownDeadline = 30 * time.Second

// run starts d serving HTTP and the queue worker, then blocks until
// either the HTTP server stops on its own or a shutdown signal arrives,
// draining via shutdownOnSignal in the latter case. Returns the process
// exit code main should use. d.startWorker runs synchronously, before
// ListenAndServe's own goroutine starts, so d.workerCancel/d.workers are
// set before shutdown could ever read them from another goroutine.
func (d *daemon) run() int {
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	d.startWorker()

	serveErr := make(chan error, 1)
	go func() { serveErr <- d.srv.ListenAndServe() }()

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "receiptd: %v\n", err)
			return 1
		}
		return 0
	case <-sigCh:
		return d.shutdownOnSignal(sigCh)
	}
}

// shutdownOnSignal runs d.shutdown, bounded by d.deadline (or
// shutdownDeadline), and treats a second value on sigCh while draining
// like the deadline expiring — exit immediately (ADR-0018 Phase 3).
// Callable directly with a test-controlled channel, so the shutdown
// sequence is testable without a real OS signal. Returns 0 for a clean
// drain or 1 if forced; never calls os.Exit itself.
func (d *daemon) shutdownOnSignal(sigCh <-chan os.Signal) int {
	log.Println("receiptd: shutdown signal received, draining...")

	deadline := d.deadline
	if deadline <= 0 {
		deadline = shutdownDeadline
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- d.shutdown(ctx) }()

	select {
	case err := <-done:
		if err != nil {
			log.Println("receiptd: shutdown deadline exceeded, forcing exit")
			return 1
		}
		return 0
	case <-sigCh:
		cancel()
		log.Println("receiptd: repeat shutdown signal received, forcing exit")
		return 1
	}
}

// shutdown performs ADR-0018's Phase 1 (stop accepting: d.srv.Shutdown
// and cancel d.workerCancel, concurrently) and Phase 2 (wait for both to
// finish, bounded by ctx as a whole). It deliberately never touches the
// context of a Process call already in progress — an in-flight
// Printer.Send is left to finish naturally, this ADR's central safety
// argument. Returns nil on a clean drain, or ctx.Err() if the deadline
// (or an early cancel from shutdownOnSignal) wins first; never calls
// os.Exit itself.
func (d *daemon) shutdown(ctx context.Context) error {
	if d.workerCancel != nil {
		d.workerCancel()
	}

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = d.srv.Shutdown(ctx)
		}()
		go func() {
			defer wg.Done()
			d.workers.Wait()
		}()
		wg.Wait()
	}()

	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
