package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/config"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// validConfig returns a minimal, valid *config.Config for build/loadAndBuild
// tests: an in-memory queue store and auth disabled. Every path is confined
// to t.TempDir() so tests never write into the repo's working directory.
//
// It configures one printer, "preview-printer", solely so previewRequest()
// has a real Profile to resolve — Preview only ever reads Service.Profiles,
// never Service.Printers, so this never dials anything (docs/adr/0006).
// Deliberately not named "front-desk": several /print tests below rely on
// that specific name staying unconfigured here.
func validConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Server: config.ServerConfig{Address: "127.0.0.1:0"},
		Auth:   config.AuthConfig{Enabled: false},
		Queue: config.QueueConfig{
			Store:        "memory",
			Path:         filepath.Join(t.TempDir(), "queue.db"),
			MaxAttempts:  3,
			RetryBackoff: 5 * time.Second,
		},
		Printers: []config.PrinterConfig{
			{
				Name:       "preview-printer",
				Connection: printer.Connection{Transport: "network", Address: "127.0.0.1:0"},
				Profile:    printer.Profile{WidthDots: 384, DPI: 203},
			},
		},
	}
}

func previewRequest() *http.Request {
	body := `{"printer":"preview-printer","receipt":{"elements":[{"type":"text","content":"hi"}]}}`
	return httptest.NewRequest(http.MethodPost, "/api/v1/preview", bytes.NewBufferString(body))
}

func printRequest() *http.Request {
	body := `{"printer":"front-desk","receipt":{"elements":[{"type":"text","content":"hi"}]}}`
	return httptest.NewRequest(http.MethodPost, "/api/v1/print", bytes.NewBufferString(body))
}

