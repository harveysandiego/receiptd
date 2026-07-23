// Package api implements Receiptd's versioned REST handlers
// (/api/v1/...), translating HTTP requests into app.Service calls and
// apperr.Kind values into HTTP status codes (KindValidation→400,
// KindNotFound→404, KindUnauthorized→401, KindTransient→503,
// KindPermanent→500). See docs/ARCHITECTURE.md §5.
//
// This package is the trust boundary between an API client and Receiptd's
// internals: a 4xx response body carries the actionable underlying error
// detail, but a 5xx body is always the fixed "internal server error"
// message, with the real error logged server-side (see writeError). The
// same boundary applies to a Job's LastError inside a 200 response —
// diagnostic detail a background Processor produced, not something the
// client caused — which JobStatusHandler replaces with a fixed message
// (see sanitizedLastError).
package api
