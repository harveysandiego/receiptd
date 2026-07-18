package api

import (
	"errors"
	"net/http"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// maxRequestBodyBytes caps how much of a request body PreviewHandler and
// PrintHandler will read before giving up, so a client can't exhaust
// server memory with an oversized body (json.Decode has no size limit of
// its own). 10 MiB is far beyond any Milestone 2 Receipt (text-only
// elements) while leaving headroom; it's a sanity cap, not a considered
// per-element-type budget — Milestone 3's Image/Asset elements may need
// this revisited.
const maxRequestBodyBytes = 10 << 20

// isBodyTooLarge reports whether err is the error http.MaxBytesReader
// produces once a request body exceeds maxRequestBodyBytes.
func isBodyTooLarge(err error) bool {
	var mbErr *http.MaxBytesError
	return errors.As(err, &mbErr)
}

// statusForKind maps an apperr.Kind to the HTTP status code the API
// returns for it. See docs/ARCHITECTURE.md §5.
func statusForKind(k apperr.Kind) int {
	switch k {
	case apperr.KindValidation:
		return http.StatusBadRequest
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindUnauthorized:
		return http.StatusUnauthorized
	case apperr.KindTransient:
		return http.StatusServiceUnavailable
	case apperr.KindPermanent:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// statusForError returns the HTTP status err maps to via statusForKind if
// err wraps an *apperr.Error, or fallback otherwise.
func statusForError(err error, fallback int) int {
	var e *apperr.Error
	if errors.As(err, &e) {
		return statusForKind(e.Kind)
	}
	return fallback
}
