package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
)

// ADTClient is the subset of adt.Client the agent tools need.
// Implementing this interface is also the swap point for replacing adtler with
// a different SAP ADT client if needed.
type ADTClient interface {
	// Source reading
	GetTransportObjects(ctx context.Context, transportNumber string) ([]adt.TransportObject, error)
	GetSource(ctx context.Context, objectURI string) (*adt.SourceResult, error)
	GetIncludeSource(ctx context.Context, objectURI, include string) (*adt.SourceResult, error)
	// Quality & analysis
	SyntaxCheck(ctx context.Context, objectURI string) ([]adt.SyntaxMessage, error)
	RunATCCheck(ctx context.Context, objectURIs []string, checkVariant string) (*adt.ATCResult, error)
	// Navigation & metadata
	GetObjectInfo(ctx context.Context, objectURI string) (*adt.ObjectInfo, error)
	GetVersionHistory(ctx context.Context, objectURI string) ([]adt.VersionInfo, error)
	WhereUsed(ctx context.Context, objectURI string) ([]adt.ObjectInfo, error)
	DiffActiveInactive(ctx context.Context, objectURI string) (*adt.DiffResult, error)
	// RunQuery executes an ADT SQL SELECT and returns the result rows.
	// Used for preflight checks that the ADT transport-objects endpoint cannot satisfy
	// (e.g. SYST/CUST transport types that are invisible to GetTransportObjects).
	RunQuery(ctx context.Context, query string, maxRows int) (*adt.QueryResult, error)
}

// TRObject is the agent-facing view of a transport request object.
// URI is pre-computed so Claude doesn't need to know ADT path conventions.
type TRObject struct {
	// PgmID is the SAP Program ID — a CTS classification that groups object types
	// (e.g. "R3TR" for repository objects, "LIMU" for sub-objects like includes).
	// Sourced directly from adtler's TransportObject.PgmID.
	PgmID string `json:"pgmid"`
	// Type is the SAP object type code — e.g. "PROG" (program), "CLAS" (class),
	// "INTF" (interface), "FUGR" (function group). See ObjectURI for supported types.
	Type string `json:"type"`
	Name string `json:"name"`
	URI  string `json:"uri"` // empty for unsupported types; see ObjectURI
}

// Tools holds the ADT client and exposes the three agent tools as methods.
type Tools struct {
	client ADTClient
}

// NewTools creates a Tools instance backed by the given ADTClient.
func NewTools(client ADTClient) *Tools {
	return &Tools{client: client}
}

// ListTRObjects fetches all objects in a transport request and annotates each
// with its ADT URI (empty for unsupported object types like FUGR).
func (t *Tools) ListTRObjects(ctx context.Context, trID string) ([]TRObject, error) {
	raw, err := t.client.GetTransportObjects(ctx, trID)
	if err != nil {
		return nil, fmt.Errorf("list TR objects %q: %w", trID, err)
	}
	out := make([]TRObject, len(raw))
	for i, obj := range raw {
		out[i] = TRObject{
			PgmID: obj.PgmID,
			Type:  obj.Type,
			Name:  obj.Name,
			URI:   ObjectURI(obj),
		}
	}
	return out, nil
}

// FetchSource returns the main source code for any PROG/CLAS/INTF object URI,
// annotated with 1-based line numbers (see annotateLineNumbers).
func (t *Tools) FetchSource(ctx context.Context, objectURI string) (string, error) {
	res, err := t.client.GetSource(ctx, objectURI)
	if err != nil {
		return "", fmt.Errorf("fetch source %q: %w", objectURI, err)
	}
	return annotateLineNumbers(res.Source), nil
}

// annotateLineNumbers prefixes each line of src with its 1-based line number,
// e.g. "12 | DATA lv_x TYPE i.". The numbers are counted from the fetched
// source, which starts at line 1, so they line up with the SE38/ADT editor and
// with the line numbers reported by run_atc_check and syntax_check. This grounds
// the review: every finding can cite a concrete line. Without these anchors the
// model is free to describe code that was never fetched — see issue #42, where a
// FORM-based report was reviewed against a hallucinated class structure.
func annotateLineNumbers(src string) string {
	// ADT may return CRLF (or lone CR) line endings; normalise to "\n" first so
	// the line count matches the SE38/ADT editor and no stray carriage return
	// hangs off the end of an annotated line.
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	lines := strings.Split(src, "\n")
	// A trailing newline makes Split emit a final empty element
	// (strings.Split("A\n", "\n") == ["A", ""]). Drop it so we don't number a
	// phantom line the editor never shows — which would also break the claimed
	// alignment with run_atc_check / syntax_check line numbers.
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, line := range lines {
		lines[i] = fmt.Sprintf("%d | %s", i+1, line)
	}
	return strings.Join(lines, "\n")
}

