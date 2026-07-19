package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

func newTestClient(t *testing.T, handler http.Handler, token string) *apiClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &apiClient{baseURL: srv.URL, token: token, http: srv.Client()}
}

func sampleReceipt() receipt.Receipt {
	return receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: "hi"}}}
}

func TestAPIClient_Preview_Success_ReturnsResponseBytes(t *testing.T) {
	want := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/preview", func(w http.ResponseWriter, r *http.Request) {
		var req previewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		if len(req.Receipt.Elements) != 1 {
			t.Errorf("len(Elements) = %d, want 1", len(req.Receipt.Elements))
		}
		if req.Printer != "front-desk" {
			t.Errorf("Printer = %q, want %q", req.Printer, "front-desk")
		}
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(want)
	})
	client := newTestClient(t, mux, "")

	got, err := client.preview(context.Background(), sampleReceipt(), "front-desk")
	if err != nil {
		t.Fatalf("preview() error = %v, want nil", err)
	}
	if string(got) != string(want) {
		t.Errorf("preview() = %x, want %x", got, want)
	}
}

func TestAPIClient_Print_Success_ReturnsJobID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/print", func(w http.ResponseWriter, r *http.Request) {
		var req printRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.Printer != "front-desk" {
			t.Errorf("Printer = %q, want %q", req.Printer, "front-desk")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(printResponse{JobID: "abc123"})
	})
	client := newTestClient(t, mux, "")

	jobID, err := client.print(context.Background(), sampleReceipt(), "front-desk")
	if err != nil {
		t.Fatalf("print() error = %v, want nil", err)
	}
	if jobID != "abc123" {
		t.Errorf("print() = %q, want %q", jobID, "abc123")
	}
}

func TestAPIClient_JobStatus_Success_ReturnsJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if got := r.PathValue("id"); got != "abc123" {
			t.Errorf("path id = %q, want %q", got, "abc123")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(jobStatusResponse{ID: "abc123", State: "pending"})
	})
	client := newTestClient(t, mux, "")

	job, err := client.jobStatus(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("jobStatus() error = %v, want nil", err)
	}
	if job.ID != "abc123" || job.State != "pending" {
		t.Errorf("jobStatus() = %+v, want ID=abc123 State=pending", job)
	}
}

func TestAPIClient_SendsBearerTokenWhenSet(t *testing.T) {
	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/preview", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})
	client := newTestClient(t, mux, "secret-token")

	if _, err := client.preview(context.Background(), sampleReceipt(), "front-desk"); err != nil {
		t.Fatalf("preview() error = %v, want nil", err)
	}
	if want := "Bearer secret-token"; gotAuth != want {
		t.Errorf("Authorization header = %q, want %q", gotAuth, want)
	}
}

func TestAPIClient_NoTokenConfigured_OmitsAuthorizationHeader(t *testing.T) {
	var sawHeader bool
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/preview", func(w http.ResponseWriter, r *http.Request) {
		sawHeader = r.Header.Get("Authorization") != ""
		w.WriteHeader(http.StatusOK)
	})
	client := newTestClient(t, mux, "")

	if _, err := client.preview(context.Background(), sampleReceipt(), "front-desk"); err != nil {
		t.Fatalf("preview() error = %v, want nil", err)
	}
	if sawHeader {
		t.Error("Authorization header was sent despite no token being configured")
	}
}

func TestAPIClient_NonJSONErrorResponse_PropagatesCleanly(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/print", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})
	client := newTestClient(t, mux, "")

	_, err := client.print(context.Background(), sampleReceipt(), "front-desk")
	if err == nil {
		t.Fatal("print() error = nil, want non-nil for a 500 response")
	}
}

func TestAPIClient_APIErrorResponse_PropagatesMessage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/print", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(errorResponse{Error: "invalid receipt: no elements"})
	})
	client := newTestClient(t, mux, "")

	_, err := client.print(context.Background(), sampleReceipt(), "front-desk")
	if err == nil {
		t.Fatal("print() error = nil, want non-nil for a 400 response")
	}
	if !strings.Contains(err.Error(), "invalid receipt: no elements") {
		t.Errorf("print() error = %q, want it to contain the API's error message", err.Error())
	}
}

func TestAPIClient_UnreachableAddress_PropagatesCleanly(t *testing.T) {
	// Nothing listens on this address once ln is closed, so the request
	// fails at the transport level (connection refused) rather than
	// receiving any HTTP response.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("ln.Close: %v", err)
	}

	client := &apiClient{baseURL: "http://" + addr, http: http.DefaultClient}

	if _, err := client.preview(context.Background(), sampleReceipt(), "front-desk"); err == nil {
		t.Fatal("preview() error = nil, want non-nil for an unreachable daemon address")
	}
}
