package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/harveysandiego/receiptd/internal/api"
	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/assets"
	"github.com/harveysandiego/receiptd/internal/auth"
	"github.com/harveysandiego/receiptd/internal/config"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/queue"
)

// pollInterval is how often the idle background worker checks for a
// pending Job. Not yet configurable (docs/ARCHITECTURE.md §7); a short
// fixed interval suffices since ProcessNext returns immediately when
// nothing is pending.
const pollInterval = 1 * time.Second

// HTTP server timeouts, not yet configurable (docs/ARCHITECTURE.md §7).
// Fixed non-zero defaults rather than http.Server's zero value, which
// would let a stalled client hold a goroutine open forever:
// readHeaderTimeout mitigates a Slowloris-style attack; readTimeout
// bounds the whole request and must exceed it while allowing the api
// package's 10 MiB body over a slow link; writeTimeout covers rendering
// a preview PNG; idleTimeout bounds an idle keep-alive connection.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
)

// daemon is everything run needs once wiring is complete: an HTTP server
// ready to bind cfg.Server.Address, the Queue the per-printer workers
// drain, and the printer names those workers are started for. Building
// one performs no network I/O and never blocks — only run does.
type daemon struct {
	srv   *http.Server
	queue *queue.Queue
	// printerNames is the set startWorker starts one goroutine per entry
	// of — one worker per configured printer, never more than one per
	// name (docs/adr/0016-queue-concurrency-per-printer-workers.md).
	printerNames []string

	// workerCancel cancels the one context shared by every worker
	// startWorker started, so all of them stop claiming new Jobs and wake
	// from an idle/backoff sleep at the same instant on shutdown
	// (docs/adr/0018-graceful-shutdown.md). nil until startWorker has run.
	workerCancel context.CancelFunc
	// workers reaches zero once every worker goroutine has returned.
	workers sync.WaitGroup
	// deadline overrides shutdownDeadline (shutdown.go) for this daemon;
	// zero means "use the constant". Exists so tests can drive Phase 3's
	// deadline-exceeded path in milliseconds instead of the real 30s.
	deadline time.Duration
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

// build wires the components a daemon needs: the configured queue Store,
// app.Service, the versioned API routes, and — when cfg.Auth.Enabled —
// Bearer middleware in front of them.
func build(cfg *config.Config) (*daemon, error) {
	store, err := buildStore(cfg.Queue)
	if err != nil {
		return nil, err
	}

	token, err := auth.ResolveToken(cfg.Auth)
	if err != nil {
		return nil, err
	}

	// Service and Queue are mutually referential — Service implements
	// queue.Processor structurally and Queue needs that Processor at
	// construction — so Service is built first and its Queue field filled
	// in one line later.
	svc := &app.Service{}
	q := queue.NewWithRetry(store, svc, cfg.Queue.MaxAttempts, cfg.Queue.RetryBackoff)
	svc.Queue = q
	var printerNames []string
	svc.Printers, svc.Profiles, printerNames = buildPrinters(cfg.Printers)
	svc.Assets = assets.NewFilesystemStore(cfg.Assets.Path)

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/print", api.NewPrintHandler(svc))
	mux.Handle("POST /api/v1/preview", api.NewPreviewHandler(svc))
	mux.Handle("GET /api/v1/jobs/{id}", api.NewJobStatusHandler(svc))

	var handler http.Handler = mux
	if cfg.Auth.Enabled {
		handler = auth.Bearer(token)(mux)
	}

	return &daemon{
		srv: &http.Server{
			Addr:              cfg.Server.Address,
			Handler:           handler,
			ReadTimeout:       readTimeout,
			ReadHeaderTimeout: readHeaderTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		},
		queue:        q,
		printerNames: printerNames,
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

// buildPrinters constructs a printer.Printer and resolves the Profile
// for every entry in cfgs, keyed by PrinterConfig.Name — the key
// app.Service.Process looks a Job's PrinterName up under
// (docs/ARCHITECTURE.md §4 step 8a, 8f) — plus that same name set as a
// slice, so it and the maps can never disagree about which printers are
// configured. Only "network" transport exists (config.Validate rejects
// others), so a second case isn't needed yet (docs/ARCHITECTURE.md §11).
func buildPrinters(cfgs []config.PrinterConfig) (printers map[string]printer.Printer, profiles map[string]printer.Profile, names []string) {
	printers = make(map[string]printer.Printer, len(cfgs))
	profiles = make(map[string]printer.Profile, len(cfgs))
	names = make([]string, 0, len(cfgs))
	for _, c := range cfgs {
		printers[c.Name] = printer.NewNetworkPrinter(c.Connection)
		profiles[c.Name] = c.Profile
		names = append(names, c.Name)
	}
	return printers, profiles, names
}

// startWorker starts one runWorker goroutine per entry of d.printerNames,
// all sharing one cancellable context (d.workerCancel, d.workers) so
// every printer's worker stops at the same instant on shutdown
// (docs/adr/0018-graceful-shutdown.md), then returns without blocking. It
// runs synchronously in run, before the goroutine that calls
// ListenAndServe, so d.workerCancel/d.workers are already set before
// shutdown could ever read them from another goroutine.
func (d *daemon) startWorker() {
	ctx, cancel := context.WithCancel(context.Background())
	d.workerCancel = cancel
	d.workers.Add(len(d.printerNames))
	for _, name := range d.printerNames {
		go func(printerName string) {
			defer d.workers.Done()
			runWorker(ctx, d.queue, printerName)
		}(name)
	}
}

// runWorker calls q.ProcessNextForPrinter(ctx, printerName) on a loop
// until ctx is cancelled, sleeping pollInterval between calls — one
// printer's worker of docs/adr/0016-queue-concurrency-per-printer-workers.md.
// Cancellation stops it claiming a new Job and wakes an idle poll sleep
// (sleepUnlessDone) immediately, but doesn't affect a
// ProcessNextForPrinter call already in progress
// (docs/adr/0018-graceful-shutdown.md).
func runWorker(ctx context.Context, q *queue.Queue, printerName string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := q.ProcessNextForPrinter(ctx, printerName); err != nil {
			fmt.Fprintf(os.Stderr, "receiptd: queue.ProcessNextForPrinter(%q): %v\n", printerName, err)
		}
		sleepUnlessDone(ctx, pollInterval)
	}
}

// sleepUnlessDone blocks for d, or until ctx is cancelled, whichever
// comes first — unlike time.Sleep, so an idle poll wait doesn't hold up
// shutdown (docs/adr/0018-graceful-shutdown.md).
func sleepUnlessDone(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}
