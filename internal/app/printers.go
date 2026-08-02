package app

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
)

// printerStatusTimeout bounds a single printer's Status check, well under
// cmd/receiptd's 30s HTTP WriteTimeout: every configured printer is
// checked concurrently (below), so total wall time for ListPrinters is
// one timeout period at worst, not len(Printers) of them serially. 5s is
// generous for a LAN dial/response and still leaves most of the 30s
// window for the rest of the request (render + write).
const printerStatusTimeout = 5 * time.Second

// ListPrinters returns a PrinterSummary for every configured printer,
// sorted by Name for a deterministic result regardless of Go's undefined
// map iteration order (and regardless of the concurrent Status checks
// below completing in a different order).
//
// Printers is treated as the authoritative key set: buildPrinters
// (cmd/receiptd) populates Printers, Profiles, and Connections together,
// in one loop, from the same []config.PrinterConfig, so a name present in
// Printers but missing from Profiles or Connections can only mean a
// construction bug, not a valid partial printer — ListPrinters reports
// that as apperr.KindPermanent rather than silently rendering an
// incomplete row. This check happens up front, before any Status call, so
// a construction bug is reported immediately rather than after waiting on
// a network round trip.
//
// Each printer's Status is checked concurrently, one goroutine per
// printer, each bounded by printerStatusTimeout — a single slow or
// black-holed printer only delays its own result, never the others', and
// can never make the whole call take longer than printerStatusTimeout
// regardless of how many printers are configured. A printer that fails
// its Status check (offline, a transport error, or a timeout) is still
// included, with Online false — one unreachable printer must not fail
// the whole listing, since ListPrinters backs a page showing every
// configured printer at once.
func (s *Service) ListPrinters(ctx context.Context) ([]PrinterSummary, error) {
	type entry struct {
		name    string
		printer printer.Printer
		profile printer.Profile
		conn    ConnectionSummary
	}

	entries := make([]entry, 0, len(s.Printers))
	for name, p := range s.Printers {
		profile, ok := s.Profiles[name]
		if !ok {
			return nil, apperr.Wrap(apperr.KindPermanent, "app.ListPrinters", fmt.Errorf("printer %q has no configured Profile", name))
		}
		conn, ok := s.Connections[name]
		if !ok {
			return nil, apperr.Wrap(apperr.KindPermanent, "app.ListPrinters", fmt.Errorf("printer %q has no configured ConnectionSummary", name))
		}
		entries = append(entries, entry{name: name, printer: p, profile: profile, conn: conn})
	}

	summaries := make([]PrinterSummary, len(entries))
	var wg sync.WaitGroup
	wg.Add(len(entries))
	for i, e := range entries {
		go func(i int, e entry) {
			defer wg.Done()
			summaries[i] = printerSummaryWithStatus(ctx, e.name, e.printer, e.profile, e.conn)
		}(i, e)
	}
	wg.Wait()

	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Name < summaries[j].Name })
	return summaries, nil
}

// PrinterNames returns every configured printer's name, sorted. Unlike
// ListPrinters it does no I/O (no Status probe), so it's cheap enough to
// call on every page render.
func (s *Service) PrinterNames() []string {
	names := make([]string, 0, len(s.Printers))
	for name := range s.Printers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// printerSummaryWithStatus checks one printer's Status, bounded by its own
// derived context (the earlier of ctx's deadline and printerStatusTimeout
// — see context.WithTimeout), and builds its PrinterSummary. A timeout or
// any other Status error is reported as Online: false with the error's
// text as Detail, exactly like a plain offline report; the caller doesn't
// need to distinguish "dial failed" from "took too long to answer".
func printerSummaryWithStatus(ctx context.Context, name string, p printer.Printer, profile printer.Profile, conn ConnectionSummary) PrinterSummary {
	statusCtx, cancel := context.WithTimeout(ctx, printerStatusTimeout)
	defer cancel()

	status, err := p.Status(statusCtx)
	if err != nil {
		status.Online = false
		status.Detail = err.Error()
	}

	return PrinterSummary{
		Name:               name,
		Transport:          conn.Transport,
		Address:            conn.Address,
		WidthDots:          profile.WidthDots,
		DPI:                profile.DPI,
		SupportsCut:        profile.SupportsCut,
		SupportsPartialCut: profile.SupportsPartialCut,
		Online:             status.Online,
		StatusDetail:       status.Detail,
	}
}
