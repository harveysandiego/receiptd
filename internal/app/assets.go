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
