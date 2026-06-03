# AI ABAP Code Review Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an AI-powered ABAP code review service on BTP CF: users submit a transport request ID via an HTMX web UI, a Claude agent fetches objects from SAP via ADT, and returns a printable markdown review.

**Architecture:** Async job pattern — POST creates a UUID job and fires a goroutine; HTMX polls GET /api/reviews/:id/status every 3s until done. Claude uses tool-use to autonomously fetch TR objects and source via adtler. All rendering is server-side (goldmark → HTML fragments).

**Tech Stack:** Go 1.26, Gin, adtler v0.2.0, anthropic-sdk-go v1.46.0, goldmark v1.8.2, google/uuid v1.6.0, HTMX (CDN), SAP BTP Cloud Foundry + XSUAA + Cloud Connector.

**Spec:** `docs/superpowers/specs/2026-06-02-ai-abap-review-service-design.md`

---

## File Map

| File | Action | Purpose |
|---|---|---|
| `internal/reviewstore/store.go` | Create | JobStore interface + Job/JobStatus types |
| `internal/reviewstore/memory.go` | Create | sync.Map-backed in-memory implementation |
| `internal/reviewstore/memory_test.go` | Create | Store tests |
| `internal/agent/uri.go` | Create | objectURI(TransportObject) → ADT path helper |
| `internal/agent/uri_test.go` | Create | URI builder tests |
| `internal/agent/tools.go` | Create | 3 Claude tool definitions wrapping adtler.Client |
| `internal/agent/tools_test.go` | Create | Tools tests with fake adtler client |
| `prompts/review_prompt.md` | Create | System prompt for Claude (FORK point) |
| `internal/agent/runner.go` | Create | Claude tool-use loop; returns markdown |
| `internal/agent/runner_test.go` | Create | Runner test against httptest Claude server |
| `internal/adtclient/factory.go` | Create | NewFromBTPEnv: wires adtler to BTP Connectivity |
| `config.yml` | Modify | Add `examples.sap_client: "100"` field |
| `cmd/apply-config/config.go` | Modify | Add `SapClient string` to ExamplesConfig |
| `cmd/apply-config/rewriters.go` | Modify | Add `planAdtClientSapClient` walker |
| `cmd/apply-config/rewriters_test.go` | Modify | Test new rewriter |
| `internal/ui/templates.go` | Create | embed.FS declaration + template loader |
| `internal/ui/templates_test.go` | Create | Template rendering tests |
| `internal/ui/templates/index.html` | Create | Submit form |
| `internal/ui/templates/review.html` | Create | Result page shell |
| `internal/ui/templates/_status.html` | Create | HTMX polling fragment (3 states) |
| `examples/aireview/handler.go` | Create | POST /api/reviews + GET /api/reviews/:id/status |
| `examples/aireview/handler_test.go` | Create | Handler tests with fake JobStore |
| `cmd/server/main.go` | Modify | Wire adtler, store, runner, register routes |
| `web/xs-app.json` | Modify | Add UI routes with xsuaa auth |

---

## Task 1: Job Store Interface + In-Memory Implementation

**Files:**
- Create: `internal/reviewstore/store.go`
- Create: `internal/reviewstore/memory.go`
- Create: `internal/reviewstore/memory_test.go`

- [ ] **Step 1: Create the interface file**

```go
// internal/reviewstore/store.go
package reviewstore

import (
	"context"
	"time"
)

type JobStatus string

const (
	JobStatusPending JobStatus = "pending"
	JobStatusRunning JobStatus = "running"
	JobStatusDone    JobStatus = "done"
	JobStatusFailed  JobStatus = "failed"
)

type Job struct {
	ID         string
	TRID       string
	Status     JobStatus
	ReviewHTML string
	ErrMsg     string
	CreatedAt  time.Time
	FinishedAt *time.Time
}

type JobStore interface {
	Create(ctx context.Context, trID string) (*Job, error)
	Get(ctx context.Context, id string) (*Job, error)
	MarkRunning(ctx context.Context, id string) error
	MarkDone(ctx context.Context, id string, reviewHTML string) error
	MarkFailed(ctx context.Context, id string, errMsg string) error
}
```

- [ ] **Step 2: Write the failing tests**

```go
// internal/reviewstore/memory_test.go
package reviewstore_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/reviewstore"
)

func TestCreate_ReturnsJobWithPendingStatus(t *testing.T) {
	store := reviewstore.NewMemoryStore()
	job, err := store.Create(context.Background(), "NPLK900014")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if job.ID == "" {
		t.Error("expected non-empty ID")
	}
	if job.TRID != "NPLK900014" {
		t.Errorf("TRID: got %q, want %q", job.TRID, "NPLK900014")
	}
	if job.Status != reviewstore.JobStatusPending {
		t.Errorf("Status: got %q, want pending", job.Status)
	}
}

func TestGet_UnknownID_ReturnsError(t *testing.T) {
	store := reviewstore.NewMemoryStore()
	_, err := store.Get(context.Background(), "does-not-exist")
	if err == nil {
		t.Error("expected error for unknown ID")
	}
}

func TestMarkDone_StoresHTMLAndTimestamp(t *testing.T) {
	store := reviewstore.NewMemoryStore()
	job, _ := store.Create(context.Background(), "TR001")

	err := store.MarkDone(context.Background(), job.ID, "<h1>Review</h1>")
	if err != nil {
		t.Fatalf("MarkDone: %v", err)
	}

	got, _ := store.Get(context.Background(), job.ID)
	if got.Status != reviewstore.JobStatusDone {
		t.Errorf("Status: got %q, want done", got.Status)
	}
	if !strings.Contains(got.ReviewHTML, "<h1>Review</h1>") {
		t.Errorf("ReviewHTML missing expected content, got: %q", got.ReviewHTML)
	}
	if got.FinishedAt == nil {
		t.Error("FinishedAt should be set")
	}
}

func TestMarkFailed_StoresErrMsg(t *testing.T) {
	store := reviewstore.NewMemoryStore()
	job, _ := store.Create(context.Background(), "TR002")

	_ = store.MarkFailed(context.Background(), job.ID, "upstream timeout")

	got, _ := store.Get(context.Background(), job.ID)
	if got.Status != reviewstore.JobStatusFailed {
		t.Errorf("Status: got %q, want failed", got.Status)
	}
	if got.ErrMsg != "upstream timeout" {
		t.Errorf("ErrMsg: got %q", got.ErrMsg)
	}
}

func TestMarkRunning_UpdatesStatus(t *testing.T) {
	store := reviewstore.NewMemoryStore()
	job, _ := store.Create(context.Background(), "TR003")
	_ = store.MarkRunning(context.Background(), job.ID)
	got, _ := store.Get(context.Background(), job.ID)
	if got.Status != reviewstore.JobStatusRunning {
		t.Errorf("Status: got %q, want running", got.Status)
	}
}
```

