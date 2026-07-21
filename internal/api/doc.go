// Package api implements Receiptd's versioned REST handlers
// (/api/v1/...), translating HTTP requests into app.Service calls and
// apperr.Kind values into HTTP status codes (KindValidation→400,
// KindNotFound→404, KindUnauthorized→401, KindTransient→503,
// KindPermanent→500). See docs/ARCHITECTURE.md §5.
//
// This package is the trust boundary between an API client and Receiptd's
// internals: a 4xx response body carries the actionable underlying error
// detail, but a 5xx response body is always the fixed, generic
// "internal server error" message — never a wrapped error, filesystem or
// database path, network error, or apperr.Error Op — with the real error
// logged server-side instead. See writeError.
package api
