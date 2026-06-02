package agent

import (
	"context"
	"fmt"

	"github.com/Hochfrequenz/adtler/adt"
)

// ADTClient is the subset of adt.Client the agent tools need.
type ADTClient interface {
	GetTransportObjects(ctx context.Context, transportNumber string) ([]adt.TransportObject, error)
	GetSource(ctx context.Context, objectURI string) (*adt.SourceResult, error)
	GetIncludeSource(ctx context.Context, objectURI, include string) (*adt.SourceResult, error)
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
	Type  string `json:"type"`
	Name  string `json:"name"`
	URI   string `json:"uri"` // empty for unsupported types; see ObjectURI
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

// FetchSource returns the main source code for any PROG/CLAS/INTF object URI.
func (t *Tools) FetchSource(ctx context.Context, objectURI string) (string, error) {
	res, err := t.client.GetSource(ctx, objectURI)
	if err != nil {
		return "", fmt.Errorf("fetch source %q: %w", objectURI, err)
	}
	return res.Source, nil
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
		out[inc] = res.Source
	}
	return out, nil
}