- [ ] **Step 3: Run tests — expect compile error (NewMemoryStore not defined)**

```
cd C:\github\ai-abap-code-review-service
go test ./internal/reviewstore/...
```

Expected: `undefined: reviewstore.NewMemoryStore`

- [ ] **Step 4: Implement the in-memory store**

```go
// internal/reviewstore/memory.go
package reviewstore

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yuin/goldmark"
)

type memoryStore struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

func NewMemoryStore() JobStore {
	return &memoryStore{jobs: make(map[string]*Job)}
}

func (s *memoryStore) Create(_ context.Context, trID string) (*Job, error) {
	job := &Job{
		ID:        uuid.New().String(),
		TRID:      trID,
		Status:    JobStatusPending,
		CreatedAt: time.Now(),
	}
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()
	return job, nil
}

func (s *memoryStore) Get(_ context.Context, id string) (*Job, error) {
	s.mu.RLock()
	job, ok := s.jobs[id]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("job %q not found", id)
	}
	return job, nil
}

func (s *memoryStore) MarkRunning(_ context.Context, id string) error {
	return s.update(id, func(j *Job) { j.Status = JobStatusRunning })
}

func (s *memoryStore) MarkDone(_ context.Context, id string, reviewHTML string) error {
	html, err := renderMarkdown(reviewHTML)
	if err != nil {
		return err
	}
	now := time.Now()
	return s.update(id, func(j *Job) {
		j.Status = JobStatusDone
		j.ReviewHTML = html
		j.FinishedAt = &now
	})
}

func (s *memoryStore) MarkFailed(_ context.Context, id string, errMsg string) error {
	now := time.Now()
	return s.update(id, func(j *Job) {
		j.Status = JobStatusFailed
		j.ErrMsg = errMsg
		j.FinishedAt = &now
	})
}

func (s *memoryStore) update(id string, fn func(*Job)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return fmt.Errorf("job %q not found", id)
	}
	fn(job)
	return nil
}

func renderMarkdown(md string) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(md), &buf); err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	return buf.String(), nil
}
```

- [ ] **Step 5: Run tests — expect PASS**

```
go test ./internal/reviewstore/... -v
```

Expected: all 5 tests PASS

- [ ] **Step 6: Commit**

```
git add internal/reviewstore/
git commit -m "feat: add JobStore interface and in-memory implementation"
```

---

## Task 2: URI Builder

**Files:**
- Create: `internal/agent/uri.go`
- Create: `internal/agent/uri_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/uri_test.go
package agent_test

import (
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/agent"
)

func TestObjectURI_KnownTypes(t *testing.T) {
	tests := []struct {
		obj  adt.TransportObject
		want string
	}{
		{adt.TransportObject{Type: "PROG", Name: "ZREPORT"}, "/sap/bc/adt/programs/programs/ZREPORT"},
		{adt.TransportObject{Type: "CLAS", Name: "ZCL_EXAMPLE"}, "/sap/bc/adt/oo/classes/ZCL_EXAMPLE"},
		{adt.TransportObject{Type: "INTF", Name: "ZIF_EXAMPLE"}, "/sap/bc/adt/oo/interfaces/ZIF_EXAMPLE"},
	}
	for _, tt := range tests {
		got := agent.ObjectURI(tt.obj)
		if got != tt.want {
			t.Errorf("ObjectURI(%q) = %q, want %q", tt.obj.Type, got, tt.want)
		}
	}
}

func TestObjectURI_UnknownType_ReturnsEmpty(t *testing.T) {
	got := agent.ObjectURI(adt.TransportObject{Type: "FUGR", Name: "ZFUGR"})
	if got != "" {
		t.Errorf("expected empty for FUGR, got %q", got)
	}
}
```

- [ ] **Step 2: Run — expect compile error**

```
go test ./internal/agent/... 2>&1 | head -5
```

- [ ] **Step 3: Implement**

```go
// internal/agent/uri.go
package agent

import (
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
)

// ObjectURI maps a TransportObject to its ADT URI path.
// Returns "" for unsupported types (FUGR, DTEL, etc.) — the agent
// prompt instructs Claude to skip objects with empty URIs.
func ObjectURI(obj adt.TransportObject) string {
	name := strings.ToLower(obj.Name)
	switch obj.Type {
	case "PROG":
		return "/sap/bc/adt/programs/programs/" + name
	case "CLAS":
		return "/sap/bc/adt/oo/classes/" + name
	case "INTF":
		return "/sap/bc/adt/oo/interfaces/" + name
	default:
		return ""
	}
}
```

- [ ] **Step 4: Run tests — expect PASS**

```
go test ./internal/agent/... -run TestObjectURI -v
```

- [ ] **Step 5: Commit**

```
git add internal/agent/uri.go internal/agent/uri_test.go
git commit -m "feat: add ADT URI builder for transport objects"
```

---

## Task 3: Review Prompt Placeholder

**Files:**
- Create: `prompts/review_prompt.md`

- [ ] **Step 1: Create the prompt file**

