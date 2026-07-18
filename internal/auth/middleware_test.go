package auth_test

import (
	"encoding/base64"
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

func TestBasic_ValidCredentials(t *testing.T) {
	var called bool
	h := auth.Basic("secret-token")(nextHandler(t, &called))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("receiptd", "secret-token")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if !called {
		t.Error("next handler was not invoked for a valid credential")
	}
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}

func TestBasic_UsernameIsNotChecked(t *testing.T) {
	// docs/ARCHITECTURE.md §1: Basic-Auth shares Bearer's single configured
	// token, checked as the password. Any non-empty username is accepted.
	usernames := []string{"receiptd", "admin", "anything-at-all"}

	for _, username := range usernames {
		t.Run(username, func(t *testing.T) {
			var called bool
			h := auth.Basic("secret-token")(nextHandler(t, &called))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.SetBasicAuth(username, "secret-token")
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if !called {
				t.Errorf("next handler was not invoked for username %q with a valid password", username)
			}
			if got, want := rec.Code, http.StatusOK; got != want {
				t.Errorf("status = %d, want %d", got, want)
			}
		})
	}
}

func TestBasic_EmptyUsernameFails(t *testing.T) {
	var called bool
	h := auth.Basic("secret-token")(nextHandler(t, &called))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("", "secret-token")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if called {
		t.Error("next handler was invoked for an empty username")
	}
	if got, want := rec.Code, http.StatusUnauthorized; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}

func TestBasic_Unauthorized(t *testing.T) {
	tests := []struct {
		name   string
		header string // "" means no Authorization header at all
	}{
		{name: "missing credential", header: ""},
		{name: "incorrect password", header: basicHeader("receiptd", "wrong-token")},
		{name: "malformed credential: wrong scheme", header: "Bearer secret-token"},
		{name: "malformed credential: no scheme", header: "secret-token"},
		{name: "malformed credential: not base64", header: "Basic not-valid-base64!!"},
		{name: "malformed credential: no colon separator", header: "Basic " + base64Encode("receiptdsecret-token")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			h := auth.Basic("secret-token")(nextHandler(t, &called))

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
			if got, want := rec.Header().Get("WWW-Authenticate"), `Basic realm="Receiptd"`; got != want {
				t.Errorf("WWW-Authenticate = %q, want %q", got, want)
			}
			if strings.Contains(rec.Body.String(), "secret-token") {
				t.Errorf("response body leaked the expected token: %q", rec.Body.String())
			}
		})
	}
}

func TestBasic_EmptyExpectedTokenAlwaysRejects(t *testing.T) {
	var called bool
	h := auth.Basic("")(nextHandler(t, &called))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("receiptd", "")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if called {
		t.Error("next handler was invoked despite an empty expected token")
	}
	if got, want := rec.Code, http.StatusUnauthorized; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}

// basicHeader builds a well-formed "Basic <base64>" Authorization header
// value for the given credentials, for table-driven tests that need to
// construct the header string directly rather than via r.SetBasicAuth.
func basicHeader(username, password string) string {
	return "Basic " + base64Encode(username+":"+password)
}

func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
