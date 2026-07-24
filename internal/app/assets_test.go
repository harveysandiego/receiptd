package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/assets"
)

// errAssetStore is an assets.Store test double whose List always
// returns listErr, letting TestService_ListAssets_StoreErrorPropagates
// observe error propagation without a real Store.
type errAssetStore struct {
	assets.Store
	listErr error
}

func (s *errAssetStore) List(_ context.Context) ([]string, error) {
	return nil, s.listErr
}

func TestService_ListAssets_EmptyStore_ReturnsEmptySlice(t *testing.T) {
	s := newTestService()
	s.Assets = assets.NewMemoryStore()

	summaries, err := s.ListAssets(context.Background())
	if err != nil {
		t.Fatalf("ListAssets() error = %v, want nil", err)
	}
	if len(summaries) != 0 {
		t.Errorf("len(ListAssets()) = %d, want 0", len(summaries))
	}
}

func TestService_ListAssets_ReturnsSummaryPerStoredAsset(t *testing.T) {
	ctx := context.Background()
	store := assets.NewMemoryStore()
	if err := store.Put(ctx, "logo.png", []byte("fake-png-bytes")); err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}

	s := newTestService()
	s.Assets = store

	summaries, err := s.ListAssets(ctx)
	if err != nil {
		t.Fatalf("ListAssets() error = %v, want nil", err)
	}
	want := []app.AssetSummary{{Name: "logo.png"}}
	if len(summaries) != len(want) || summaries[0] != want[0] {
		t.Errorf("ListAssets() = %+v, want %+v", summaries, want)
	}
}

func TestService_ListAssets_SortedByName(t *testing.T) {
	ctx := context.Background()
	store := assets.NewMemoryStore()
	for _, name := range []string{"zebra.png", "banner.png", "kitchen.png"} {
		if err := store.Put(ctx, name, []byte("data")); err != nil {
			t.Fatalf("Put(%q) error = %v, want nil", name, err)
		}
	}

	s := newTestService()
	s.Assets = store

	summaries, err := s.ListAssets(ctx)
	if err != nil {
		t.Fatalf("ListAssets() error = %v, want nil", err)
	}
	want := []string{"banner.png", "kitchen.png", "zebra.png"}
	if len(summaries) != len(want) {
		t.Fatalf("len(ListAssets()) = %d, want %d", len(summaries), len(want))
	}
	for i, s := range summaries {
		if s.Name != want[i] {
			t.Errorf("ListAssets()[%d].Name = %q, want %q (sorted by name)", i, s.Name, want[i])
		}
	}
}

func TestService_ListAssets_StoreErrorPropagates(t *testing.T) {
	wantErr := apperr.Wrap(apperr.KindPermanent, "assets.Store.List", errors.New("disk error"))
	s := newTestService()
	s.Assets = &errAssetStore{listErr: wantErr}

	_, err := s.ListAssets(context.Background())
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("ListAssets() error = %v, want apperr.KindPermanent", err)
	}
}

func TestService_UploadAsset_StoresRetrievableBytes(t *testing.T) {
	ctx := context.Background()
	s := newTestService()
	s.Assets = assets.NewMemoryStore()

	if err := s.UploadAsset(ctx, "logo.png", []byte("fake-png-bytes")); err != nil {
		t.Fatalf("UploadAsset() error = %v, want nil", err)
	}

	got, err := s.Assets.Get(ctx, "logo.png")
	if err != nil {
		t.Fatalf("Assets.Get() error = %v, want nil", err)
	}
	if string(got) != "fake-png-bytes" {
		t.Errorf("Assets.Get() = %q, want %q", got, "fake-png-bytes")
	}
}

func TestService_UploadAsset_ExistingName_Overwrites(t *testing.T) {
	ctx := context.Background()
	s := newTestService()
	s.Assets = assets.NewMemoryStore()

	if err := s.UploadAsset(ctx, "logo.png", []byte("first")); err != nil {
		t.Fatalf("UploadAsset() error = %v, want nil", err)
	}
	if err := s.UploadAsset(ctx, "logo.png", []byte("second")); err != nil {
		t.Fatalf("UploadAsset() error = %v, want nil", err)
	}

	got, err := s.Assets.Get(ctx, "logo.png")
	if err != nil {
		t.Fatalf("Assets.Get() error = %v, want nil", err)
	}
	if string(got) != "second" {
		t.Errorf("Assets.Get() = %q, want the second upload to have replaced the first", got)
	}
}

func TestService_UploadAsset_InvalidName_ReturnsValidationError(t *testing.T) {
	s := newTestService()
	s.Assets = assets.NewMemoryStore()

	err := s.UploadAsset(context.Background(), "", []byte("data"))
	if !apperr.Is(err, apperr.KindValidation) {
		t.Fatalf("UploadAsset(\"\") error = %v, want apperr.KindValidation", err)
	}
}

func TestService_DeleteAsset_RemovesStoredAsset(t *testing.T) {
	ctx := context.Background()
	s := newTestService()
	s.Assets = assets.NewMemoryStore()
	if err := s.Assets.Put(ctx, "logo.png", []byte("data")); err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}

	if err := s.DeleteAsset(ctx, "logo.png"); err != nil {
		t.Fatalf("DeleteAsset() error = %v, want nil", err)
	}

	if _, err := s.Assets.Get(ctx, "logo.png"); !apperr.Is(err, apperr.KindNotFound) {
		t.Errorf("Assets.Get() after delete: error = %v, want apperr.KindNotFound", err)
	}
}

func TestService_DeleteAsset_MissingName_ReturnsNotFound(t *testing.T) {
	s := newTestService()
	s.Assets = assets.NewMemoryStore()

	err := s.DeleteAsset(context.Background(), "does-not-exist.png")
	if !apperr.Is(err, apperr.KindNotFound) {
		t.Fatalf("DeleteAsset() error = %v, want apperr.KindNotFound", err)
	}
}
