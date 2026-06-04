package adtclient

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/Hochfrequenz/adtler/adt"
)

// SQLTransportLister fetches open transport requests via ADT SQL (RunQuery on E070/E07T).
// This bypasses the ADT transport organizer tree endpoint which only handles
// standard workbench (KORRDEV="K") requests and misses SYST/CUST types used in S/4HANA.
type SQLTransportLister struct {
	client adt.Client
	lang   string // ABAP language key for E07T descriptions, e.g. "D" or "E"
}

// NewSQLTransportLister creates a lister that queries E070/E07T directly.
// lang is the ABAP language key for TR descriptions (e.g. "D" for German, "E" for English).
func NewSQLTransportLister(client adt.Client, lang string) *SQLTransportLister {
	if lang == "" {
		lang = "D"
	}
	return &SQLTransportLister{client: client, lang: lang}
}

// GetTransportRequests queries E070 for open TRs, then enriches with E07T descriptions.
func (s *SQLTransportLister) GetTransportRequests(ctx context.Context, user, status string) ([]adt.TransportRequest, error) {
	// Build E070 query — modifiable TRs, excluding SAP-owned standard packages.
	// SAP-owned TRs (AS4USER='SAP') are standard package transports, not customer
	// development — they clog the list and are never useful for code review.
	// STRKORR = '' selects only top-level Transportaufträge (requests), not
	// Transportaufgaben (tasks). Tasks have STRKORR pointing at their parent request.
	// status="" means all statuses; "D"=open only; "L"=locked only; "R"=released only.
	// Whitelist guards against SQL injection through the public interface.
	if status != "" && status != "D" && status != "L" && status != "R" {
		return nil, fmt.Errorf("invalid transport status %q", status)
	}
	where := "AS4USER <> 'SAP' AND STRKORR = ''"
	if status != "" {
		where = fmt.Sprintf("TRSTATUS = '%s' AND AS4USER <> 'SAP' AND STRKORR = ''", status)
	}
	if user != "" {
		where += fmt.Sprintf(" AND AS4USER = '%s'", user)
	}
	// ADT SQL doesn't support DESC; we sort client-side after fetching.
	q070 := fmt.Sprintf("SELECT TRKORR, AS4USER, TRSTATUS, AS4DATE FROM E070 WHERE %s", where)

	res070, err := s.client.RunQuery(ctx, q070, 2000)
	if err != nil {
		slog.InfoContext(ctx, "sqllister E070 error", "err", err)
		return nil, fmt.Errorf("query E070: %w", err)
	}
	if len(res070.Rows) == 0 {
		return nil, nil
	}

	// Collect TR numbers; row = [TRKORR, AS4USER, TRSTATUS, AS4DATE].
	type trWithDate struct {
		tr   *adt.TransportRequest
		date string
	}
	entries := make([]trWithDate, 0, len(res070.Rows))
	trMap := make(map[string]*adt.TransportRequest, len(res070.Rows))
	for _, row := range res070.Rows {
		if len(row) < 4 {
			continue
		}
		tr := &adt.TransportRequest{
			Number: row[0],
			Owner:  row[1],
			Status: row[2],
		}
		entries = append(entries, trWithDate{tr: tr, date: row[3]})
		trMap[row[0]] = tr
	}
	// Sort descending by date (string comparison works for YYYYMMDD format).
	sort.Slice(entries, func(i, j int) bool { return entries[i].date > entries[j].date })
	numbers := make([]string, 0, len(entries))
	for _, e := range entries {
		numbers = append(numbers, e.tr.Number)
	}

	// Fetch descriptions from E07T for the language we want.
	// ADT SQL doesn't support IN clauses, so we fetch recent entries and filter client-side.
	q07t := fmt.Sprintf("SELECT TRKORR, AS4TEXT FROM E07T WHERE LANGU = '%s'", s.lang)
	res07t, err := s.client.RunQuery(ctx, q07t, 5000)
	if err != nil {
		slog.InfoContext(ctx, "sqllister E07T error — descriptions unavailable", "err", err)
	} else {
		for _, row := range res07t.Rows {
			if len(row) < 2 {
				continue
			}
			if tr, ok := trMap[row[0]]; ok {
				tr.Description = row[1]
			}
		}
	}

	// Return in E070 order (already DESC by date).
	result := make([]adt.TransportRequest, 0, len(numbers))
	for _, n := range numbers {
		if tr, ok := trMap[n]; ok {
			result = append(result, *tr)
		}
	}
	slog.InfoContext(ctx, "sqllister result", "count", len(result))
	return result, nil
}
