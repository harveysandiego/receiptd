// Package api implements Receiptd's versioned REST handlers
// (/api/v1/...), translating HTTP requests into app.Service calls and
// apperr.Kind values into HTTP status codes (KindValidation→400,
// KindNotFound→404, KindUnauthorized→401, KindTransient→503,
// KindPermanent→500). See docs/ARCHITECTURE.md §5.
package api
