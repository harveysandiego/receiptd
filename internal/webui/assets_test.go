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

func (errListAssetStore) List(_ context.Context) ([]string, error) {
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
