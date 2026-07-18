package api

import (
	"errors"
	"net/http"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

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