```markdown
<!-- FORK: This file is the primary customisation point for the AI code review.
     Edit the review criteria, style guide, and output format to match your
     organisation's standards. The file is embedded at build time. -->

# ABAP Code Review Instructions

You are an expert ABAP developer performing a code review of a SAP transport request.

## Your task

1. Call `list_tr_objects` with the provided transport request ID to see all objects.
2. For each PROG, CLAS, or INTF object (skip others — URI will be empty), call `fetch_source` to read the source code.
3. For CLAS objects, also call `fetch_class_includes` to read definitions, implementations, testclasses, and macros.
4. After gathering the code, write a thorough code review in Markdown.

## Review criteria

- **Correctness:** Logic errors, off-by-one, unhandled exceptions, missing SY-SUBRC checks.
- **Naming:** Adherence to naming conventions (Z/Y prefix, meaningful names, no abbreviations).
- **Modularity:** Methods/functions that are too long or do too many things.
- **Error handling:** CATCH blocks that swallow exceptions silently, missing MESSAGE statements.
- **Performance:** SELECT * instead of field list, missing WHERE clause, nested SELECTs in loops.
- **Security:** Dynamic SQL injection risks, missing authority checks.
- **Testability:** Classes without unit tests, global state, hard-coded values.

## Output format

Write your review in Markdown. Structure it as:

```
# Code Review: <Transport Request ID>

## Summary
2–3 sentence executive summary.

## Findings

### <Object Name> (<type>)
For each finding:
**[Severity: Critical/Major/Minor]** Short title
Description and recommendation.

## Overall Assessment
One paragraph.
```

Use `##` and `###` headings, bullet lists for findings. Keep language clear and actionable.
```

- [ ] **Step 2: Verify the file exists**

```
dir prompts\review_prompt.md
```

- [ ] **Step 3: Commit**

```
git add prompts/review_prompt.md
git commit -m "feat: add review prompt placeholder (FORK point)"
```

---

## Task 4: ADT Tools

**Files:**
- Create: `internal/agent/tools.go`
- Create: `internal/agent/tools_test.go`

The tools wrap adtler calls into the shape the Anthropic SDK's tool-use protocol expects. Each tool is a struct with a `Name`, `Description`, `InputSchema`, and an `Execute` func.

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/tools_test.go
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
	// absent includes are omitted, not errors
	if _, ok := result["testclasses"]; ok {
		t.Error("testclasses should be absent")
	}
}
```

- [ ] **Step 2: Run — expect compile error (agent.NewTools not defined)**

```
go test ./internal/agent/... 2>&1 | head -5
```

- [ ] **Step 3: Define the ADTClient interface and Tools**

```go
// internal/agent/tools.go
package agent

import (
	"context"
	"fmt"

	"github.com/Hochfrequenz/adtler/adt"
)

// ADTClient is the subset of adt.Client the agent tools need.
// Using a narrow interface keeps the handler tests simple (one-method fakes).
type ADTClient interface {
	GetTransportObjects(ctx context.Context, transportNumber string) ([]adt.TransportObject, error)
	GetSource(ctx context.Context, objectURI string) (*adt.SourceResult, error)
	GetIncludeSource(ctx context.Context, objectURI, include string) (*adt.SourceResult, error)
}

// TRObject is the agent-facing view of a transport request object.
// URI is pre-computed so Claude doesn't need to know ADT path conventions.
type TRObject struct {
	PgmID string `json:"pgmid"`
	Type  string `json:"type"`
	Name  string `json:"name"`
	URI   string `json:"uri"` // empty for unsupported types
}

// Tools holds the ADT client and exposes the three agent tools as methods.
type Tools struct {
	client ADTClient
}

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
		res, err := t.client.GetIncludeSource(ctx, classURI, inc)
		if err != nil {
			continue // absent include — not an error
		}
		out[inc] = res.Source
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests — expect PASS**

```
go test ./internal/agent/... -v
```

- [ ] **Step 5: Commit**

```
git add internal/agent/tools.go internal/agent/tools_test.go
git commit -m "feat: add ADT tools with narrow ADTClient interface"
```

---

## Task 5: Agent Runner

**Files:**
- Create: `internal/agent/runner.go`
- Create: `internal/agent/runner_test.go`

The runner calls the Claude API in a tool-use loop. Tests redirect the SDK to an `httptest.Server` that returns a canned response.

- [ ] **Step 1: Write the failing test**

The test creates a mock HTTP server returning a Claude-shaped JSON response (one tool call then a final text response), verifies the runner calls the tool and returns the final text.

```go
// internal/agent/runner_test.go
package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/agent"
)

func TestRunner_ToolLoopAndFinalText(t *testing.T) {
	// Track which tool calls the server receives.
	var calls []string

	// The mock server returns a tool-use block on the first request,
	// then a text block on the second (simulating Claude finishing after one tool call).
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		// Parse the incoming request to capture tool results if present.
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		if callCount == 1 {
			// First response: ask Claude to call list_tr_objects.
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           "msg_01",
				"type":         "message",
				"role":         "assistant",
				"model":        "claude-opus-4-8",
				"stop_reason":  "tool_use",
				"content": []map[string]any{
					{
						"type":  "tool_use",
						"id":    "tool_01",
						"name":  "list_tr_objects",
						"input": map[string]any{"transport_request_id": "NPLK900014"},
					},
				},
				"usage": map[string]any{"input_tokens": 10, "output_tokens": 5},
			})
			calls = append(calls, "list_tr_objects")
			return
		}

		// Second response: final review text.
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "msg_02",
			"type":        "message",
			"role":        "assistant",
			"model":       "claude-opus-4-8",
			"stop_reason": "end_turn",
			"content": []map[string]any{
				{"type": "text", "text": "# Code Review\n\nAll good."},
			},
			"usage": map[string]any{"input_tokens": 20, "output_tokens": 15},
		})
	}))
	defer srv.Close()

	// Fake ADT client — returns an empty object list so the tool call succeeds.
	fake := &fakeADTClient{trObjects: nil}
	tools := agent.NewTools(fake)

	// Build a Claude client pointing at the mock server.
	claudeClient := anthropic.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test-key"),
	)

	runner := agent.NewRunner(tools, claudeClient)
	result, err := runner.Run(context.Background(), "NPLK900014")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(result, "Code Review") {
		t.Errorf("expected review text in result, got: %q", result)
	}
	if len(calls) != 1 || calls[0] != "list_tr_objects" {
		t.Errorf("expected list_tr_objects call, got: %v", calls)
	}
}
```

- [ ] **Step 2: Run — expect compile error**

```
go test ./internal/agent/... -run TestRunner 2>&1 | head -5
```

- [ ] **Step 3: Implement the runner**

```go
// internal/agent/runner.go
package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

