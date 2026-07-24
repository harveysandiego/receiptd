package webui

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRenderStub_ExecutesBaseTemplateWithoutError exercises the render
// path in isolation from routing/auth: baseTemplate (parsed from the
// embedded web.FS at package init via template.Must, which would already
// have panicked had parsing failed) actually executes against real data
// and produces HTML, proving embedding, parsing, and rendering all work
// end to end.
func TestRenderStub_ExecutesBaseTemplateWithoutError(t *testing.T) {
	rec := httptest.NewRecorder()

	renderStub(rec, "Example")

	if got, want := rec.Header().Get("Content-Type"), "text/html; charset=utf-8"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("body is empty, want the executed base template's HTML")
	}
	if !strings.Contains(rec.Body.String(), "Example is not implemented yet.") {
		t.Errorf("body = %s, want it to contain the stub message", rec.Body)
	}
}