// webuiPreviewFormRequest builds a form-encoded POST /preview request, the
// Web UI's own request shape (distinct from previewRequest's JSON body
// against /api/v1/preview) — see internal/webui/preview_test.go for the
// same helper's use in non-daemon tests.
func webuiPreviewFormRequest() *http.Request {
	form := url.Values{
		"printer": {"preview-printer"},
		"receipt": {`{"version":1,"elements":[{"type":"text","content":"hi"}]}`},
	}
	req := httptest.NewRequest(http.MethodPost, "/preview", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func writeTokenFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("secret-token"), 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	return path
}

// buildDaemon calls build and fails the test if it errors.
func buildDaemon(t *testing.T, cfg *config.Config) *daemon {
	t.Helper()
	d, err := build(cfg)
	if err != nil {
		t.Fatalf("build() error = %v, want nil", err)
	}
	return d
}

func TestBuildPrinters_ConstructsPrinterAndProfileForEachConfiguredEntry(t *testing.T) {
	cfgs := []config.PrinterConfig{
		{
			Name:       "front-desk",
			Connection: printer.Connection{Transport: "network", Address: "127.0.0.1:9100"},
			Profile:    printer.Profile{WidthDots: 576, DPI: 203, SupportsCut: true, DefaultCut: "full"},
		},
		{
			Name:       "kitchen",
			Connection: printer.Connection{Transport: "network", Address: "127.0.0.1:9101"},
			Profile:    printer.Profile{WidthDots: 384, DPI: 203, DefaultCut: "partial"},
		},
	}

	printers, profiles, connections, names := buildPrinters(cfgs)

	if len(printers) != len(cfgs) {
		t.Errorf("len(printers) = %d, want %d", len(printers), len(cfgs))
	}
	if len(profiles) != len(cfgs) {
		t.Errorf("len(profiles) = %d, want %d", len(profiles), len(cfgs))
	}
	if len(connections) != len(cfgs) {
		t.Errorf("len(connections) = %d, want %d", len(connections), len(cfgs))
	}
	if len(names) != len(cfgs) {
		t.Errorf("len(names) = %d, want %d", len(names), len(cfgs))
	}
	for _, c := range cfgs {
		if _, ok := printers[c.Name]; !ok {
			t.Errorf("printers[%q] missing, want a constructed printer.Printer", c.Name)
		}
		if got := profiles[c.Name]; got != c.Profile {
			t.Errorf("profiles[%q] = %+v, want %+v", c.Name, got, c.Profile)
		}
		want := app.ConnectionSummary{Transport: c.Connection.Transport, Address: c.Connection.Address}
		if got := connections[c.Name]; got != want {
			t.Errorf("connections[%q] = %+v, want %+v", c.Name, got, want)
		}
	}
	// names must be exactly the same set of printer names as printers/
	// profiles/connections are keyed by — all four come from the same
	// loop over cfgs, so they can never disagree; this pins that they
	// don't.
	for _, name := range names {
		if _, ok := printers[name]; !ok {
			t.Errorf("names contains %q, which is not a key of printers", name)
		}
	}
}

func TestBuildPrinters_NoConfiguredPrinters_ReturnsEmptyMaps(t *testing.T) {
	printers, profiles, connections, names := buildPrinters(nil)
	if len(printers) != 0 {
		t.Errorf("len(printers) = %d, want 0", len(printers))
	}
	if len(profiles) != 0 {
		t.Errorf("len(profiles) = %d, want 0", len(profiles))
	}
	if len(connections) != 0 {
		t.Errorf("len(connections) = %d, want 0", len(connections))
	}
	if len(names) != 0 {
		t.Errorf("len(names) = %d, want 0", len(names))
	}
}

func TestBuild_ValidConfig_Succeeds(t *testing.T) {
	d := buildDaemon(t, validConfig(t))
	if d.srv.Addr != "127.0.0.1:0" {
		t.Errorf("srv.Addr = %q, want %q", d.srv.Addr, "127.0.0.1:0")
	}
	if d.srv.Handler == nil {
		t.Error("srv.Handler is nil")
	}
	if d.queue == nil {
		t.Error("queue is nil")
	}
}

// TestBuild_PrinterNamesMatchConfiguredPrinters pins build()'s wiring of
// daemon.printerNames from cfg.Printers: serve starts one worker per entry
// of this slice (docs/adr/0016-queue-concurrency-per-printer-workers.md),
// so it must contain exactly the configured printer names, no more and no
// fewer — an empty or wrong set here would silently mean some printer's
// Jobs are never claimed by any worker.
func TestBuild_PrinterNamesMatchConfiguredPrinters(t *testing.T) {
	cfg := validConfig(t)
	cfg.Printers = []config.PrinterConfig{
		{
			Name:       "front-desk",
			Connection: printer.Connection{Transport: "network", Address: "127.0.0.1:9100"},
			Profile:    printer.Profile{WidthDots: 576, DPI: 203},
		},
		{
			Name:       "kitchen",
			Connection: printer.Connection{Transport: "network", Address: "127.0.0.1:9101"},
			Profile:    printer.Profile{WidthDots: 384, DPI: 203},
		},
	}
	d := buildDaemon(t, cfg)

	want := map[string]bool{"front-desk": true, "kitchen": true}
	if len(d.printerNames) != len(want) {
		t.Fatalf("len(printerNames) = %d, want %d (printerNames = %v)", len(d.printerNames), len(want), d.printerNames)
	}
	for _, name := range d.printerNames {
		if !want[name] {
			t.Errorf("printerNames contains unexpected name %q", name)
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Errorf("printerNames is missing %v", want)
	}
}

// TestBuild_SinglePrinterConfig_HasExactlyOnePrinterName pins the common
// homelab deployment shape this ADR promises stays behaviorally unchanged:
// one configured printer means serve starts exactly one worker, same as
// before this ADR (docs/adr/0016-queue-concurrency-per-printer-workers.md).
func TestBuild_SinglePrinterConfig_HasExactlyOnePrinterName(t *testing.T) {
	d := buildDaemon(t, validConfig(t))
	if len(d.printerNames) != 1 {
		t.Fatalf("len(printerNames) = %d, want 1 (validConfig configures exactly \"preview-printer\")", len(d.printerNames))
	}
	if d.printerNames[0] != "preview-printer" {
		t.Errorf("printerNames = %v, want [\"preview-printer\"]", d.printerNames)
	}
}

// TestBuild_NoConfiguredPrinters_HasNoPrinterNames proves the edge of this
// ADR's "one worker per configured printer" rule: zero configured printers
// means zero workers started, not a single global fallback worker.
func TestBuild_NoConfiguredPrinters_HasNoPrinterNames(t *testing.T) {
	cfg := validConfig(t)
	cfg.Printers = nil
	d := buildDaemon(t, cfg)
	if len(d.printerNames) != 0 {
		t.Errorf("len(printerNames) = %d, want 0", len(d.printerNames))
	}
}

// TestDaemon_Workers_OnePrinterOffline_DoesNotBlockAnotherPrinterWorker
// exercises this ADR's central guarantee through real production wiring —
// build()'s printerNames and runWorker, not a fake Store/Processor
// (docs/adr/0016-queue-concurrency-per-printer-workers.md). "kitchen" is
// an offline printer whose worker retries with backoff for a few hundred
// ms; "front-desk"'s Job must still reach JobDone well within that
// window, proving one printer's stuck retries never delay another's.
func TestDaemon_Workers_OnePrinterOffline_DoesNotBlockAnotherPrinterWorker(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = io.Copy(io.Discard, conn)
			_ = conn.Close()
		}
	}()

	// A listener opened and immediately closed refuses new connections
	// instantly (no accept queue) — see TestBuild_HonoursConfiguredQueue
	// RetrySettings's own comment for why this is a fast, reliable way to
	// simulate an offline printer without depending on an unbound port.
	offlineLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	offlineAddr := offlineLn.Addr().String()
	if err := offlineLn.Close(); err != nil {
		t.Fatalf("offlineLn.Close() error = %v", err)
	}

	cfg := validConfig(t)
	cfg.Queue.MaxAttempts = 3
	cfg.Queue.RetryBackoff = 100 * time.Millisecond
	cfg.Printers = []config.PrinterConfig{
		{
			Name:       "front-desk",
			Connection: printer.Connection{Transport: "network", Address: ln.Addr().String()},
			Profile:    printer.Profile{WidthDots: 576, DPI: 203, DefaultCut: "full"},
		},
		{
			Name:       "kitchen",
			Connection: printer.Connection{Transport: "network", Address: offlineAddr},
			Profile:    printer.Profile{WidthDots: 384, DPI: 203, DefaultCut: "full"},
		},
	}
	d := buildDaemon(t, cfg)

	r := receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: "hello"}}}

	// A backlog of kitchen Jobs, not just one: with only one, a buggy
	// ClaimNextPending that ignored printerName could still pass by
	// coincidence. A backlog makes that coincidence vanishingly unlikely.
	const kitchenBacklog = 8
	for i := 0; i < kitchenBacklog; i++ {
		if err := d.queue.Enqueue(context.Background(), &queue.Job{PrinterName: "kitchen", Receipt: r}); err != nil {
			t.Fatalf("Enqueue(kitchen #%d) error = %v, want nil", i, err)
		}
	}
	frontDeskJob := &queue.Job{PrinterName: "front-desk", Receipt: r}
	if err := d.queue.Enqueue(context.Background(), frontDeskJob); err != nil {
		t.Fatalf("Enqueue(front-desk) error = %v, want nil", err)
	}

	workerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, name := range d.printerNames {
		go runWorker(workerCtx, d.queue, name)
	}

	// front-desk succeeds on its first attempt, with nothing to wait on;
	// kitchen needs at least ~300ms of backoff before it can fail. A 150ms
	// deadline for front-desk to finish is generous for the former and
	// impossible to hit if front-desk's worker were somehow stuck behind
	// kitchen's.
	deadline := time.Now().Add(150 * time.Millisecond)
	for {
		got, err := d.queue.Get(context.Background(), frontDeskJob.ID)
		if err != nil {
			t.Fatalf("queue.Get(front-desk) error = %v, want nil", err)
		}
		if got.State == queue.JobDone {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("front-desk Job did not reach JobDone within %v (state = %v) — kitchen's offline retries may have blocked front-desk's worker", 150*time.Millisecond, got.State)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestBuild_ServerHasTimeouts guards against http.Server's zero-value
// defaults (no timeout at all) reaching production: a client that opens a
// connection and never finishes sending a request, or never reads the
// response, would otherwise tie up a server goroutine indefinitely.
func TestBuild_ServerHasTimeouts(t *testing.T) {
	d := buildDaemon(t, validConfig(t))

	if d.srv.ReadTimeout <= 0 {
		t.Errorf("srv.ReadTimeout = %v, want a positive timeout", d.srv.ReadTimeout)
	}
	if d.srv.ReadHeaderTimeout <= 0 {
		t.Errorf("srv.ReadHeaderTimeout = %v, want a positive timeout", d.srv.ReadHeaderTimeout)
	}
	if d.srv.WriteTimeout <= 0 {
		t.Errorf("srv.WriteTimeout = %v, want a positive timeout", d.srv.WriteTimeout)
	}
	if d.srv.IdleTimeout <= 0 {
		t.Errorf("srv.IdleTimeout = %v, want a positive timeout", d.srv.IdleTimeout)
	}
}

func TestBuild_PreviewRouteWired(t *testing.T) {
	d := buildDaemon(t, validConfig(t))

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, previewRequest())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
	pngSignature := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if !bytes.HasPrefix(rec.Body.Bytes(), pngSignature) {
		t.Errorf("response did not start with the PNG signature: %x", rec.Body.Bytes())
	}
}

func TestBuild_PrintAndJobStatusRoutesWired(t *testing.T) {
	d := buildDaemon(t, validConfig(t))

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, printRequest())
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/v1/print status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body)
	}

	var resp struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode /print response: %v", err)
	}
	if resp.JobID == "" {
		t.Fatal("job_id is empty")
	}

	rec = httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+resp.JobID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/jobs/%s status = %d, want %d, body = %s", resp.JobID, rec.Code, http.StatusOK, rec.Body)
	}
}

