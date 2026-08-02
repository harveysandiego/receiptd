package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/harveysandiego/receiptd/internal/api"
)

func TestVersionHandlerReportsBuildIdentity(t *testing.T) {
	rec := httptest.NewRecorder()
	h := api.NewVersionHandler("0.5.1", "e4c9007", "2026-08-03T09:00:00Z")

	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/version", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Header().Get("Content-Type"), "application/json"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	if got, want := rec.Header().Get("Cache-Control"), "no-store"; got != want {
		t.Errorf("Cache-Control = %q, want %q", got, want)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	want := map[string]string{"version": "0.5.1", "commit": "e4c9007", "date": "2026-08-03T09:00:00Z"}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("body[%q] = %q, want %q", k, body[k], v)
		}
	}
	if len(body) != len(want) {
		t.Errorf("body has %d fields (%v), want %d", len(body), body, len(want))
	}
}