//go:embed ../../prompts/review_prompt.md
var systemPrompt string

// Runner runs the Claude tool-use loop to produce an ABAP code review.
type Runner struct {
	tools  *Tools
	client *anthropic.Client
}

func NewRunner(tools *Tools, client *anthropic.Client) *Runner {
	return &Runner{tools: tools, client: client}
}

// Run calls Claude with tool access, letting it autonomously fetch TR objects
// and source code, then returns the final markdown review text.
func (r *Runner) Run(ctx context.Context, trID string) (string, error) {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(
			fmt.Sprintf("Please review transport request: %s", trID),
		)),
	}

	toolDefs := r.buildToolDefs()

	for {
		resp, err := r.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaudeOpus4_8,
			MaxTokens: 8192,
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Tools:    toolDefs,
			Messages: messages,
		})
		if err != nil {
			return "", fmt.Errorf("claude api: %w", err)
		}

		// Append assistant turn.
		messages = append(messages, resp.ToParam())

		if resp.StopReason == anthropic.StopReasonEndTurn {
			// Extract the final text block.
			for _, block := range resp.Content {
				if block.Type == anthropic.ContentBlockTypeText {
					return block.Text, nil
				}
			}
			return "", fmt.Errorf("end_turn but no text block in response")
		}

		if resp.StopReason != anthropic.StopReasonToolUse {
			return "", fmt.Errorf("unexpected stop_reason: %s", resp.StopReason)
		}

		// Execute all tool calls and collect results.
		var toolResults []anthropic.ToolResultBlockParam
		for _, block := range resp.Content {
			if block.Type != anthropic.ContentBlockTypeToolUse {
				continue
			}
			result, callErr := r.dispatch(ctx, block.Name, block.Input)
			if callErr != nil {
				result = fmt.Sprintf("error: %v", callErr)
			}
			toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, result, callErr != nil))
		}
		messages = append(messages, anthropic.NewToolResultMessage(toolResults...))
	}
}

