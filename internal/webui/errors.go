package webui

import (
	"errors"
	"net/http"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// statusForErr maps err's apperr.Kind to the HTTP status a Web UI page
// returns for it, mirroring internal/api/status.go's statusForKind for the
// same apperr.Kind set (docs/ARCHITECTURE.md §5) — kept as its own copy
// rather than a shared import, since an HTML error page has nothing else
// in common with internal/api's JSON error responses.
func statusForErr(err error) int {
	var e *apperr.Error
	if errors.As(err, &e) {
		switch e.Kind {
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
		}
	}
	return http.StatusInternalServerError
}

// errorPage is the data shape for a Web UI page that failed to load. It
// reuses base.tmpl's default "content" block (a bare title and message)
// rather than a dedicated template — an error page has nothing
// page-specific to render.
type errorPage struct {
	Title   string
	Message string
}

// renderError writes a generic error page for err, mapping its
// apperr.Kind to an HTTP status without echoing err's own message to the
// browser (the same trust-boundary rule internal/api/job_status.go
// applies to a Job's LastError). Callers decide whether/when to call it;
// renderError only decides how to present the result consistently.
func renderError(w http.ResponseWriter, title string, err error) {
	render(w, baseTemplate, statusForErr(err), "base", errorPage{
		Title:   title,
		Message: title + " could not load right now. Please try again shortly.",
	})
}
