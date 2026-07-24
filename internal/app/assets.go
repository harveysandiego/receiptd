package app

import (
	"context"
	"sort"
)

// ListAssets returns an AssetSummary for every stored asset, sorted by
// Name for a deterministic result regardless of what order the
// underlying assets.Store happens to return (both implementations
// already sort, but ListAssets doesn't rely on that — it sorts its own
// result independently of the Store's guarantees).
func (s *Service) ListAssets(ctx context.Context) ([]AssetSummary, error) {
	names, err := s.Assets.List(ctx)
	if err != nil {
		return nil, err
	}

	summaries := make([]AssetSummary, 0, len(names))
	for _, name := range names {
		summaries = append(summaries, AssetSummary{Name: name})
	}

	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Name < summaries[j].Name })
	return summaries, nil
}

// UploadAsset stores data under name, via the Web UI's upload form
// (docs/adr/0026-asset-upload-multipart-form-data.md). It's a thin pass
// through to assets.Store.Put, whose own validateName rejects an empty
// or path-escaping name; Put also silently overwrites an existing name
// rather than erroring, so re-uploading an existing name replaces it.
func (s *Service) UploadAsset(ctx context.Context, name string, data []byte) error {
	return s.Assets.Put(ctx, name, data)
}

// DeleteAsset removes the named asset, via the Web UI's delete action. It
// is a thin pass through to assets.Store.Delete, which returns
// apperr.KindNotFound for a name that isn't currently stored.
func (s *Service) DeleteAsset(ctx context.Context, name string) error {
	return s.Assets.Delete(ctx, name)
}
