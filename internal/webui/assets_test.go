package webui_test

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/assets"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/webui"
)

// uploadRequest builds a multipart/form-data POST /assets request with
// name as the asset name and content as the uploaded file's bytes,
// including the hidden csrf_token field (docs/adr/0027) every real
// upload.tmpl submission carries. token normally comes from
// csrfTokenFromPage against a prior GET /assets.
func uploadRequest(t *testing.T, token, name, content string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("csrf_token", token); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	if name != "" {
		if err := w.WriteField("name", name); err != nil {
			t.Fatalf("WriteField: %v", err)
		}
	}
	if content != "" || name != "" {
		fw, err := w.CreateFormFile("file", "upload.png")
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/assets", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

// uploadRequestNoFile builds a multipart/form-data POST /assets request
// carrying a name field and a valid csrf_token but no file part at all.
func uploadRequestNoFile(t *testing.T, token, name string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("csrf_token", token); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	if err := w.WriteField("name", name); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/assets", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

// deleteAssetRequest builds a form-encoded POST /assets/{name}/delete
// request carrying the hidden csrf_token field the delete form submits.
func deleteAssetRequest(token, name string) *http.Request {
	form := url.Values{"csrf_token": {token}}
	req := httptest.NewRequest(http.MethodPost, "/assets/"+name+"/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func newAssetsTestService() *app.Service {
	svc := app.New(queue.New(queue.NewMemoryStore(), dashboardNoopProcessor{}))
	svc.Assets = assets.NewMemoryStore()
	return svc
}

// TestAssetsHandler_List_RendersStoredAssets proves the happy path: every
// stored asset's name appears in the rendered body, sourced only through
// the ListAssets service seam.
func TestAssetsHandler_List_RendersStoredAssets(t *testing.T) {
	svc := newAssetsTestService()
	if err := svc.Assets.Put(context.Background(), "logo.png", []byte("data")); err != nil {
		t.Fatalf("Assets.Put: %v", err)
	}
	if err := svc.Assets.Put(context.Background(), "banner.png", []byte("data")); err != nil {
		t.Fatalf("Assets.Put: %v", err)
	}

	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "logo.png") {
		t.Errorf("body = %s, want it to contain %q", body, "logo.png")
	}
	if !strings.Contains(body, "banner.png") {
		t.Errorf("body = %s, want it to contain %q", body, "banner.png")
	}
}

// TestAssetsHandler_List_EmptyState_RendersNoAssetsMessage proves an
// instance with no stored assets renders a clear empty state instead of
// an empty or malformed list.
func TestAssetsHandler_List_EmptyState_RendersNoAssetsMessage(t *testing.T) {
	svc := newAssetsTestService()

	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body)
	}
	if body := rec.Body.String(); !strings.Contains(body, "No assets stored") {
		t.Errorf("body = %s, want it to contain the empty-state message", body)
	}
}

// TestAssetsHandler_List_ServiceError_RendersGenericErrorWithoutLeakingDetail
// proves a ListAssets failure produces a non-200 status and a generic
// message, never the underlying error text — the same contract
// dashboard_test.go and printers_test.go pin for their own handlers.
func TestAssetsHandler_List_ServiceError_RendersGenericErrorWithoutLeakingDetail(t *testing.T) {
	svc := app.New(queue.New(queue.NewMemoryStore(), dashboardNoopProcessor{}))
	svc.Assets = errListAssetStore{}

	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusInternalServerError, rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, "disk error") {
		t.Errorf("body leaks the underlying error detail: %s", body)
	}
	if !strings.Contains(body, "could not load") {
		t.Errorf("body = %s, want a generic could-not-load message", body)
	}
}

// errListAssetStore is an assets.Store test double whose List always
// fails, letting the service-error test observe error propagation
// without touching a real Store.
type errListAssetStore struct {
	assets.Store
}

func (errListAssetStore) List(_ context.Context) ([]assets.Info, error) {
	return nil, apperr.Wrap(apperr.KindPermanent, "assets.Store.List", errors.New("disk error"))
}

// TestAssetsHandler_Upload_StoresAssetThenRedirectsToList proves a valid
// multipart upload reaches Service.UploadAsset and redirects back to
// GET /assets, per the "deletion/upload should redirect back to
// /assets" requirement.
func TestAssetsHandler_Upload_StoresAssetThenRedirectsToList(t *testing.T) {
	svc := newAssetsTestService()

	router := webui.NewRouter(svc)
	token := csrfTokenFromPage(t, router, "/assets")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, uploadRequest(t, token, "logo.png", "fake-png-bytes"))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusSeeOther, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/assets" {
		t.Errorf("Location = %q, want %q", loc, "/assets")
	}

	got, err := svc.Assets.Get(context.Background(), "logo.png")
	if err != nil {
		t.Fatalf("Assets.Get() error = %v, want nil", err)
	}
	if string(got) != "fake-png-bytes" {
		t.Errorf("Assets.Get() = %q, want %q", got, "fake-png-bytes")
	}
}

