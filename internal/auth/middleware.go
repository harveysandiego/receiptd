package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Bearer returns middleware that protects next behind the
// "Authorization: Bearer <token>" scheme, checked against the given
// expected token (typically the result of ResolveToken). A missing,
// malformed, or incorrect credential gets a 401 response with a
// WWW-Authenticate: Bearer challenge and never reaches next; the
// expected token is never included in the response.
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

// tokensEqual reports whether supplied matches expected using a
// constant-time comparison, so a failed attempt can't be used to probe
// the expected token via response timing. An empty expected token never
// matches, so a misconfigured (unresolved) token can't be bypassed with
// an empty credential. Shared by Bearer and future Basic-Auth middleware
// (docs/ARCHITECTURE.md §1).
func tokensEqual(supplied, expected string) bool {
	if expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(supplied), []byte(expected)) == 1
}
