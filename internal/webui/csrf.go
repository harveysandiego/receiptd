package webui

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
)

// csrfSecret is a random per-process key used only to sign/verify the
// CSRF token embedded in every state-changing form (docs/adr/0027) —
// generated once at process start, never persisted, never sent to a
// client in any form.
var csrfSecret = mustRandomBytes(32)

func mustRandomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read failing means the platform's entropy source is
		// broken — not a condition this package can recover from or
		// should silently downgrade past.
		panic("webui: failed to generate CSRF secret: " + err.Error())
	}
	return b
}

// csrfTokenMessage is the fixed message csrfSecret signs. The token needs
// no per-render randomness of its own (docs/adr/0027): its
// unforgeability comes entirely from csrfSecret never leaving the
// server, not from the message varying, so the same signed value is
// reused for the life of the process.
const csrfTokenMessage = "receiptd-webui-csrf"

// csrfFieldName is the hidden field every CSRF-protected form carries.
const csrfFieldName = "csrf_token"

// csrfToken returns the current CSRF token, embedded as a hidden field in
// every state-changing form (docs/adr/0027).
func csrfToken() string {
	mac := hmac.New(sha256.New, csrfSecret)
	mac.Write([]byte(csrfTokenMessage))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// errCSRF is a plain, unwrapped error reused as apperr.Wrap's cause at
// every call site that rejects a request over a missing/invalid CSRF
// token — not a sentinel compared with ==, just shared message text.
var errCSRF = errors.New("missing or invalid CSRF token")

// verifyCSRF reports whether r carries a valid csrfFieldName form value,
// compared in constant time so a mismatch's timing can't leak how much of
// the token guess was correct. The caller must already have called
// r.ParseForm (or r.ParseMultipartForm) before this is meaningful.
func verifyCSRF(r *http.Request) bool {
	got := r.FormValue(csrfFieldName)
	want := csrfToken()
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
