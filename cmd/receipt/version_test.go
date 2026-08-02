package main

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestAPIClient_ServerVersion_Success_ReturnsDecodedResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"0.5.1","commit":"e4c9007","date":"2026-08-03T09:00:00Z"}`))
	})
	client := newTestClient(t, mux, "")

	got, err := client.serverVersion(context.Background())
	if err != nil {
		t.Fatalf("serverVersion() error = %v, want nil", err)
	}
	want := versionResponse{Version: "0.5.1", Commit: "e4c9007", Date: "2026-08-03T09:00:00Z"}
	if *got != want {
		t.Errorf("serverVersion() = %+v, want %+v", *got, want)
	}
}

func TestAPIClient_ServerVersion_ErrorResponse_ReturnsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
	client := newTestClient(t, mux, "")

	if _, err := client.serverVersion(context.Background()); err == nil {
		t.Fatal("serverVersion() error = nil, want an error")
	}
}

// The daemon being unreachable must not fail the command — configPath
// points at a file that doesn't exist, so serverVersion errors before any
// request is attempted.
func TestVersionCmd_DaemonUnavailable_StillPrintsCLIVersion(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version", "--config", t.TempDir() + "/absent.yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	got := out.String()
	if !strings.Contains(got, "receipt "+version) {
		t.Errorf("output = %q, want this CLI's version", got)
	}
	if !strings.Contains(got, "receiptd unavailable") {
		t.Errorf("output = %q, want the daemon reported as unavailable", got)
	}
}
