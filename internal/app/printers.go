package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// ListPrinters returns a PrinterSummary for every configured printer,
// sorted by Name for a deterministic result regardless of Go's undefined
// map iteration order.
//
// Printers is treated as the authoritative key set: buildPrinters
// (cmd/receiptd) populates Printers, Profiles, and Connections together,
// in one loop, from the same []config.PrinterConfig, so a name present in
// Printers but missing from Profiles or Connections can only mean a
// construction bug, not a valid partial printer — ListPrinters reports
// that as apperr.KindPermanent rather than silently rendering an
// incomplete row.
//
// A printer that fails its Status check (offline, or a transport error)
// is still included, with Online false — one unreachable printer must
// not fail the whole listing, since ListPrinters backs a page showing
// every configured printer at once.
func (s *Service) ListPrinters(ctx context.Context) ([]PrinterSummary, error) {
	summaries := make([]PrinterSummary, 0, len(s.Printers))

	for name, p := range s.Printers {
		profile, ok := s.Profiles[name]
		if !ok {
			return nil, apperr.Wrap(apperr.KindPermanent, "app.ListPrinters", fmt.Errorf("printer %q has no configured Profile", name))
		}
		conn, ok := s.Connections[name]
		if !ok {
			return nil, apperr.Wrap(apperr.KindPermanent, "app.ListPrinters", fmt.Errorf("printer %q has no configured ConnectionSummary", name))
		}

		status, err := p.Status(ctx)
		if err != nil {
			status.Online = false
			status.Detail = err.Error()
		}

		summaries = append(summaries, PrinterSummary{
			Name:               name,
			Transport:          conn.Transport,
			Address:            conn.Address,
			WidthDots:          profile.WidthDots,
			DPI:                profile.DPI,
			SupportsCut:        profile.SupportsCut,
			SupportsPartialCut: profile.SupportsPartialCut,
			Online:             status.Online,
			StatusDetail:       status.Detail,
		})
	}

	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Name < summaries[j].Name })
	return summaries, nil
}