// TestAssetsHandler_Upload_ExistingName_Overwrites proves uploading a
// duplicate name replaces the existing asset rather than erroring or
// creating a second entry — assets.Store.Put's existing overwrite
// contract, exercised end-to-end through the Web UI.
func TestAssetsHandler_Upload_ExistingName_Overwrites(t *testing.T) {
	svc := newAssetsTestService()
	if err := svc.Assets.Put(context.Background(), "logo.png", []byte("first")); err != nil {
		t.Fatalf("Assets.Put: %v", err)
	}

	router := webui.NewRouter(svc)
	token := csrfTokenFromPage(t, router, "/assets")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, uploadRequest(t, token, "logo.png", "second"))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusSeeOther, rec.Body)
	}

	got, err := svc.Assets.Get(context.Background(), "logo.png")
	if err != nil {
		t.Fatalf("Assets.Get() error = %v, want nil", err)
	}
	if string(got) != "second" {
		t.Errorf("Assets.Get() = %q, want the upload to have replaced the existing asset", got)
	}

	names, err := svc.Assets.List(context.Background())
	if err != nil {
		t.Fatalf("Assets.List() error = %v, want nil", err)
	}
	if len(names) != 1 {
		t.Errorf("Assets.List() = %v, want exactly one entry (overwritten, not duplicated)", names)
	}
}

// TestAssetsHandler_Upload_MissingName_RendersValidationError proves an
// upload with no asset name fails validation (assets.Store's own
// validateName) with a generic 4xx error rather than storing an
// unnamed asset.
func TestAssetsHandler_Upload_MissingName_RendersValidationError(t *testing.T) {
	svc := newAssetsTestService()

	router := webui.NewRouter(svc)
	token := csrfTokenFromPage(t, router, "/assets")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, uploadRequest(t, token, "", "fake-png-bytes"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}

	names, err := svc.Assets.List(context.Background())
	if err != nil {
		t.Fatalf("Assets.List() error = %v, want nil", err)
	}
	if len(names) != 0 {
		t.Errorf("Assets.List() = %v, want nothing stored after a rejected upload", names)
	}
}

// TestAssetsHandler_Upload_MissingFile_RendersValidationError proves an
// upload with a name but no file part at all is rejected rather than
// stored as an empty asset.
func TestAssetsHandler_Upload_MissingFile_RendersValidationError(t *testing.T) {
	svc := newAssetsTestService()

	router := webui.NewRouter(svc)
	token := csrfTokenFromPage(t, router, "/assets")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, uploadRequestNoFile(t, token, "logo.png"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}

	names, err := svc.Assets.List(context.Background())
	if err != nil {
		t.Fatalf("Assets.List() error = %v, want nil", err)
	}
	if len(names) != 0 {
		t.Errorf("Assets.List() = %v, want nothing stored after a rejected upload", names)
	}
}

// TestAssetsHandler_Upload_MalformedMultipartBody_RendersValidationError
// proves a request claiming multipart/form-data whose body isn't valid
// multipart (ParseMultipartForm's own decode failure) is rejected the
// same way as any other bad upload, not a 500.
func TestAssetsHandler_Upload_MalformedMultipartBody_RendersValidationError(t *testing.T) {
	svc := newAssetsTestService()

	req := httptest.NewRequest(http.MethodPost, "/assets", strings.NewReader("not multipart"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=missing")

	router := webui.NewRouter(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
}

// TestAssetsHandler_Delete_RemovesAssetThenRedirectsToList proves
// deletion reaches Service.DeleteAsset and redirects back to GET
// /assets.
func TestAssetsHandler_Delete_RemovesAssetThenRedirectsToList(t *testing.T) {
	svc := newAssetsTestService()
	if err := svc.Assets.Put(context.Background(), "logo.png", []byte("data")); err != nil {
		t.Fatalf("Assets.Put: %v", err)
	}

	router := webui.NewRouter(svc)
	token := csrfTokenFromPage(t, router, "/assets")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, deleteAssetRequest(token, "logo.png"))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusSeeOther, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/assets" {
		t.Errorf("Location = %q, want %q", loc, "/assets")
	}

	if _, err := svc.Assets.Get(context.Background(), "logo.png"); err == nil {
		t.Error("Assets.Get() after delete: want an error, got nil")
	}
}

// TestAssetsHandler_Delete_MissingName_RendersGenericNotFoundError proves
// deleting a name that isn't stored produces a generic not-found error
// rather than a silent success or a leaked internal detail.
func TestAssetsHandler_Delete_MissingName_RendersGenericNotFoundError(t *testing.T) {
	svc := newAssetsTestService()

	router := webui.NewRouter(svc)
	token := csrfTokenFromPage(t, router, "/assets")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, deleteAssetRequest(token, "does-not-exist.png"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body)
	}
	if body := rec.Body.String(); strings.Contains(body, "does-not-exist.png") {
		t.Errorf("body leaks the underlying error detail: %s", body)
	}
}

