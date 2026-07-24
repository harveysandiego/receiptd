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