func TestBuild_PrintWithConfiguredPrinter_JobSucceeds(t *testing.T) {
	// This is the wiring TestBuild_PrintWithoutConfiguredPrinter_JobFails
	// pins the absence of: build() populates Service.Printers/Profiles from
	// cfg.Printers, so a Job targeting a configured PrinterName reaches a
	// real network printer.Printer end to end. Per docs/ARCHITECTURE.md §9
	// ("printer (network): Local net.Listen fake server — no hardware in
	// CI"), the "printer" here is a local TCP listener, not real hardware.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = io.Copy(io.Discard, conn)
			_ = conn.Close()
		}
	}()

	cfg := validConfig(t)
	cfg.Printers = []config.PrinterConfig{
		{
			Name:       "front-desk",
			Connection: printer.Connection{Transport: "network", Address: ln.Addr().String()},
			Profile:    printer.Profile{WidthDots: 576, DPI: 203, DefaultCut: "full"},
		},
	}
	d := buildDaemon(t, cfg)

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, printRequest())
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/v1/print status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	var printResp struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &printResp); err != nil {
		t.Fatalf("decode /print response: %v", err)
	}

	if err := d.queue.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}

	rec = httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+printResp.JobID, nil))
	var statusResp struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("decode job status response: %v", err)
	}
	if statusResp.State != "done" {
		t.Errorf("job state = %q, want %q (front-desk is configured and its fake server accepts the connection)", statusResp.State, "done")
	}
}

