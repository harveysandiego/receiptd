// Package apperr defines Receiptd's typed error taxonomy: a small,
// closed set of failure Kinds (validation, not found, transient,
// permanent, unauthorized) wrapped with the operation that produced
// them, so callers across the API, job queue, and logs can make
// decisions based on what kind of failure occurred rather than by
// matching error message text.
//
// See docs/ARCHITECTURE.md §5 and docs/adr/0005-error-handling.md for
// the full design and rationale. apperr has no internal dependencies —
// almost everything else in this module depends on it.
package apperr