func (r *Runner) dispatch(ctx context.Context, toolName string, input json.RawMessage) (string, error) {
	switch toolName {
	case "list_tr_objects":
		var args struct {
			TransportRequestID string `json:"transport_request_id"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		objs, err := r.tools.ListTRObjects(ctx, args.TransportRequestID)
		if err != nil {
			return "", err
		}
		out, _ := json.Marshal(objs)
		return string(out), nil

	case "fetch_source":
		var args struct {
			ObjectURI string `json:"object_uri"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		return r.tools.FetchSource(ctx, args.ObjectURI)

	case "fetch_class_includes":
		var args struct {
			ClassURI string `json:"class_uri"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		includes, err := r.tools.FetchClassIncludes(ctx, args.ClassURI)
		if err != nil {
			return "", err
		}
		out, _ := json.Marshal(includes)
		return string(out), nil

	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}

func (r *Runner) buildToolDefs() []anthropic.ToolParam {
	return []anthropic.ToolParam{
		{
			Name:        "list_tr_objects",
			Description: anthropic.String("List all objects in a SAP transport request. Returns objects with their ADT URIs. Objects with empty URI are unsupported types — skip them."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"transport_request_id": map[string]any{"type": "string", "description": "The transport request number, e.g. NPLK900014"},
				},
				Required: []string{"transport_request_id"},
			},
		},
		{
			Name:        "fetch_source",
			Description: anthropic.String("Fetch the main ABAP source code for an object using its ADT URI."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"object_uri": map[string]any{"type": "string", "description": "The ADT URI of the object, e.g. /sap/bc/adt/oo/classes/zcl_example"},
				},
				Required: []string{"object_uri"},
			},
		},
		{
			Name:        "fetch_class_includes",
			Description: anthropic.String("Fetch all available include sections of an ABAP class (definitions, implementations, testclasses, macros). Returns a map of include name to source code."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"class_uri": map[string]any{"type": "string", "description": "The ADT URI of the class, e.g. /sap/bc/adt/oo/classes/zcl_example"},
				},
				Required: []string{"class_uri"},
			},
		},
	}
}
```

- [ ] **Step 4: Run tests — expect PASS**

```
go test ./internal/agent/... -v
```

- [ ] **Step 5: Commit**

```
git add internal/agent/runner.go internal/agent/runner_test.go
git commit -m "feat: add Claude agent runner with tool-use loop"
```

---

## Task 6: adtler Client Factory

**Files:**
- Create: `internal/adtclient/factory.go`

This package is the single bridge between `internal/btp` and `adtler/adt`. No unit test — it requires a live BTP env. Correct wiring is validated by the running service.

- [ ] **Step 1: Create the factory**

```go
// internal/adtclient/factory.go
package adtclient

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Hochfrequenz/adtler/adt"
	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"

	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/btp"
)

// FORK: "HF_S4" is the BTP destination name. "100" is the SAP client number.
// apply-config rewrites both from config.yml (examples.destination_name and
// examples.sap_client) when you fork this repo.
const (
	destinationName = "HF_S4"
	sapClientNumber = "100"
)

// NewFromBTPEnv builds an adtler Client that routes through the BTP
// Connectivity service's SOCKS5 proxy to the on-premise SAP system.
// This is the single place in the service that bridges internal/btp and adtler.
func NewFromBTPEnv(ctx context.Context, env btp.Env) (adt.Client, error) {
	dest, err := btp.LookupDestination(ctx, env.Dest, destinationName)
	if err != nil {
		return nil, fmt.Errorf("lookup destination %q: %w", destinationName, err)
	}

	// NewTokenFetcher takes an optional *http.Client (nil = 10s default).
	// Fetch takes (ctx, tokenBaseURL, clientID, clientSecret); env.Conn.URL
	// is the XSUAA token base URL from the connectivity binding.
	fetcher := btp.NewTokenFetcher(nil)

	// ConnTokenProvider takes *http.Request so cancellation propagates
	// into the token fetch.
	provider := btp.ConnTokenProvider(func(req *http.Request) (string, error) {
		return fetcher.Fetch(req.Context(), env.Conn.URL, env.Conn.ClientID, env.Conn.ClientSecret)
	})

	transport, err := btp.NewOnPremiseTransport(env.Conn, provider)
	if err != nil {
		return nil, fmt.Errorf("on-premise transport: %w", err)
	}

	cfg := sapmcpconfig.SAPSystem{
		Host:     dest.URL,
		User:     dest.User,
		Password: dest.Password,
		Client:   sapClientNumber,
	}
	return adt.NewClientWithTransport(cfg, transport), nil
}
```

- [ ] **Step 2: Verify it compiles**

```
go build ./internal/adtclient/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```
git add internal/adtclient/factory.go
git commit -m "feat: add adtler BTP factory (bridges internal/btp and adtler)"
```

---

## Task 7: config.yml sap_client + apply-config Rewriter

**Files:**
- Modify: `config.yml`
- Modify: `cmd/apply-config/config.go`
- Modify: `cmd/apply-config/rewriters.go`
- Modify: `cmd/apply-config/rewriters_test.go`

- [ ] **Step 1: Add `sap_client` to config.yml**

Find the `examples:` block in `config.yml` and add the new field:

```yaml
examples:
  destination_name: "HF_S4"
  sap_client: "100"
```

- [ ] **Step 2: Add `SapClient` field to ExamplesConfig in `cmd/apply-config/config.go`**

In the `ExamplesConfig` struct, add after `DestinationName`:

```go
// SapClient is the three-digit SAP client number passed to adtler.
// Replaces the sapClientNumber literal in internal/adtclient/factory.go.
// Defaults to "100".
SapClient string `yaml:"sap_client"`
```

In `applyDefaults()`, after the `DestinationName` default block, add:

```go
c.Examples.SapClient = strings.TrimSpace(c.Examples.SapClient)
if c.Examples.SapClient == "" {
    c.Examples.SapClient = "100"
}
```

- [ ] **Step 3: Write failing test for the new rewriter**

In `cmd/apply-config/rewriters_test.go`, add:

```go
func TestPlanAdtClientSapClient_RewritesSapClient(t *testing.T) {
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "internal", "adtclient", "factory.go")
	_ = os.MkdirAll(filepath.Dir(factoryPath), 0755)
	_ = os.WriteFile(factoryPath, []byte(`package adtclient
const (
	destinationName = "HF_S4"
	sapClientNumber = "100"
)
`), 0644)

	cfg := &Config{Examples: ExamplesConfig{DestinationName: "MY_DEST", SapClient: "200"}}
	plan, err := planAdtClientSapClient(dir, cfg)
	then.AssertThat(t, err, is.Nil())
	then.AssertThat(t, len(plan) > 0, is.True())
	then.AssertThat(t, string(plan[0].result.After), is.StringContaining(`"200"`))
}
```

- [ ] **Step 4: Run test — expect compile error (planAdtClientSapClient not defined)**

```
go test ./cmd/apply-config/... 2>&1 | head -5
```

- [ ] **Step 5: Add the rewriter function to `cmd/apply-config/rewriters.go`**

Add a new function and hook it into `Run`:

```go
// planAdtClientSapClient rewrites the sapClientNumber literal in
// internal/adtclient/factory.go to match config.yml's examples.sap_client.
func planAdtClientSapClient(root string, cfg *Config) ([]pending, error) {
	path := filepath.Join(root, "internal", "adtclient", "factory.go")
	old, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("adtclient factory: read: %w", err)
	}

	// Discover the current value from the const block.
	re := regexp.MustCompile(`sapClientNumber\s*=\s*"([^"]+)"`)
	m := re.FindSubmatch(old)
	if m == nil {
		return nil, nil // constant not present — nothing to rewrite
	}
	current := string(m[1])
	desired := cfg.Examples.SapClient
	rel := filepath.Join("internal", "adtclient", "factory.go")
	if current == desired {
		return []pending{{absPath: path, result: RewriterResult{Name: "adtclient sap_client", Path: rel, Before: old, After: old}}}, nil
	}

	// Replace `sapClientNumber = "old"` with `sapClientNumber = "new"`.
	// The regex matches the full assignment so only the sapClientNumber line
	// is touched, not destinationName or any other string in the file.
	newContent := re.ReplaceAll(old, []byte(`sapClientNumber = "`+desired+`"`))
	return []pending{{
		absPath: path,
		result:  RewriterResult{Name: "adtclient sap_client", Path: rel, Before: old, After: newContent},
	}}, nil
}
```

Then in `Run`, after `examplesPlan`, add:

```go
adtClientPlan, err := planAdtClientSapClient(root, cfg)
if err != nil {
    return nil, err
}
for _, p := range adtClientPlan {
    plan = append(plan, p)
    res.Rewriters = append(res.Rewriters, p.result)
}
```

- [ ] **Step 6: Run apply-config tests — expect PASS**

```
go test ./cmd/apply-config/... -v
```

- [ ] **Step 7: Commit**

```
git add config.yml cmd/apply-config/config.go cmd/apply-config/rewriters.go cmd/apply-config/rewriters_test.go
git commit -m "feat: add sap_client config field and apply-config rewriter"
```

---

## Task 8: UI Templates Package

**Files:**
- Create: `internal/ui/templates.go`
- Create: `internal/ui/templates/_status.html`
- Create: `internal/ui/templates/index.html`
- Create: `internal/ui/templates/review.html`
- Create: `internal/ui/templates_test.go`

- [ ] **Step 1: Write failing template tests**

```go
// internal/ui/templates_test.go
package ui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/reviewstore"
	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/ui"
)

func pendingJob() *reviewstore.Job {
	return &reviewstore.Job{ID: "abc-123", TRID: "NPLK900014", Status: reviewstore.JobStatusPending, CreatedAt: time.Now()}
}
func doneJob() *reviewstore.Job {
	return &reviewstore.Job{ID: "abc-123", TRID: "NPLK900014", Status: reviewstore.JobStatusDone, ReviewHTML: "<p>LGTM</p>", CreatedAt: time.Now()}
}
func failedJob() *reviewstore.Job {
	return &reviewstore.Job{ID: "abc-123", TRID: "NPLK900014", Status: reviewstore.JobStatusFailed, ErrMsg: "upstream timeout", CreatedAt: time.Now()}
}

func TestStatusFragment_Pending_HasPolling(t *testing.T) {
	tmpl := ui.MustLoadTemplates()
	out := mustRenderStatus(t, tmpl, pendingJob())
	if !strings.Contains(out, `hx-trigger="every 3s"`) {
		t.Error("pending fragment must contain hx-trigger")
	}
	if !strings.Contains(out, `/api/reviews/abc-123/status`) {
		t.Error("pending fragment must contain correct hx-get URL")
	}
	if strings.Contains(out, "window.print()") {
		t.Error("pending fragment must not contain print button")
	}
}

func TestStatusFragment_Done_HasContentNoPoll(t *testing.T) {
	tmpl := ui.MustLoadTemplates()
	out := mustRenderStatus(t, tmpl, doneJob())
	if strings.Contains(out, "hx-trigger") {
		t.Error("done fragment must not poll")
	}
	if !strings.Contains(out, "LGTM") {
		t.Error("done fragment must contain ReviewHTML content")
	}
	if !strings.Contains(out, "window.print()") {
		t.Error("done fragment must have print button")
	}
}

func TestStatusFragment_Failed_HasErrorNoPoll(t *testing.T) {
	tmpl := ui.MustLoadTemplates()
	out := mustRenderStatus(t, tmpl, failedJob())
	if strings.Contains(out, "hx-trigger") {
		t.Error("failed fragment must not poll")
	}
	if !strings.Contains(out, "upstream timeout") {
		t.Error("failed fragment must contain error message")
	}
}

func mustRenderStatus(t *testing.T, tmpl ui.Templates, job *reviewstore.Job) string {
	t.Helper()
	out, err := tmpl.RenderStatus(job)
	if err != nil {
		t.Fatalf("RenderStatus: %v", err)
	}
	return out
}
```

- [ ] **Step 2: Run — expect compile error**

```
go test ./internal/ui/... 2>&1 | head -5
```

- [ ] **Step 3: Create templates.go**

```go
// internal/ui/templates.go
package ui

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"

	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/reviewstore"
)