func TestBuild_PrintWithoutConfiguredPrinter_JobFails(t *testing.T) {
	// build() wires Service.Printers/Profiles from cfg.Printers, but
	// validConfig(t) never configures a printer named "front-desk" (only
	// "preview-printer", for previewRequest()) — so a Job targeting
	// "front-desk" still has neither a Printer nor a Profile to resolve,
	// and is expected to fail. This pins the (still honest) failure
	// behavior for a printer name that simply isn't in cfg.Printers.
	d := buildDaemon(t, validConfig(t))

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, printRequest())
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/v1/print status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	var printResp struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &printResp); err != nil {
		t.Fatalf("decode /print response: %v", err)
	}

	if err := d.queue.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}

	rec = httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+printResp.JobID, nil))
	var statusResp struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("decode job status response: %v", err)
	}
	if statusResp.State != "failed" {
		t.Errorf("job state = %q, want %q (%q is not in cfg.Printers)", statusResp.State, "failed", "front-desk")
	}
}

// TestBuild_HonoursConfiguredQueueRetrySettings proves build() wires
// cfg.Queue.MaxAttempts/RetryBackoff into the Queue it constructs, instead
// of queue's own package-default retry settings: a printer.Connection
// nobody is listening on fails every Send with apperr.KindTransient (see
// printer.networkPrinter.Send), so the number of Process calls one
// ProcessNext performs is a direct, observable proxy for which
// max_attempts value is actually in effect.
func TestBuild_HonoursConfiguredQueueRetrySettings(t *testing.T) {
	// A listener opened and immediately closed frees its port back to the
	// OS but almost certainly still refuses new connections instantly
	// (no accept queue), giving a fast, reliable dial failure without
	// depending on a specific unused port being unbound.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("ln.Close() error = %v", err)
	}

	const configuredMaxAttempts = 2
	cfg := validConfig(t)
	cfg.Queue.MaxAttempts = configuredMaxAttempts
	cfg.Queue.RetryBackoff = time.Millisecond
	cfg.Printers = []config.PrinterConfig{
		{
			Name:       "front-desk",
			Connection: printer.Connection{Transport: "network", Address: addr},
			Profile:    printer.Profile{WidthDots: 576, DPI: 203, DefaultCut: "full"},
		},
	}
	d := buildDaemon(t, cfg)

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, printRequest())
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/v1/print status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	var printResp struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &printResp); err != nil {
		t.Fatalf("decode /print response: %v", err)
	}

	if err := d.queue.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}

	rec = httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+printResp.JobID, nil))
	var statusResp struct {
		State    string `json:"state"`
		Attempts int    `json:"attempts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("decode job status response: %v", err)
	}
	if statusResp.State != "failed" {
		t.Fatalf("job state = %q, want %q", statusResp.State, "failed")
	}
	if statusResp.Attempts != configuredMaxAttempts {
		t.Errorf("job attempts = %d, want %d (cfg.Queue.MaxAttempts), not queue's own package default", statusResp.Attempts, configuredMaxAttempts)
	}
}

func TestBuild_AuthEnabled_RequiresBearerToken(t *testing.T) {
	cfg := validConfig(t)
	cfg.Auth = config.AuthConfig{Enabled: true, TokenFile: writeTokenFile(t)}

	d := buildDaemon(t, cfg)

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, previewRequest())
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("missing credential: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	req := previewRequest()
	req.Header.Set("Authorization", "Bearer secret-token")
	rec = httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("valid credential: status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
}

func TestBuild_AuthDisabled_NoCredentialRequired(t *testing.T) {
	d := buildDaemon(t, validConfig(t))

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, previewRequest())
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
}

func TestBuild_WebDisabled_RootPathNotMounted(t *testing.T) {
	cfg := validConfig(t)
	cfg.Web = config.WebConfig{Enabled: false}
	d := buildDaemon(t, cfg)

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (web.enabled is false, so no Web UI route should be mounted)", rec.Code, http.StatusNotFound)
	}
}

// webUIStubRoutes is every route in the Milestone 4 routing contract
// (docs/ARCHITECTURE.md §10) still awaiting its own slice, shared by the
// auth-disabled and auth-enabled coverage tests below so both exercise
// exactly the same route list. GET / (Dashboard), GET /printers
// (Printers), the Assets routes, and the Preview routes are covered
// separately by the tests below, since those slices have replaced their
// stubs and no longer answer 501.
func webUIStubRoutes() []struct{ method, path string } {
	return []struct{ method, path string }{
		{http.MethodGet, "/status"},
		{http.MethodGet, "/print"},
		{http.MethodPost, "/print"},
	}
}

func TestBuild_WebEnabled_AuthDisabled_EveryRoute_ReachesStub(t *testing.T) {
	cfg := validConfig(t)
	cfg.Web = config.WebConfig{Enabled: true}
	d := buildDaemon(t, cfg)

	for _, rt := range webUIStubRoutes() {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			d.srv.Handler.ServeHTTP(rec, httptest.NewRequest(rt.method, rt.path, nil))
			if rec.Code != http.StatusNotImplemented {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body)
			}
		})
	}
}

// TestBuild_WebEnabled_AuthEnabled_EveryRoute_RequiresBasicThenReachesStub
// proves the routing contract's full coverage: every documented route is
// both mounted and protected — a missing credential is 401 and never
// reaches a handler, a valid one reaches the (still-stub) handler and
// gets 501, for every route, not just one representative path.
func TestBuild_WebEnabled_AuthEnabled_EveryRoute_RequiresBasicThenReachesStub(t *testing.T) {
	cfg := validConfig(t)
	cfg.Web = config.WebConfig{Enabled: true}
	cfg.Auth = config.AuthConfig{Enabled: true, TokenFile: writeTokenFile(t)}
	d := buildDaemon(t, cfg)

	for _, rt := range webUIStubRoutes() {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			d.srv.Handler.ServeHTTP(rec, httptest.NewRequest(rt.method, rt.path, nil))
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("missing credential: status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}

			req := httptest.NewRequest(rt.method, rt.path, nil)
			req.SetBasicAuth("operator", "secret-token")
			rec = httptest.NewRecorder()
			d.srv.Handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotImplemented {
				t.Errorf("valid credential: status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body)
			}
		})
	}
}

// TestBuild_WebEnabled_AuthDisabled_Dashboard_RendersOK proves GET / now
// reaches the real Dashboard handler (docs/ARCHITECTURE.md §10) rather
// than the stub — its own slice has landed, so it no longer belongs in
// webUIStubRoutes' 501 coverage above.
func TestBuild_WebEnabled_AuthDisabled_Dashboard_RendersOK(t *testing.T) {
	cfg := validConfig(t)
	cfg.Web = config.WebConfig{Enabled: true}
	d := buildDaemon(t, cfg)

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
}

// TestBuild_WebEnabled_AuthEnabled_Dashboard_RequiresBasicThenRendersOK
// proves the Dashboard route stays Basic-protected the same way every
// other Web UI route does, now that it answers with real content instead
// of a 501 stub.
func TestBuild_WebEnabled_AuthEnabled_Dashboard_RequiresBasicThenRendersOK(t *testing.T) {
	cfg := validConfig(t)
	cfg.Web = config.WebConfig{Enabled: true}
	cfg.Auth = config.AuthConfig{Enabled: true, TokenFile: writeTokenFile(t)}
	d := buildDaemon(t, cfg)

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("missing credential: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("operator", "secret-token")
	rec = httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("valid credential: status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
}

// TestBuild_WebEnabled_AuthDisabled_Printers_RendersOK proves GET
// /printers now reaches the real Printers handler (docs/ARCHITECTURE.md
// §10) rather than the stub — its own slice has landed, so it no longer
// belongs in webUIStubRoutes' 501 coverage above.
func TestBuild_WebEnabled_AuthDisabled_Printers_RendersOK(t *testing.T) {
	cfg := validConfig(t)
	cfg.Web = config.WebConfig{Enabled: true}
	d := buildDaemon(t, cfg)

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/printers", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
}

// TestBuild_WebEnabled_AuthEnabled_Printers_RequiresBasicThenRendersOK
// proves the Printers route stays Basic-protected the same way every
// other Web UI route does, now that it answers with real content instead
// of a 501 stub.
func TestBuild_WebEnabled_AuthEnabled_Printers_RequiresBasicThenRendersOK(t *testing.T) {
	cfg := validConfig(t)
	cfg.Web = config.WebConfig{Enabled: true}
	cfg.Auth = config.AuthConfig{Enabled: true, TokenFile: writeTokenFile(t)}
	d := buildDaemon(t, cfg)

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/printers", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("missing credential: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	req := httptest.NewRequest(http.MethodGet, "/printers", nil)
	req.SetBasicAuth("operator", "secret-token")
	rec = httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("valid credential: status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
}

// TestBuild_WebEnabled_AuthDisabled_Assets_RendersOK proves GET /assets
// now reaches the real Assets handler (docs/ARCHITECTURE.md §10) rather
// than the stub — its own slice has landed, so it no longer belongs in
// webUIStubRoutes' 501 coverage above.
func TestBuild_WebEnabled_AuthDisabled_Assets_RendersOK(t *testing.T) {
	cfg := validConfig(t)
	cfg.Web = config.WebConfig{Enabled: true}
	d := buildDaemon(t, cfg)

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
}

// TestBuild_WebEnabled_AuthEnabled_Assets_RequiresBasicThenRendersOK
// proves the Assets routes stay Basic-protected the same way every other
// Web UI route does: GET /assets now answers with real content instead
// of a 501 stub, and both mutating routes (POST /assets, POST
// /assets/{name}/delete) still require the same credential before
// reaching AssetsHandler at all.
func TestBuild_WebEnabled_AuthEnabled_Assets_RequiresBasicThenRendersOK(t *testing.T) {
	cfg := validConfig(t)
	cfg.Web = config.WebConfig{Enabled: true}
	cfg.Auth = config.AuthConfig{Enabled: true, TokenFile: writeTokenFile(t)}
	d := buildDaemon(t, cfg)

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("missing credential: GET /assets status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	req := httptest.NewRequest(http.MethodGet, "/assets", nil)
	req.SetBasicAuth("operator", "secret-token")
	rec = httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("valid credential: GET /assets status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}

	for _, rt := range []struct{ method, path string }{
		{http.MethodPost, "/assets"},
		{http.MethodPost, "/assets/logo.png/delete"},
	} {
		rec := httptest.NewRecorder()
		d.srv.Handler.ServeHTTP(rec, httptest.NewRequest(rt.method, rt.path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("missing credential: %s %s status = %d, want %d", rt.method, rt.path, rec.Code, http.StatusUnauthorized)
		}
	}
}

// TestBuild_WebEnabled_AuthDisabled_Preview_RendersOK proves GET /preview
// now reaches the real Preview handler (docs/ARCHITECTURE.md §10) rather
// than the stub — its own slice has landed, so it no longer belongs in
// webUIStubRoutes' 501 coverage above.
func TestBuild_WebEnabled_AuthDisabled_Preview_RendersOK(t *testing.T) {
	cfg := validConfig(t)
	cfg.Web = config.WebConfig{Enabled: true}
	d := buildDaemon(t, cfg)

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/preview", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
}

// TestBuild_WebEnabled_AuthEnabled_Preview_RequiresBasicThenRendersOK
// proves the Preview routes stay Basic-protected the same way every other
// Web UI route does: GET /preview now answers with real content instead
// of a 501 stub, and POST /preview still requires the same credential
// before reaching PreviewHandler at all.
func TestBuild_WebEnabled_AuthEnabled_Preview_RequiresBasicThenRendersOK(t *testing.T) {
	cfg := validConfig(t)
	cfg.Web = config.WebConfig{Enabled: true}
	cfg.Auth = config.AuthConfig{Enabled: true, TokenFile: writeTokenFile(t)}
	d := buildDaemon(t, cfg)

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/preview", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("missing credential: GET /preview status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	req := httptest.NewRequest(http.MethodGet, "/preview", nil)
	req.SetBasicAuth("operator", "secret-token")
	rec = httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("valid credential: GET /preview status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}

	rec = httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, webuiPreviewFormRequest())
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("missing credential: POST /preview status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestBuild_WebEnabled_APIRoutesStayBearerProtected proves mounting the
// Web UI behind auth.Basic on "/" doesn't loosen or replace /api/v1's
// existing Bearer requirement — the two subtrees keep independent
// middleware even though they now share one *http.Server.
func TestBuild_WebEnabled_APIRoutesStayBearerProtected(t *testing.T) {
	cfg := validConfig(t)
	cfg.Web = config.WebConfig{Enabled: true}
	cfg.Auth = config.AuthConfig{Enabled: true, TokenFile: writeTokenFile(t)}
	d := buildDaemon(t, cfg)

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, previewRequest())
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("missing Bearer credential: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	req := previewRequest()
	req.SetBasicAuth("operator", "secret-token")
	rec = httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Basic credential against /api/v1: status = %d, want %d (API only accepts Bearer)", rec.Code, http.StatusUnauthorized)
	}
}

func TestLoadAndBuild_MissingConfigFile_Propagates(t *testing.T) {
	_, err := loadAndBuild(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("loadAndBuild: expected error, got nil")
	}
	if !apperr.Is(err, apperr.KindNotFound) {
		t.Errorf("loadAndBuild: err = %v, want apperr.KindNotFound", err)
	}
}

func TestBuild_AuthEnabled_MissingTokenFile_Propagates(t *testing.T) {
	cfg := validConfig(t)
	cfg.Auth = config.AuthConfig{Enabled: true, TokenFile: filepath.Join(t.TempDir(), "does-not-exist")}

	_, err := build(cfg)
	if err == nil {
		t.Fatal("build: expected error, got nil")
	}
	if !apperr.Is(err, apperr.KindNotFound) {
		t.Errorf("build: err = %v, want apperr.KindNotFound", err)
	}
}

func TestDaemon_Serve_ListenFailure_ReturnsError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	cfg := validConfig(t)
	cfg.Server.Address = ln.Addr().String()

	d := buildDaemon(t, cfg)

	// Calling ListenAndServe directly (rather than d.run(), which also
	// calls d.startWorker) exercises the same startup-failure path
	// without also starting the background queue worker, which would
	// otherwise leak past the end of the test with nothing to cancel it.
	if err := d.srv.ListenAndServe(); err == nil {
		t.Fatal("ListenAndServe: expected error for an address already in use, got nil")
	}
}
