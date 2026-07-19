package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/config"
	"github.com/harveysandiego/receiptd/internal/printer"
)

// validConfig returns a minimal, valid *config.Config for build/loadAndBuild
// tests: an in-memory queue store and auth disabled. Every path is confined
// to t.TempDir() so tests never write into the repo's working directory.
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
	}
}

func previewRequest() *http.Request {
	body := `{"elements":[{"type":"text","content":"hi"}]}`
	return httptest.NewRequest(http.MethodPost, "/api/v1/preview", bytes.NewBufferString(body))
}

func printRequest() *http.Request {
	body := `{"printer":"front-desk","receipt":{"elements":[{"type":"text","content":"hi"}]}}`
	return httptest.NewRequest(http.MethodPost, "/api/v1/print", bytes.NewBufferString(body))
}

func writeTokenFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
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

	printers, profiles := buildPrinters(cfgs)

	if len(printers) != len(cfgs) {
		t.Errorf("len(printers) = %d, want %d", len(printers), len(cfgs))
	}
	if len(profiles) != len(cfgs) {
		t.Errorf("len(profiles) = %d, want %d", len(profiles), len(cfgs))
	}
	for _, c := range cfgs {
		if _, ok := printers[c.Name]; !ok {
			t.Errorf("printers[%q] missing, want a constructed printer.Printer", c.Name)
		}
		if got := profiles[c.Name]; got != c.Profile {
			t.Errorf("profiles[%q] = %+v, want %+v", c.Name, got, c.Profile)
		}
	}
}

func TestBuildPrinters_NoConfiguredPrinters_ReturnsEmptyMaps(t *testing.T) {
	printers, profiles := buildPrinters(nil)
	if len(printers) != 0 {
		t.Errorf("len(printers) = %d, want 0", len(printers))
	}
	if len(profiles) != 0 {
		t.Errorf("len(profiles) = %d, want 0", len(profiles))
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
	// validConfig(t) configures no printers at all — so a Job targeting
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

func TestBuild_AuthEnabled_RequiresBearerToken(t *testing.T) {
	cfg := validConfig(t)
	cfg.Auth = config.AuthConfig{Enabled: true, TokenFile: writeTokenFile(t, "secret-token")}

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

	// Calling ListenAndServe directly (rather than d.serve()) exercises the
	// same startup-failure path without also starting the background queue
	// worker, which has no cancellation in this slice and would otherwise
	// leak past the end of the test.
	if err := d.srv.ListenAndServe(); err == nil {
		t.Fatal("ListenAndServe: expected error for an address already in use, got nil")
	}
}
