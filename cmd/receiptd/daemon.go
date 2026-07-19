package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/harveysandiego/receiptd/internal/api"
	"github.com/harveysandiego/receiptd/internal/app"
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

// build wires together the existing components Receiptd needs: the
// configured queue Store, app.Service, the versioned API routes, and —
// when cfg.Auth.Enabled — Bearer middleware in front of them. Every step
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

	// Service and Queue are mutually referential — Service implements
	// queue.Processor structurally, and Queue needs that Processor at
	// construction — so Service is built first with its Queue field filled
	// in one line later, exactly as service.go's own doc comment expects
	// of a composition root.
	svc := &app.Service{}
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
		srv:   &http.Server{Addr: cfg.Server.Address, Handler: handler},
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

// serve starts the background queue worker and blocks serving HTTP on
// d.srv.Addr until the server stops, returning its error — always
// non-nil, per http.Server.ListenAndServe's contract.
func (d *daemon) serve() error {
	go runWorker(context.Background(), d.queue)
	return d.srv.ListenAndServe()
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
