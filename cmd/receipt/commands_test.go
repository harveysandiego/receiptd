package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestConfig writes a minimal, valid config.yaml pointing at address
// and returns its path. Every path is confined to t.TempDir().
func writeTestConfig(t *testing.T, address string) string {
	t.Helper()
	dir := t.TempDir()
	yaml := "server:\n" +
		"  address: \"" + address + "\"\n" +
		"auth:\n" +
		"  enabled: false\n" +
		"queue:\n" +
		"  store: memory\n" +
		"  path: \"" + filepath.ToSlash(filepath.Join(dir, "queue.db")) + "\"\n" +
		"  max_attempts: 3\n" +
		"  retry_backoff: 5s\n"
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	return path
}

// serverAddress strips httptest.Server's "http://" scheme, since
// cfg.Server.Address is a bare host:port (docs/ARCHITECTURE.md §7).
func serverAddress(srv *httptest.Server) string {
	return strings.TrimPrefix(srv.URL, "http://")
}

func execCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(new(bytes.Buffer))
	err := cmd.Execute()
	return out.String(), err
}

func TestPreviewCmd_Success_WritesResponseToOutFile(t *testing.T) {
	want := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/preview", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(want)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfgPath := writeTestConfig(t, serverAddress(srv))
	in := writeInput(t, validReceiptJSON)
	out := filepath.Join(t.TempDir(), "preview.png")

	if _, err := execCLI(t, "preview", in, "--out", out, "--config", cfgPath); err != nil {
		t.Fatalf("execute preview error = %v, want nil", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("os.ReadFile(%q): %v", out, err)
	}
	if !bytes.Equal(data, want) {
		t.Errorf("preview output = %x, want %x", data, want)
	}
}

func TestPrintCmd_Success_PrintsJobID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/print", func(w http.ResponseWriter, r *http.Request) {
		var req printRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		if req.Printer != "front-desk" {
			t.Errorf("Printer = %q, want %q", req.Printer, "front-desk")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(printResponse{JobID: "abc123"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfgPath := writeTestConfig(t, serverAddress(srv))
	in := writeInput(t, validReceiptJSON)

	out, err := execCLI(t, "print", in, "--printer", "front-desk", "--config", cfgPath)
	if err != nil {
		t.Fatalf("execute print error = %v, want nil", err)
	}
	if !strings.Contains(out, "abc123") {
		t.Errorf("output = %q, want it to contain the job ID", out)
	}
}

func TestJobsCmd_Success_PrintsJobStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(jobStatusResponse{ID: "abc123", State: "pending"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfgPath := writeTestConfig(t, serverAddress(srv))

	out, err := execCLI(t, "jobs", "abc123", "--config", cfgPath)
	if err != nil {
		t.Fatalf("execute jobs error = %v, want nil", err)
	}
	if !strings.Contains(out, "abc123") || !strings.Contains(out, "pending") {
		t.Errorf("output = %q, want it to contain the job id and state", out)
	}
}

func TestPrintCmd_APIErrorResponse_PropagatesCleanly(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/print", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(errorResponse{Error: "invalid receipt"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfgPath := writeTestConfig(t, serverAddress(srv))
	in := writeInput(t, validReceiptJSON)

	_, err := execCLI(t, "print", in, "--printer", "front-desk", "--config", cfgPath)
	if err == nil {
		t.Fatal("execute print error = nil, want non-nil for a 400 API response")
	}
	if !strings.Contains(err.Error(), "invalid receipt") {
		t.Errorf("error = %q, want it to contain the API's error message", err.Error())
	}
}

func TestPreviewCmd_InvalidDaemonAddress_FailsCleanlyWithoutWritingOutput(t *testing.T) {
	// Nothing listens on this address once ln is closed, so config.yaml
	// names a daemon that can never be reached.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("ln.Close: %v", err)
	}

	cfgPath := writeTestConfig(t, addr)
	in := writeInput(t, validReceiptJSON)
	out := filepath.Join(t.TempDir(), "preview.png")

	if _, err := execCLI(t, "preview", in, "--out", out, "--config", cfgPath); err == nil {
		t.Fatal("execute preview error = nil, want non-nil for an unreachable daemon address")
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Errorf("os.Stat(%q) error = %v, want IsNotExist (no file should be written on failure)", out, statErr)
	}
}
