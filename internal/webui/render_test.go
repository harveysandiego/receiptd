package webui

import (
	"html/template"
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

	render(rec, "0.5.1", baseTemplate, http.StatusOK, errorPage{Message: "hello"})

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

func TestRender_RendersVersionInFooter(t *testing.T) {
	rec := httptest.NewRecorder()

	render(rec, "0.5.1", baseTemplate, http.StatusOK, errorPage{Message: "hello"})

	if got := rec.Body.String(); !strings.Contains(got, "<footer><p>Receiptd 0.5.1</p></footer>") {
		t.Errorf("body = %s, want it to contain the version footer", got)
	}
}

// A Service left without BuildInfo (only tests do this) must not render a
// footer trailing an empty version.
func TestRender_NoVersion_OmitsFooter(t *testing.T) {
	rec := httptest.NewRecorder()

	render(rec, "", baseTemplate, http.StatusOK, errorPage{Message: "hello"})

	if got := rec.Body.String(); strings.Contains(got, "<footer") {
		t.Errorf("body = %s, want no footer", got)
	}
}

// The envelope must stay invisible to a page: its own "content" block
// still receives its own model, not the wrapper.
func TestRender_ContentBlockReceivesThePageModel(t *testing.T) {
	rec := httptest.NewRecorder()

	render(rec, "0.5.1", dashboardTemplate, http.StatusOK, dashboardPage{StatusMessage: "page-owned"})

	body := rec.Body.String()
	if !strings.Contains(body, "page-owned") {
		t.Errorf("body = %s, want the page's own model rendered by its content block", body)
	}
	if !strings.Contains(body, "Receiptd 0.5.1") {
		t.Errorf("body = %s, want the footer alongside the page's own content", body)
	}
}

// TestRender_TemplateExecutionFails_RespondsWithGenericErrorNotPartialBody
// proves a template execution failure (here, forced by a template
// referencing a field the passed data doesn't have) never leaves headers
// committed to the caller's requested status with a partially-written or
// empty body — render must buffer execution and only write once it
// succeeds.
func TestRender_TemplateExecutionFails_RespondsWithGenericErrorNotPartialBody(t *testing.T) {
	badTmpl := template.Must(template.New("base").Parse(`{{.NoSuchField}}`))
	rec := httptest.NewRecorder()

	render(rec, "0.5.1", badTmpl, http.StatusOK, errorPage{Message: "hello"})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d (the caller asked for 200, but execution failed, so that status must never be committed)", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "hello") {
		t.Errorf("body = %s, want no partial template output written", rec.Body)
	}
}