//go:embed templates/*
var templateFS embed.FS

// Templates holds the parsed template set.
type Templates struct {
	t *template.Template
}

// MustLoadTemplates parses all embedded templates and panics on error.
// Call once at startup.
func MustLoadTemplates() Templates {
	t, err := template.ParseFS(templateFS, "templates/*")
	if err != nil {
		panic(fmt.Sprintf("parse templates: %v", err))
	}
	return Templates{t: t}
}

// RenderStatus renders the _status.html fragment for the given job state.
func (tmpl Templates) RenderStatus(job *reviewstore.Job) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.t.ExecuteTemplate(&buf, "_status.html", job); err != nil {
		return "", fmt.Errorf("render _status.html: %w", err)
	}
	return buf.String(), nil
}

// RenderIndex renders the index.html page.
func (tmpl Templates) RenderIndex() (string, error) {
	var buf bytes.Buffer
	if err := tmpl.t.ExecuteTemplate(&buf, "index.html", nil); err != nil {
		return "", fmt.Errorf("render index.html: %w", err)
	}
	return buf.String(), nil
}

// RenderReview renders the review.html page with the current job embedded.
func (tmpl Templates) RenderReview(job *reviewstore.Job) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.t.ExecuteTemplate(&buf, "review.html", job); err != nil {
		return "", fmt.Errorf("render review.html: %w", err)
	}
	return buf.String(), nil
}
```

- [ ] **Step 4: Create `internal/ui/templates/_status.html`**

```html
{{- if eq .Status "done" -}}
<article class="review">{{.ReviewHTML | safeHTML}}</article>
<button onclick="window.print()">Print / Save as PDF</button>
{{- else if eq .Status "failed" -}}
<div class="error">Review failed: {{.ErrMsg}}</div>
{{- else -}}
<div hx-get="/api/reviews/{{.ID}}/status"
     hx-trigger="every 3s"
     hx-swap="outerHTML">
  ⏳ Reviewing {{.TRID}}…
</div>
{{- end -}}
```

Note: `safeHTML` needs to be registered as a template function so `ReviewHTML` (which is already sanitised server-side goldmark output) renders as HTML, not escaped text. Add it to `MustLoadTemplates`:

```go
func MustLoadTemplates() Templates {
	t := template.New("").Funcs(template.FuncMap{
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
	})
	t, err := t.ParseFS(templateFS, "templates/*")
	if err != nil {
		panic(fmt.Sprintf("parse templates: %v", err))
	}
	return Templates{t: t}
}
```

- [ ] **Step 5: Create `internal/ui/templates/index.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>ABAP Code Review</title>
  <script src="https://unpkg.com/htmx.org@2.0.4" crossorigin="anonymous"></script>
</head>
<body>
  <h1>ABAP Code Review</h1>
  <form hx-post="/api/reviews"
        hx-target="#result"
        hx-swap="innerHTML">
    <label for="tr">Transport Request ID</label>
    <input id="tr" name="transport_request_id" placeholder="NPLK900014" required>
    <button type="submit">Request Review</button>
  </form>
  <div id="result"></div>
</body>
</html>
```

- [ ] **Step 6: Create `internal/ui/templates/review.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Review {{.TRID}}</title>
  <script src="https://unpkg.com/htmx.org@2.0.4" crossorigin="anonymous"></script>
  <style media="print">
    nav, button { display: none; }
    body { font-family: serif; max-width: 100%; }
  </style>
</head>
<body>
  <nav><a href="/">← New review</a></nav>
  <div id="status">
    {{template "_status.html" .}}
  </div>
</body>
</html>
```

- [ ] **Step 7: Run tests — expect PASS**

```
go test ./internal/ui/... -v
```

- [ ] **Step 8: Commit**

```
git add internal/ui/
git commit -m "feat: add UI template package with embedded HTMX templates"
```

---

## Task 9: HTTP Handlers

**Files:**
- Create: `examples/aireview/handler.go`
- Create: `examples/aireview/handler_test.go`

- [ ] **Step 1: Write failing handler tests**

```go
// examples/aireview/handler_test.go
package aireview_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hochfrequenz/go-sap-btp-cf-template/examples/aireview"
	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/reviewstore"
	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/ui"
)