// contentRequest builds a GET /assets/{name}/content request.
func contentRequest(name string) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/assets/"+name+"/content", nil)
}

func serveContent(t *testing.T, svc *app.Service, name string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	webui.NewRouter(svc).ServeHTTP(rec, contentRequest(name))
	return rec
}

// TestAssetsHandler_Content_ImageServesInline proves an asset whose
// extension and bytes agree is served with its real image type and no
// attachment disposition, so the Assets page can render it in an <img>.
func TestAssetsHandler_Content_ImageServesInline(t *testing.T) {
	svc := newAssetsTestService()
	png := []byte("\x89PNG\r\n\x1a\n")
	if err := svc.Assets.Put(context.Background(), "logo.png", png); err != nil {
		t.Fatalf("Assets.Put: %v", err)
	}

	rec := serveContent(t, svc, "logo.png")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q, want %q", got, "image/png")
	}
	// inline, not attachment — but still naming the asset, since the URL
	// ends in /content and a save-as would otherwise offer that.
	if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "inline") {
		t.Errorf("Content-Disposition = %q, want an inline disposition", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "logo.png") {
		t.Errorf("Content-Disposition = %q, want it to name the asset", got)
	}
	if got := rec.Body.Bytes(); string(got) != string(png) {
		t.Errorf("body = %q, want the stored bytes %q", got, png)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q (Put overwrites in place)", got, "no-store")
	}
}

// TestAssetsHandler_Content_HTMLNamedAsPNGDownloads is the security case
// this endpoint exists to get right: a caller controls both the asset
// name and its bytes, so an HTML payload under an image extension must
// never be served inline, where it would execute in the operator's
// authenticated origin.
func TestAssetsHandler_Content_HTMLNamedAsPNGDownloads(t *testing.T) {
	svc := newAssetsTestService()
	payload := []byte(`<!DOCTYPE html><html><body><script>alert(1)</script></body></html>`)
	if err := svc.Assets.Put(context.Background(), "evil.png", payload); err != nil {
		t.Fatalf("Assets.Put: %v", err)
	}

	rec := serveContent(t, svc, "evil.png")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want %q — HTML must never be served under an image type", got, "application/octet-stream")
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment") {
		t.Errorf("Content-Disposition = %q, want an attachment disposition", got)
	}
}

// TestAssetsHandler_Content_SVGDownloads proves SVG is excluded from the
// inline allowlist despite being an image: it executes script when a
// browser renders it (docs/adr/0029).
func TestAssetsHandler_Content_SVGDownloads(t *testing.T) {
	svc := newAssetsTestService()
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	if err := svc.Assets.Put(context.Background(), "logo.svg", svg); err != nil {
		t.Fatalf("Assets.Put: %v", err)
	}

	rec := serveContent(t, svc, "logo.svg")

	if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want %q", got, "application/octet-stream")
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment") {
		t.Errorf("Content-Disposition = %q, want an attachment disposition", got)
	}
}

func TestAssetsHandler_Content_MissingAsset_ReturnsNotFound(t *testing.T) {
	rec := serveContent(t, newAssetsTestService(), "does-not-exist.png")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if body := rec.Body.String(); strings.Contains(body, "does-not-exist.png") {
		t.Errorf("body leaks the underlying error detail: %s", body)
	}
}

// TestAssetsHandler_List_RendersThumbnailForImageAndLinkOtherwise proves
// the listing distinguishes the two by extension, and shows the size and
// modified columns the Store now reports.
func TestAssetsHandler_List_RendersThumbnailForImageAndLinkOtherwise(t *testing.T) {
	svc := newAssetsTestService()
	ctx := context.Background()
	if err := svc.Assets.Put(ctx, "logo.png", []byte("\x89PNG\r\n\x1a\n")); err != nil {
		t.Fatalf("Assets.Put: %v", err)
	}
	if err := svc.Assets.Put(ctx, "notes.txt", []byte("hello")); err != nil {
		t.Fatalf("Assets.Put: %v", err)
	}

	rec := httptest.NewRecorder()
	webui.NewRouter(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<img src="/assets/logo.png/content"`) {
		t.Errorf("body has no thumbnail for the image asset: %s", body)
	}
	if strings.Contains(body, `<img src="/assets/notes.txt/content"`) {
		t.Errorf("body renders a thumbnail for a non-image asset: %s", body)
	}
	if !strings.Contains(body, "/assets/notes.txt/content") {
		t.Errorf("body has no download link for the non-image asset: %s", body)
	}
	if !strings.Contains(body, "5 B") {
		t.Errorf("body does not show the non-image asset's size: %s", body)
	}
}