// SyntaxCheck runs an ADT syntax check on the saved object at objectURI.
// Returns syntax messages (errors, warnings, info). An empty slice means no issues.
func (t *Tools) SyntaxCheck(ctx context.Context, objectURI string) ([]adt.SyntaxMessage, error) {
	msgs, err := t.client.SyntaxCheck(ctx, objectURI)
	if err != nil {
		return nil, fmt.Errorf("syntax check %q: %w", objectURI, err)
	}
	return msgs, nil
}

// GetObjectInfo returns metadata for an ABAP object: type, name, description, package.
func (t *Tools) GetObjectInfo(ctx context.Context, objectURI string) (*adt.ObjectInfo, error) {
	info, err := t.client.GetObjectInfo(ctx, objectURI)
	if err != nil {
		return nil, fmt.Errorf("get object info %q: %w", objectURI, err)
	}
	return info, nil
}

// GetVersionHistory returns the version history of an ABAP object (author, date, transport per version).
func (t *Tools) GetVersionHistory(ctx context.Context, objectURI string) ([]adt.VersionInfo, error) {
	hist, err := t.client.GetVersionHistory(ctx, objectURI)
	if err != nil {
		return nil, fmt.Errorf("get version history %q: %w", objectURI, err)
	}
	return hist, nil
}

// WhereUsed returns objects that reference the given ABAP object (callers, users).
func (t *Tools) WhereUsed(ctx context.Context, objectURI string) ([]adt.ObjectInfo, error) {
	callers, err := t.client.WhereUsed(ctx, objectURI)
	if err != nil {
		return nil, fmt.Errorf("where used %q: %w", objectURI, err)
	}
	return callers, nil
}

// DiffActiveInactive returns the diff between the active (released) and inactive (pending) version.
// HasChanges=false means the object has no pending edits.
func (t *Tools) DiffActiveInactive(ctx context.Context, objectURI string) (*adt.DiffResult, error) {
	diff, err := t.client.DiffActiveInactive(ctx, objectURI)
	if err != nil {
		return nil, fmt.Errorf("diff active/inactive %q: %w", objectURI, err)
	}
	return diff, nil
}

// RunATCCheck runs the ATC (ABAP Test Cockpit) static analysis on the given object URIs.
// checkVariant is the ATC check variant name; pass "" to use the system default.
// Returns findings with priority, check name, and message for each issue.
func (t *Tools) RunATCCheck(ctx context.Context, objectURIs []string, checkVariant string) (*adt.ATCResult, error) {
	result, err := t.client.RunATCCheck(ctx, objectURIs, checkVariant)
	if err != nil {
		return nil, fmt.Errorf("run ATC check: %w", err)
	}
	return result, nil
}

// RunQuery executes an ADT SQL SELECT and returns the result.
// Used by Preflight to check E071 when the transport-objects endpoint returns nothing.
func (t *Tools) RunQuery(ctx context.Context, query string, maxRows int) (*adt.QueryResult, error) {
	res, err := t.client.RunQuery(ctx, query, maxRows)
	if err != nil {
		return nil, fmt.Errorf("run query: %w", err)
	}
	return res, nil
}

// FetchClassIncludes returns a map of include name → source for a CLAS URI.
// Missing includes (e.g. testclasses not yet created) are silently omitted.
func (t *Tools) FetchClassIncludes(ctx context.Context, classURI string) (map[string]string, error) {
	includes := []string{"definitions", "implementations", "testclasses", "macros"}
	out := make(map[string]string)
	for _, inc := range includes {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		res, err := t.client.GetIncludeSource(ctx, classURI, inc)
		if err != nil {
			continue // absent include on this SAP system — not an error
		}
		out[inc] = annotateLineNumbers(res.Source)
	}
	return out, nil
}