// fakeStore implements reviewstore.JobStore for tests.
type fakeStore struct {
	job    *reviewstore.Job
	doneCh chan string // receives ReviewHTML when MarkDone is called
	getErr error
}

func newFakeStore(jobID string) *fakeStore {
	return &fakeStore{
		job: &reviewstore.Job{
			ID:        jobID,
			TRID:      "NPLK000001",
			Status:    reviewstore.JobStatusPending,
			CreatedAt: time.Now(),
		},
		doneCh: make(chan string, 1),
	}
}

func (f *fakeStore) Create(_ context.Context, trID string) (*reviewstore.Job, error) {
	f.job.TRID = trID
	return f.job, nil
}
func (f *fakeStore) Get(_ context.Context, _ string) (*reviewstore.Job, error) {
	return f.job, f.getErr
}
func (f *fakeStore) MarkRunning(_ context.Context, _ string) error { return nil }
func (f *fakeStore) MarkDone(_ context.Context, _ string, html string) error {
	f.job.Status = reviewstore.JobStatusDone
	f.job.ReviewHTML = html
	f.doneCh <- html
	return nil
}
func (f *fakeStore) MarkFailed(_ context.Context, _ string, errMsg string) error {
	f.job.Status = reviewstore.JobStatusFailed
	f.job.ErrMsg = errMsg
	return nil
}

// fakeRunner returns a canned markdown review immediately.
type fakeRunner struct{}

func (f *fakeRunner) Run(_ context.Context, _ string) (string, error) {
	return "# Review\n\nAll good.", nil
}

func newRouter(store reviewstore.JobStore, runner aireview.ReviewRunner, tmpl ui.Templates) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	aireview.Register(api, context.Background(), store, runner, tmpl)
	return r
}

func TestPost_ValidBody_Returns200WithLink(t *testing.T) {
	store := newFakeStore("test-uuid-1")
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	body, _ := json.Marshal(map[string]string{"transport_request_id": "NPLK900014"})
	req := httptest.NewRequest(http.MethodPost, "/api/reviews", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type: got %q, want text/html", ct)
	}
	if !strings.Contains(w.Body.String(), "/reviews/") {
		t.Errorf("response should contain review link, got: %s", w.Body.String())
	}
}

func TestPost_EmptyBody_Returns400(t *testing.T) {
	store := newFakeStore("test-uuid-2")
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	req := httptest.NewRequest(http.MethodPost, "/api/reviews", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", w.Code)
	}
}

func TestPost_GoroutineCallsMarkDone(t *testing.T) {
	store := newFakeStore("test-uuid-3")
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	body, _ := json.Marshal(map[string]string{"transport_request_id": "NPLK900014"})
	req := httptest.NewRequest(http.MethodPost, "/api/reviews", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Block until goroutine calls MarkDone.
	select {
	case html := <-store.doneCh:
		if !strings.Contains(html, "Review") {
			t.Errorf("MarkDone called with unexpected HTML: %q", html)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for MarkDone to be called")
	}
}

func TestGetStatus_Pending_HasPolling(t *testing.T) {
	store := newFakeStore("test-uuid-4")
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	req := httptest.NewRequest(http.MethodGet, "/api/reviews/test-uuid-4/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `hx-trigger="every 3s"`) {
		t.Error("pending response must contain hx-trigger")
	}
	if !strings.Contains(body, `/api/reviews/test-uuid-4/status`) {
		t.Error("pending response must contain correct hx-get URL")
	}
}

func TestGetStatus_Done_HasContentNoPoll(t *testing.T) {
	store := newFakeStore("test-uuid-5")
	store.job.Status = reviewstore.JobStatusDone
	store.job.ReviewHTML = "<p>LGTM</p>"
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	req := httptest.NewRequest(http.MethodGet, "/api/reviews/test-uuid-5/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if strings.Contains(body, "hx-trigger") {
		t.Error("done response must not poll")
	}
	if !strings.Contains(body, "LGTM") {
		t.Errorf("done response must contain ReviewHTML, got: %s", body)
	}
}

func TestGetStatus_UnknownID_Returns404(t *testing.T) {
	store := newFakeStore("test-uuid-6")
	store.getErr = fmt.Errorf("job not found")
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	req := httptest.NewRequest(http.MethodGet, "/api/reviews/no-such-id/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", w.Code)
	}
}
```

(Add `"fmt"` to the imports of the test file.)

- [ ] **Step 2: Run — expect compile error**

```
go test ./examples/aireview/... 2>&1 | head -5
```

- [ ] **Step 3: Implement the handler**

```go
// examples/aireview/handler.go
package aireview

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/btp"
	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/reviewstore"
	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/ui"
)

// ReviewRunner is the interface the handler uses to start a review.
// Satisfied by *agent.Runner in production and fakeRunner in tests.
type ReviewRunner interface {
	Run(ctx context.Context, trID string) (string, error)
}

type reviewRequest struct {
	TransportRequestID string `json:"transport_request_id" binding:"required"`
}

// Register attaches the two aireview routes to the JWT-guarded api group.
// rootCtx must be the server's root context (not a request context) so the
// goroutine continues after the HTTP response is written.
func Register(api *gin.RouterGroup, rootCtx context.Context, store reviewstore.JobStore, runner ReviewRunner, tmpl ui.Templates) {
	api.POST("/reviews", postReview(rootCtx, store, runner, tmpl))
	api.GET("/reviews/:id/status", getStatus(store, tmpl))
}

