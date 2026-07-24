package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRender_ExecutesBaseTemplateWithoutError exercises render() in
// isolation from any one page's handler, reusing errorPage as a stand-in
// data shape: baseTemplate (parsed from the embedded web.FS at package
// init via template.Must, which would already have panicked had parsing
// failed) actually executes against real data and produces HTML, proving
// embedding, parsing, and rendering all work end to end.
func TestRender_ExecutesBaseTemplateWithoutError(t *testing.T) {
	rec := httptest.NewRecorder()

	render(rec, baseTemplate, http.StatusOK, errorPage{Title: "Example", Message: "hello"})

	if got, want := rec.Header().Get("Content-Type"), "text/html; charset=utf-8"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("body is empty, want the executed base template's HTML")
	}
	if !strings.Contains(rec.Body.String(), "hello") {
		t.Errorf("body = %s, want it to contain the rendered message", rec.Body)
	}
}
