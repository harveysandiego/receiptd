package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Bearer returns middleware that protects next behind the "Authorization:
// Bearer <token>" scheme, checked against the expected token (typically
// from ResolveToken). A missing, malformed, or incorrect credential gets
// a 401 with a WWW-Authenticate: Bearer challenge and never reaches next;
// the expected token is never echoed in the response.
func Bearer(token string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if supplied, ok := bearerCredential(r); ok && tokensEqual(supplied, token) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		})
	}
}

// Basic returns middleware that protects next behind HTTP Basic
// Authentication against the same shared token as Bearer
// (docs/ARCHITECTURE.md §1). The password is compared to token; the
// username must be non-empty but is otherwise not checked, matching the
// token-as-password convention. A missing, malformed, or incorrect
// credential gets a 401 with a WWW-Authenticate: Basic challenge and
// never reaches next; the expected token is never echoed in the response.
func Basic(token string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()
			if ok && username != "" && tokensEqual(password, token) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("WWW-Authenticate", `Basic realm="Receiptd"`)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		})
	}
}

// bearerCredential extracts the credential from a "Bearer <token>"
// Authorization header, reporting false if the header is absent or uses
// a different scheme. The scheme is matched case-insensitively per
// RFC 9110 §11.1.
func bearerCredential(r *http.Request) (string, bool) {
	scheme, credential, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	return credential, true
}

// tokensEqual reports whether supplied matches expected in constant time,
// so a failed attempt can't probe the expected token via response timing.
// An empty expected token never matches, so a misconfigured (unresolved)
// token can't be bypassed with an empty credential. Shared by Bearer and
// Basic.
func tokensEqual(supplied, expected string) bool {
	if expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(supplied), []byte(expected)) == 1
}
