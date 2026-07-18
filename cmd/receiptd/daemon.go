package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/harveysandiego/receiptd/internal/api"
	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/auth"
	"github.com/harveysandiego/receiptd/internal/config"
	"github.com/harveysandiego/receiptd/internal/queue"
)

// pollInterval is how often the background queue worker checks for a
// pending Job when idle. The frozen config schema (docs/ARCHITECTURE.md
// §7) has no field for this yet; queue.ProcessNext already returns
// immediately once nothing is pending, so a short fixed interval is
// enough for Milestone 2's fake-printer worker.
const pollInterval = 1 * time.Second

// daemon is every component serve needs once wiring is complete: an HTTP
// server ready to bind cfg.Server.Address, and the Queue whose Jobs the
// background worker drains. Building one performs no network I/O and
// never blocks — only serve does.
type daemon struct {
	srv   *http.Server
	queue *queue.Queue
	// logSink is the open file backing Service.LogSink. It's kept here
	// (rather than only handed to app.Service) so a *daemon can eventually
	// close it on shutdown; nothing does so yet, since graceful shutdown
	// isn't part of this slice.
	logSink *os.File
}

// loadAndBuild loads Receiptd's configuration from configPath and wires it
// into a daemon via build.
func loadAndBuild(configPath string) (*daemon, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	return build(cfg)
}

// build wires together the existing components Milestone 2 needs: the
// configured queue Store, app.Service (with its LogSink pointed at the
// fake-printer log file), the versioned API routes, and — when
// cfg.Auth.Enabled — Bearer middleware in front of them. Every step
// delegates to an existing constructor; this introduces no new business
// logic of its own.
func build(cfg *config.Config) (*daemon, error) {
	store, err := buildStore(cfg.Queue)
	if err != nil {
		return nil, err
	}

	token, err := auth.ResolveToken(cfg.Auth)
	if err != nil {
		return nil, err
	}

	// Opened last, after every other fallible step: once this succeeds,
	// build no longer returns an error, so there's no path where the open
	// file handle would leak without a *daemon around to close it.
	logSink, err := os.OpenFile(printerLogPath(cfg.Queue), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, apperr.Wrap(apperr.KindPermanent, "main.build", err)
	}

	// Service and Queue are mutually referential — Service implements
	// queue.Processor structurally, and Queue needs that Processor at
	// construction — so Service is built first with its Queue field filled
	// in one line later, exactly as service.go's own doc comment expects
	// of a composition root.
	svc := &app.Service{LogSink: logSink}
	q := queue.New(store, svc)
	svc.Queue = q

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/print", api.NewPrintHandler(svc))
	mux.Handle("POST /api/v1/preview", api.NewPreviewHandler(svc))
	mux.Handle("GET /api/v1/jobs/{id}", api.NewJobStatusHandler(svc))

	var handler http.Handler = mux
	if cfg.Auth.Enabled {
		handler = auth.Bearer(token)(mux)
	}

	return &daemon{
		srv:     &http.Server{Addr: cfg.Server.Address, Handler: handler},
		queue:   q,
		logSink: logSink,
	}, nil
}

// buildStore constructs the queue.Store cfg.Store selects. Only "memory"
// and "bbolt" ever reach here: config.Validate rejects any other value
// before config.Load returns a *config.Config at all.
func buildStore(cfg config.QueueConfig) (queue.Store, error) {
	if cfg.Store == "memory" {
		return queue.NewMemoryStore(), nil
	}
	return queue.NewBoltStore(cfg.Path)
}

// printerLogPath is where Process's rendered output goes in Milestone 2,
// standing in for a real printer until Milestone 3 (docs/ARCHITECTURE.md
// §10). No dedicated config field exists for it yet, so it lives
// alongside the queue database — the one existing field that already
// names a directory for Receiptd's local state.
func printerLogPath(cfg config.QueueConfig) string {
	return filepath.Join(filepath.Dir(cfg.Path), "printer.log")
}

// serve starts the background queue worker and blocks serving HTTP on
// d.srv.Addr until the server stops, returning its error — always
// non-nil, per http.Server.ListenAndServe's contract. build opened
// d.logSink, so serve — the daemon's other lifecycle method — is
// responsible for closing it once ListenAndServe returns; a failure to
// close it is reported but doesn't override the ListenAndServe error,
// which is what actually determines main's exit status.
func (d *daemon) serve() error {
	go runWorker(context.Background(), d.queue)
	err := d.srv.ListenAndServe()
	if closeErr := d.logSink.Close(); closeErr != nil {
		fmt.Fprintf(os.Stderr, "receiptd: closing printer log file: %v\n", closeErr)
	}
	return err
}

// runWorker calls q.ProcessNext on a loop until ctx is cancelled, sleeping
// pollInterval between calls so an idle queue doesn't busy-loop. This is
// the "queue worker (background goroutine, independent of the HTTP
// request...)" docs/ARCHITECTURE.md §4 step 8 describes — ProcessNext
// already implements everything it does; runWorker only calls it
// repeatedly.
func runWorker(ctx context.Context, q *queue.Queue) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := q.ProcessNext(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "receiptd: queue.ProcessNext: %v\n", err)
		}
		time.Sleep(pollInterval)
	}
}
