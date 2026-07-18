package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/auth"
)

func nextHandler(t *testing.T, called *bool) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	})
}

func TestBearer_ValidCredential(t *testing.T) {
	var called bool
	h := auth.Bearer("secret-token")(nextHandler(t, &called))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if !called {
		t.Error("next handler was not invoked for a valid credential")
	}
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}

func TestBearer_ValidCredential_SchemeCaseInsensitive(t *testing.T) {
	tests := []string{
		"Bearer secret-token",
		"bearer secret-token",
		"BEARER secret-token",
		"BeArEr secret-token",
	}

	for _, header := range tests {
		t.Run(header, func(t *testing.T) {
			var called bool
			h := auth.Bearer("secret-token")(nextHandler(t, &called))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", header)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if !called {
				t.Error("next handler was not invoked for a valid credential")
			}
			if got, want := rec.Code, http.StatusOK; got != want {
				t.Errorf("status = %d, want %d", got, want)
			}
		})
	}
}

func TestBearer_Unauthorized(t *testing.T) {
	tests := []struct {
		name   string
		header string // "" means no Authorization header at all
	}{
		{name: "missing credential", header: ""},
		{name: "malformed credential: wrong scheme", header: "Basic secret-token"},
		{name: "malformed credential: no scheme", header: "secret-token"},
		{name: "malformed credential: no space after scheme", header: "Bearersecret-token"},
		{name: "malformed credential: empty token", header: "Bearer "},
		{name: "incorrect credential", header: "Bearer wrong-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			h := auth.Bearer("secret-token")(nextHandler(t, &called))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if called {
				t.Error("next handler was invoked for an unauthorized request")
			}
			if got, want := rec.Code, http.StatusUnauthorized; got != want {
				t.Errorf("status = %d, want %d", got, want)
			}
			if got, want := rec.Header().Get("WWW-Authenticate"), "Bearer"; got != want {
				t.Errorf("WWW-Authenticate = %q, want %q", got, want)
			}
			if strings.Contains(rec.Body.String(), "secret-token") {
				t.Errorf("response body leaked the expected token: %q", rec.Body.String())
			}
		})
	}
}

func TestBearer_EmptyExpectedTokenAlwaysRejects(t *testing.T) {
	var called bool
	h := auth.Bearer("")(nextHandler(t, &called))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if called {
		t.Error("next handler was invoked despite an empty expected token")
	}
	if got, want := rec.Code, http.StatusUnauthorized; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}
