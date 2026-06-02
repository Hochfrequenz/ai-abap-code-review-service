package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/agent"
)

// fakeADTClient implements the subset of adt.Client the tools need.
type fakeADTClient struct {
	trObjects []adt.TransportObject
	sources   map[string]string
	trErr     error
	srcErr    error
}

func (f *fakeADTClient) GetTransportObjects(_ context.Context, _ string) ([]adt.TransportObject, error) {
	return f.trObjects, f.trErr
}
func (f *fakeADTClient) GetSource(_ context.Context, uri string) (*adt.SourceResult, error) {
	if f.srcErr != nil {
		return nil, f.srcErr
	}
	src, ok := f.sources[uri]
	if !ok {
		return nil, errors.New("not found")
	}
	return &adt.SourceResult{Source: src}, nil
}
func (f *fakeADTClient) GetIncludeSource(_ context.Context, uri, include string) (*adt.SourceResult, error) {
	key := uri + "/" + include
	src, ok := f.sources[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return &adt.SourceResult{Source: src}, nil
}

func TestListTRObjects_ReturnsObjectsWithURIs(t *testing.T) {
	fake := &fakeADTClient{
		trObjects: []adt.TransportObject{
			{Type: "CLAS", Name: "ZCL_FOO"},
			{Type: "FUGR", Name: "ZFUGR"}, // unsupported — URI will be empty
		},
	}
	tools := agent.NewTools(fake)
	result, err := tools.ListTRObjects(context.Background(), "NPLK900014")
	if err != nil {
		t.Fatalf("ListTRObjects: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(result))
	}
	if result[0].URI != "/sap/bc/adt/oo/classes/zcl_foo" {
		t.Errorf("URI: got %q", result[0].URI)
	}
	if result[1].URI != "" {
		t.Errorf("FUGR should have empty URI, got %q", result[1].URI)
	}
}

func TestFetchSource_ReturnsSource(t *testing.T) {
	fake := &fakeADTClient{
		sources: map[string]string{
			"/sap/bc/adt/oo/classes/zcl_foo": "CLASS zcl_foo DEFINITION.",
		},
	}
	tools := agent.NewTools(fake)
	src, err := tools.FetchSource(context.Background(), "/sap/bc/adt/oo/classes/zcl_foo")
	if err != nil {
		t.Fatalf("FetchSource: %v", err)
	}
	if src != "CLASS zcl_foo DEFINITION." {
		t.Errorf("source: got %q", src)
	}
}

func TestFetchClassIncludes_CancelledContext_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	fake := &fakeADTClient{sources: map[string]string{}}
	tools := agent.NewTools(fake)
	_, err := tools.FetchClassIncludes(ctx, "/sap/bc/adt/oo/classes/zcl_foo")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestFetchClassIncludes_ReturnsAvailableIncludes(t *testing.T) {
	fake := &fakeADTClient{
		sources: map[string]string{
			"/sap/bc/adt/oo/classes/zcl_foo/definitions":     "DEFINITION content",
			"/sap/bc/adt/oo/classes/zcl_foo/implementations": "IMPLEMENTATION content",
			// testclasses and macros absent — tools should tolerate missing includes
		},
	}
	tools := agent.NewTools(fake)
	result, err := tools.FetchClassIncludes(context.Background(), "/sap/bc/adt/oo/classes/zcl_foo")
	if err != nil {
		t.Fatalf("FetchClassIncludes: %v", err)
	}
	if result["definitions"] != "DEFINITION content" {
		t.Errorf("definitions: got %q", result["definitions"])
	}
	if result["implementations"] != "IMPLEMENTATION content" {
		t.Errorf("implementations: got %q", result["implementations"])
	}
	if _, ok := result["testclasses"]; ok {
		t.Error("testclasses should be absent")
	}
}
