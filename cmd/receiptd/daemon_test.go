package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/config"
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

// buildDaemon calls build and registers cleanup for the printer-log file
// handle it opens, so t.TempDir() can remove it afterward — Windows
// refuses to delete a file that's still open.
func buildDaemon(t *testing.T, cfg *config.Config) *daemon {
	t.Helper()
	d, err := build(cfg)
	if err != nil {
		t.Fatalf("build() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = d.logSink.Close() })
	return d
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

func TestBuild_LogSinkIsPrinterLogFile(t *testing.T) {
	cfg := validConfig(t)
	d := buildDaemon(t, cfg)

	rec := httptest.NewRecorder()
	d.srv.Handler.ServeHTTP(rec, printRequest())
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/v1/print status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	if err := d.queue.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}

	data, err := os.ReadFile(printerLogPath(cfg.Queue))
	if err != nil {
		t.Fatalf("reading printer log file: %v", err)
	}
	pngSignature := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if !bytes.HasPrefix(data, pngSignature) {
		t.Errorf("printer log file did not start with the PNG signature: %x", data)
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

func TestDaemon_Serve_ClosesLogSinkOnReturn(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	cfg := validConfig(t)
	cfg.Server.Address = ln.Addr().String()

	d, err := build(cfg)
	if err != nil {
		t.Fatalf("build() error = %v, want nil", err)
	}

	// serve() starts the background queue worker before ListenAndServe
	// fails on the already-bound address, so this test (unlike
	// TestDaemon_Serve_ListenFailure_ReturnsError above) does leak that
	// worker goroutine for the rest of the test binary's run — accepted
	// here specifically to exercise serve()'s own close-on-return
	// behaviour end to end, rather than build()'s or the raw *http.Server's.
	if err := d.serve(); err == nil {
		t.Fatal("serve(): expected error for an address already in use, got nil")
	}

	if closeErr := d.logSink.Close(); !errors.Is(closeErr, os.ErrClosed) {
		t.Errorf("logSink.Close() after serve() returned = %v, want os.ErrClosed (serve should already have closed it)", closeErr)
	}
}
