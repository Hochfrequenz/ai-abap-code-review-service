package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/hochfrequenz/ai-abap-code-review-service/internal/agent"
)

// fakeADTClient implements ADTClient for tests.
type fakeADTClient struct {
	trObjects      []adt.TransportObject
	sources        map[string]string
	trErr          error
	srcErr         error
	syntaxMessages []adt.SyntaxMessage
	syntaxErr      error
	objectInfo     *adt.ObjectInfo
	versionHistory []adt.VersionInfo
	whereUsed      []adt.ObjectInfo
	diffResult     *adt.DiffResult
	atcResult      *adt.ATCResult
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
func (f *fakeADTClient) SyntaxCheck(_ context.Context, _ string) ([]adt.SyntaxMessage, error) {
	return f.syntaxMessages, f.syntaxErr
}
func (f *fakeADTClient) GetObjectInfo(_ context.Context, _ string) (*adt.ObjectInfo, error) {
	if f.objectInfo == nil {
		return nil, errors.New("not found")
	}
	return f.objectInfo, nil
}
func (f *fakeADTClient) GetVersionHistory(_ context.Context, _ string) ([]adt.VersionInfo, error) {
	return f.versionHistory, nil
}
func (f *fakeADTClient) WhereUsed(_ context.Context, _ string) ([]adt.ObjectInfo, error) {
	return f.whereUsed, nil
}
func (f *fakeADTClient) DiffActiveInactive(_ context.Context, _ string) (*adt.DiffResult, error) {
	if f.diffResult == nil {
		return &adt.DiffResult{}, nil
	}
	return f.diffResult, nil
}
func (f *fakeADTClient) RunATCCheck(_ context.Context, _ []string, _ string) (*adt.ATCResult, error) {
	if f.atcResult == nil {
		return &adt.ATCResult{}, nil
	}
	return f.atcResult, nil
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

func TestSyntaxCheck_ReturnsMessages(t *testing.T) {
	fake := &fakeADTClient{
		syntaxMessages: []adt.SyntaxMessage{
			{Type: "E", Text: "Syntax error at line 5", Line: 5, Column: 3},
		},
	}
	tools := agent.NewTools(fake)
	msgs, err := tools.SyntaxCheck(context.Background(), "/sap/bc/adt/oo/classes/zcl_foo")
	if err != nil {
		t.Fatalf("SyntaxCheck: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Type != "E" || msgs[0].Line != 5 {
		t.Errorf("unexpected message: %+v", msgs[0])
	}
}

func TestGetObjectInfo_ReturnsMetadata(t *testing.T) {
	fake := &fakeADTClient{
		objectInfo: &adt.ObjectInfo{
			URI:         "/sap/bc/adt/oo/classes/zcl_foo",
			Type:        "CLAS",
			Name:        "ZCL_FOO",
			Description: "My class",
			PackageName: "ZMYPKG",
		},
	}
	tools := agent.NewTools(fake)
	info, err := tools.GetObjectInfo(context.Background(), "/sap/bc/adt/oo/classes/zcl_foo")
	if err != nil {
		t.Fatalf("GetObjectInfo: %v", err)
	}
	if info.Name != "ZCL_FOO" || info.PackageName != "ZMYPKG" {
		t.Errorf("unexpected info: %+v", info)
	}
}

func TestGetVersionHistory_ReturnsHistory(t *testing.T) {
	fake := &fakeADTClient{
		versionHistory: []adt.VersionInfo{
			{VersionNumber: "1", Author: "TESTUSER", Date: "2026-01-01T00:00:00Z", Transport: "NPLK900014"},
		},
	}
	tools := agent.NewTools(fake)
	hist, err := tools.GetVersionHistory(context.Background(), "/sap/bc/adt/oo/classes/zcl_foo")
	if err != nil {
		t.Fatalf("GetVersionHistory: %v", err)
	}
	if len(hist) != 1 || hist[0].Author != "TESTUSER" {
		t.Errorf("unexpected history: %+v", hist)
	}
}

func TestWhereUsed_ReturnsCallers(t *testing.T) {
	fake := &fakeADTClient{
		whereUsed: []adt.ObjectInfo{
			{URI: "/sap/bc/adt/oo/classes/zcl_caller", Type: "CLAS", Name: "ZCL_CALLER"},
		},
	}
	tools := agent.NewTools(fake)
	callers, err := tools.WhereUsed(context.Background(), "/sap/bc/adt/oo/classes/zcl_foo")
	if err != nil {
		t.Fatalf("WhereUsed: %v", err)
	}
	if len(callers) != 1 || callers[0].Name != "ZCL_CALLER" {
		t.Errorf("unexpected callers: %+v", callers)
	}
}

func TestDiffActiveInactive_ReturnsDiff(t *testing.T) {
	fake := &fakeADTClient{
		diffResult: &adt.DiffResult{
			HasChanges: true,
			Active:     "old source",
			Inactive:   "new source",
		},
	}
	tools := agent.NewTools(fake)
	diff, err := tools.DiffActiveInactive(context.Background(), "/sap/bc/adt/oo/classes/zcl_foo")
	if err != nil {
		t.Fatalf("DiffActiveInactive: %v", err)
	}
	if !diff.HasChanges {
		t.Error("expected HasChanges=true")
	}
	if diff.Inactive != "new source" {
		t.Errorf("unexpected inactive: %q", diff.Inactive)
	}
}

func TestRunATCCheck_ReturnsFindings(t *testing.T) {
	fake := &fakeADTClient{
		atcResult: &adt.ATCResult{
			WorklistID: "wl1",
			Findings: []adt.ATCFinding{
				{ObjectURI: "/sap/bc/adt/oo/classes/zcl_foo", CheckTitle: "NAMING_CONVENTION", MessageTitle: "Name violates convention", Priority: "1"},
			},
		},
	}
	tools := agent.NewTools(fake)
	result, err := tools.RunATCCheck(context.Background(), []string{"/sap/bc/adt/oo/classes/zcl_foo"}, "")
	if err != nil {
		t.Fatalf("RunATCCheck: %v", err)
	}
	if len(result.Findings) != 1 || result.Findings[0].CheckTitle != "NAMING_CONVENTION" {
		t.Errorf("unexpected findings: %+v", result.Findings)
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

func TestSyntaxCheck_PropagatesError(t *testing.T) {
	fake := &fakeADTClient{syntaxErr: errors.New("ADT unreachable")}
	tools := agent.NewTools(fake)
	_, err := tools.SyntaxCheck(context.Background(), "/sap/bc/adt/oo/classes/zcl_foo")
	if err == nil {
		t.Error("expected error to propagate")
	}
}