func postReview(rootCtx context.Context, store reviewstore.JobStore, runner ReviewRunner, tmpl ui.Templates) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req reviewRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			btp.AbortError(c, http.StatusBadRequest, btp.CodeInvalidRequest, err.Error(), nil)
			return
		}

		job, err := store.Create(c.Request.Context(), req.TransportRequestID)
		if err != nil {
			btp.AbortError(c, http.StatusInternalServerError, btp.CodeInternal, "failed to create review job", err)
			return
		}

		// Use context.WithoutCancel so the goroutine outlives the HTTP response.
		go func(ctx context.Context, jobID, trID string) {
			_ = store.MarkRunning(ctx, jobID)
			md, runErr := runner.Run(ctx, trID)
			if runErr != nil {
				_ = store.MarkFailed(ctx, jobID, runErr.Error())
				return
			}
			_ = store.MarkDone(ctx, jobID, md)
		}(context.WithoutCancel(rootCtx), job.ID, job.TRID)

		// Return an HTML fragment with the job link and initial polling div.
		fragment := fmt.Sprintf(
			`<p>Review started — <a href="/reviews/%s">view results</a></p>`+
				`<div hx-get="/api/reviews/%s/status" hx-trigger="every 3s" hx-swap="outerHTML">⏳ Starting…</div>`,
			job.ID, job.ID,
		)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(fragment))
	}
}

func getStatus(store reviewstore.JobStore, tmpl ui.Templates) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		job, err := store.Get(c.Request.Context(), id)
		if err != nil {
			btp.AbortError(c, http.StatusNotFound, btp.CodeNotFound, "review not found", err)
			return
		}

		html, err := tmpl.RenderStatus(job)
		if err != nil {
			btp.AbortError(c, http.StatusInternalServerError, btp.CodeInternal, "render failed", err)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
	}
}
```

- [ ] **Step 4: Run all tests — expect PASS**

```
go test ./... 2>&1 | tail -20
```

- [ ] **Step 5: Commit**

```
git add examples/aireview/
git commit -m "feat: add aireview gin handlers with HTMX fragment responses"
```

---

## Task 10: Wire main.go + xs-app.json

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `web/xs-app.json`

- [ ] **Step 1: Add UI + review routes to `buildRouter` in `cmd/server/main.go`**

At the top of the file, add imports:
```go
"github.com/hochfrequenz/go-sap-btp-cf-template/examples/aireview"
"github.com/hochfrequenz/go-sap-btp-cf-template/internal/ui"
```

`buildRouter` currently takes `(validator, caller, mutator, logger)` — update **both** the function signature (line ~187) and its call site in `main()` (line ~55 — `r := buildRouter(validator, svc, svc, logger)`).

New signature:
```go
func buildRouter(
    validator *btp.JWTValidator,
    caller btp.OnPremCaller,
    mutator btp.OnPremMutator,
    logger *slog.Logger,
    store reviewstore.JobStore,
    runner aireview.ReviewRunner,
    tmpl ui.Templates,
    rootCtx context.Context,
) *gin.Engine {
```

Inside `buildRouter`, after the existing `adtcheckrun.Register` call:
```go
// UI routes (no JWT — shells only; HTMX calls under /api are JWT-gated)
r.GET("/", func(c *gin.Context) {
    html, err := tmpl.RenderIndex()
    if err != nil {
        c.String(http.StatusInternalServerError, "template error")
        return
    }
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
})
r.GET("/reviews/:id", func(c *gin.Context) {
    id := c.Param("id")
    job, err := store.Get(c.Request.Context(), id)
    if err != nil {
        c.String(http.StatusNotFound, "review not found")
        return
    }
    html, err := tmpl.RenderReview(job)
    if err != nil {
        c.String(http.StatusInternalServerError, "template error")
        return
    }
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
})

// HTMX API routes (JWT-gated)
aireview.Register(api, rootCtx, store, runner, tmpl)
```

- [ ] **Step 2: Add startup wiring in `main()`**

After `svc, err := btp.NewService(...)`, add:

```go
adtClient, err := adtclient.NewFromBTPEnv(ctx, env)
if err != nil {
    logger.Error("adtler client init failed", "err", err)
    os.Exit(1)
}

store := reviewstore.NewMemoryStore()
agentTools := agent.NewTools(adtClient)

claudeClient := anthropic.NewClient() // reads ANTHROPIC_API_KEY from env
runner := agent.NewRunner(agentTools, claudeClient)
tmpl := ui.MustLoadTemplates()
```

Add these imports:
```go
"github.com/anthropics/anthropic-sdk-go"
"github.com/hochfrequenz/go-sap-btp-cf-template/internal/adtclient"
"github.com/hochfrequenz/go-sap-btp-cf-template/internal/agent"
"github.com/hochfrequenz/go-sap-btp-cf-template/internal/reviewstore"
"github.com/hochfrequenz/go-sap-btp-cf-template/internal/ui"
```

Update the `buildRouter` call:
```go
r := buildRouter(validator, svc, svc, logger, store, runner, tmpl, ctx)
```

- [ ] **Step 3: Verify the service compiles**

```
go build ./cmd/server/...
```

Expected: no errors.

- [ ] **Step 4: Update `web/xs-app.json`**

Add two routes before the existing `/api` route:

```json
{
  "welcomeFile": "/",
  "authenticationMethod": "route",
  "sessionTimeout": 30,
  "routes": [
    {
      "source": "^/$",
      "target": "/",
      "destination": "GoBackend",
      "authenticationType": "xsuaa"
    },
    {
      "source": "^/reviews/(.*)$",
      "target": "/reviews/$1",
      "destination": "GoBackend",
      "authenticationType": "xsuaa"
    },
    {
      "source": "^/api/(.*)$",
      "target": "/api/$1",
      "destination": "GoBackend",
      "authenticationType": "xsuaa",
      "csrfProtection": false
    },
    {
      "source": "^/healthz$",
      "target": "/healthz",
      "destination": "GoBackend",
      "authenticationType": "none"
    },
    {
      "source": "^/version$",
      "target": "/version",
      "destination": "GoBackend",
      "authenticationType": "none"
    }
  ]
}
```

- [ ] **Step 5: Run all tests**

```
go test ./... 2>&1 | tail -20
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```
git add cmd/server/main.go web/xs-app.json
git commit -m "feat: wire aireview handlers, UI routes, and adtler client into server"
```

---

## Done

All tasks complete. The service now:
- Accepts transport request IDs via an HTMX form at `GET /`
- Creates async review jobs (UUID-keyed, in-memory)
- Runs a Claude agent that autonomously fetches ABAP objects via adtler
- Polls for results at `GET /api/reviews/:id/status` and renders them as printable HTML
- Rewrites fork-specific literals (`destinationName`, `sapClientNumber`) via `apply-config`
