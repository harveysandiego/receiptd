package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
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

// daemon is everything serve needs once wiring is complete: an HTTP
// server ready to bind cfg.Server.Address, and the Queue the background
// worker drains. Building one performs no network I/O and never blocks —
// only serve does.
type daemon struct {
	srv   *http.Server
	queue *queue.Queue
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
	svc.Printers, svc.Profiles = buildPrinters(cfg.Printers)
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
		queue: q,
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
// (docs/ARCHITECTURE.md §4 step 8a, 8f). Only "network" transport exists
// (config.Validate rejects others), so a second case isn't needed yet
// (docs/ARCHITECTURE.md §11).
func buildPrinters(cfgs []config.PrinterConfig) (map[string]printer.Printer, map[string]printer.Profile) {
	printers := make(map[string]printer.Printer, len(cfgs))
	profiles := make(map[string]printer.Profile, len(cfgs))
	for _, c := range cfgs {
		printers[c.Name] = printer.NewNetworkPrinter(c.Connection)
		profiles[c.Name] = c.Profile
	}
	return printers, profiles
}

// serve starts the background queue worker and blocks serving HTTP on
// d.srv.Addr until the server stops, returning its error — always
// non-nil, per http.Server.ListenAndServe's contract.
func (d *daemon) serve() error {
	go runWorker(context.Background(), d.queue)
	return d.srv.ListenAndServe()
}

// runWorker calls q.ProcessNext on a loop until ctx is cancelled, sleeping
// pollInterval between calls so an idle queue doesn't busy-loop — the
// background queue worker of docs/ARCHITECTURE.md §4 step 8.
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
