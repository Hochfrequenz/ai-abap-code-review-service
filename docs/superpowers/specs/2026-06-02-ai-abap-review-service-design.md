# AI ABAP Code Review Service — Design Spec

**Date:** 2026-06-02
**Status:** Approved

## Overview

An AI-powered ABAP code review service running on SAP BTP Cloud Foundry. Users submit a transport request ID via a minimal HTMX web UI; an agentic Claude loop autonomously fetches objects from the on-premise SAP system via the ADT API (using `github.com/Hochfrequenz/adtler`) and produces a markdown review rendered as a printable HTML page.

---

## 1. Architecture & Package Layout

```
prompts/
  review_prompt.md          ← FORK: review style guide, embedded in agent runner

internal/
  reviewstore/
    store.go                ← JobStore interface + Job/JobStatus types
    memory.go               ← sync.Map-backed implementation (swappable)
  agent/
    runner.go               ← Claude SDK tool-use loop; returns markdown string
    tools.go                ← ADT tools wrapping adtler.Client
    uri.go                  ← objectURI(TransportObject) string helper
  adtclient/
    factory.go              ← NewFromBTPEnv: wires adtler to BTP Connectivity
                               allowed to import both internal/btp and adtler/adt;
                               this is the single bridge between the two
  ui/
    templates.go            ← //go:embed templates/* + template loader (importable sub-package)
    templates_test.go       ← template rendering tests (package ui_test)
    templates/
      index.html
      review.html
      _status.html

examples/
  aireview/
    handler.go              ← gin POST /api/reviews + GET /api/reviews/:id/status
    handler_test.go         ← fake JobStore, goroutine channel test

cmd/server/
  main.go                   ← wires adtler client, store, agent runner; registers routes
```

---

## 2. Job Store

### Interface (`internal/reviewstore/store.go`)

```go
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
    ReviewHTML string    // goldmark-rendered markdown; populated on Done
    ErrMsg     string    // populated on Failed
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

### In-memory implementation (`internal/reviewstore/memory.go`)

- Backed by `sync.Map` keyed by UUID string
- `Create` generates a UUIDv4 (`github.com/google/uuid`) and stores a `*Job` with `status=pending`
- `MarkDone` renders markdown → HTML with goldmark before storing, so the store always holds ready-to-serve HTML
- **Decision recorded:** in-memory chosen for simplicity; interface allows swap to PostgreSQL or any other store with no handler changes

---

## 3. Agent & ADT Tools

### adtler integration (`internal/adtclient/factory.go`)

adtler v0.2.0 ships `NewClientWithTransport(cfg, http.RoundTripper)`. `internal/adtclient` is the single package allowed to bridge `internal/btp` and `adtler/adt`. The factory:

1. Calls `btp.LookupDestination` to get the on-premise host and Basic Auth credentials
2. Constructs `sapmcpconfig.SAPSystem` from the destination — note that `SAPSystem.Client` (three-digit SAP client number, e.g. `"100"`) is **not** present on `btp.Destination` and must come from `config.yml`. Add a `sap_client` field under `examples` in `config.yml` and a corresponding rewriter in `cmd/apply-config/rewriters.go` so forks get it rewritten alongside the destination name.
3. Builds a `btp.TokenFetcher` for the connectivity service credentials (`env.Conn`), then constructs the `ConnTokenProvider` callback:
   ```go
   fetcher, _ := btp.NewTokenFetcher(ctx, env.Conn.TokenURL, env.Conn.ClientID, env.Conn.ClientSecret)
   provider := btp.ConnTokenProvider(func(ctx context.Context) (string, error) {
       return fetcher.Token(ctx)
   })
   transport := btp.NewOnPremiseTransport(env.Conn, provider)
   ```
4. Returns `adt.NewClientWithTransport(cfg, transport)`

The adtler client is constructed once at startup and injected into the agent runner.

### Agent runner (`internal/agent/runner.go`)

```go
func Run(ctx context.Context, client adt.Client, trID string) (string, error)
```

- Uses `github.com/anthropics/anthropic-sdk-go` with model constant `anthropic.ModelClaudeOpus4_8` (= `"claude-opus-4-8"`)
- Prompt caching applied to the system prompt and growing tool-result history
- System prompt loaded from `prompts/review_prompt.md` via `//go:embed ../../prompts/review_prompt.md`
- Runs a tool-use loop until Claude returns `stop_reason: "end_turn"`
- Returns the final markdown review string

### ADT tools (`internal/agent/tools.go`)

Scope is limited to PROG, CLAS, and INTF objects. FUGR/function module includes are **out of scope** for v1 — adtler's include model for function groups requires knowing include sub-path names which have no discovery API. The URI builder and `fetch_class_includes` tool do not handle FUGR.

| Tool | adtler call | Object types |
|---|---|---|
| `list_tr_objects` | `GetTransportObjects(ctx, trID)` | All TR object types (agent decides which to fetch) |
| `fetch_source` | `GetSource(ctx, objectURI)` | PROG / CLAS / INTF main source |
| `fetch_class_includes` | `GetIncludeSource(ctx, uri, include)` × 4 | CLAS only: definitions, implementations, testclasses, macros |

### URI builder (`internal/agent/uri.go`)

`GetTransportObjects` returns `TransportObject{PgmID, Type, Name}` without an ADT URI. A small helper maps object type to ADT path prefix:

```
PROG → /sap/bc/adt/programs/programs/{name}
CLAS → /sap/bc/adt/oo/classes/{name}
INTF → /sap/bc/adt/oo/interfaces/{name}
```

FUGR and other types return an empty string — the agent's system prompt instructs it to skip unknown URIs gracefully.

Claude receives the full URI list from `list_tr_objects` (with unrecognised types noted) and calls `fetch_source` / `fetch_class_includes` directly.

---

## 4. HTTP Handlers

Both handlers in `examples/aireview/handler.go` use the gin style (POST + resource ID pattern; no huma/OpenAPI needed). Both return `Content-Type: text/html` — a deliberate deviation from the template's JSON-only `/api/*` convention, required for HTMX fragment delivery. The `securityHeaders` middleware's `Cache-Control: no-store` still applies to these routes, which is correct (fragments are user-scoped and must not be cached).

### `POST /api/reviews` (JWT-gated)

- Binds `{ "transport_request_id": "NPLK900014" }` with `binding:"required"`
- Calls `store.Create` → gets UUID job ID
- Fires the agent in a goroutine using `context.WithoutCancel(c.Request.Context())` so the job continues after the HTTP response is written. **Never pass `c.Request.Context()` directly** — it is cancelled when the handler returns, which would immediately abort every agent run.
- `Register` therefore accepts a root `context.Context` (the server's signal context from `main.go`) in addition to `store` and `runner`
- Returns HTML fragment: job link + initial status div with HTMX polling wired in
- Response `Content-Type: text/html`

### `GET /api/reviews/:id/status` (JWT-gated)

- Calls `store.Get`; 404 + `btp.ErrorEnvelope` if not found
- Renders `_status.html` fragment for the current job state
- Response `Content-Type: text/html`

### UI routes (no JWT, registered on root gin engine)

These routes sit outside the `/api` group and receive no `Cache-Control` header from `securityHeaders`. Since the pages are shells that load live state via HTMX, this is acceptable — the fragments they load are under `/api/*` and are always `no-store`.

| Route | Handler | Notes |
|---|---|---|
| `GET /` | `ui.IndexHandler` | Serves `index.html` |
| `GET /reviews/:id` | `ui.ReviewHandler` | Serves `review.html`; embeds current status fragment server-side on load |

---

## 5. Frontend Templates (`internal/ui/templates/`)

Templates are declared in `internal/ui/templates.go` (importable sub-package, not `cmd/server/`):

```go
//go:embed templates/*
var FS embed.FS
```

Use `templates/*` (glob) rather than listing files individually so new templates are picked up automatically.

### `_status.html` — three states

**pending / running:**
```html
<div hx-get="/api/reviews/{{.ID}}/status"
     hx-trigger="every 3s"
     hx-swap="outerHTML">
  ⏳ Reviewing {{.TRID}}…
</div>
```

**done:**
```html
<article class="review">{{.ReviewHTML}}</article>
<button onclick="window.print()">Print / Save as PDF</button>
```

**failed:**
```html
<div class="error">Review failed: {{.ErrMsg}}</div>
```

When `done` or `failed`, no `hx-trigger` is present — HTMX stops polling because `outerHTML` replaces the polling element.

### Print CSS

A `<style media="print">` block in `review.html` hides nav and buttons, sets `font-family: serif; max-width: 100%`. `window.print()` produces a clean PDF from the browser.

---

## 6. Authentication

Reuses the template's existing XSUAA pattern unchanged:

- SAP approuter (`web/`) handles the XSUAA OAuth2 redirect; users log in via the SAP BTP login page
- Approuter injects `Authorization: Bearer <jwt>` for all forwarded requests
- `validator.Middleware()` (already on the `/api` group) validates signature, audience, expiry

**`xs-app.json` additions** (before the existing `/api` route, both with `"authenticationType": "xsuaa"`):
```json
{ "source": "^/(reviews.*)$", "target": "/$1", "destination": "backend", "authenticationType": "xsuaa" },
{ "source": "^/$",            "target": "/",   "destination": "backend", "authenticationType": "xsuaa" }
```

The approuter applies CSRF protection by default for non-GET routes. HTMX's `POST` to `/api/reviews` carries an `Authorization` header (Bearer token injected by the approuter) — the approuter does not add a CSRF challenge for routes where the request already carries a bearer token, so no `"csrfProtection": "disabled"` override is needed.

---

## 7. Review Prompt

**Location:** `prompts/review_prompt.md` (repo root)

- Embedded in `internal/agent/runner.go` via `//go:embed ../../prompts/review_prompt.md`
- Carries a `<!-- FORK: -->` comment at the top marking it as the primary customization point
- Contains the review style guide, criteria, and output format instructions Claude follows

---

## 8. Config & Wiring

**`config.yml`** — one new field added:
```yaml
examples:
  destination_name: "HF_S4"   # existing
  sap_client: "100"            # new — three-digit SAP client number for adtler
```
A rewriter in `cmd/apply-config/rewriters.go` rewrites the `sap_client` literal in `internal/adtclient/factory.go` on fork.

**`cmd/server/main.go` additions:**
```go
adtClient, err := adtclient.NewFromBTPEnv(ctx, env)
store := reviewstore.NewMemoryStore()
runner := agent.NewRunner(adtClient)

// UI routes (outside JWT group)
r.GET("/", ui.IndexHandler(tmpl))
r.GET("/reviews/:id", ui.ReviewHandler(tmpl, store))

// API routes (JWT group)
aireview.Register(api, ctx, store, runner)  // ctx = server root context for goroutine
```

**Environment variables:**
- `ANTHROPIC_API_KEY` — set via `cf set-env` or CF app manifest; picked up automatically by the Anthropic SDK

**Dependencies added:**

| Package | Version | Purpose |
|---|---|---|
| `github.com/Hochfrequenz/adtler` | v0.2.0 | SAP ADT API client |
| `github.com/Hochfrequenz/sap-mcp-config` | (transitive via adtler) | `SAPSystem` config struct |
| `github.com/anthropics/anthropic-sdk-go` | v1.46.0 | Claude API + tool-use loop |
| `github.com/google/uuid` | v1.6.0 | Job ID generation |
| `github.com/yuin/goldmark` | v1.8.2 | Markdown → HTML rendering |

---

## 9. Testing Strategy

### Template rendering (`internal/ui/templates_test.go`, `package ui_test`)

| Test | Asserts |
|---|---|
| `_status.html` with pending job | `hx-trigger="every 3s"` present, `hx-get="/api/reviews/<id>/status"` URL correct, no `window.print()` |
| `_status.html` with done job | `ReviewHTML` content present in output, `window.print()` present, no `hx-trigger` |
| `_status.html` with failed job | error message present, no `hx-trigger` |

### Handler integration (`examples/aireview/handler_test.go`, one-method fake `JobStore`)

| Test | Asserts |
|---|---|
| `POST` valid body | 200, `Content-Type: text/html`, response contains `/reviews/<uuid>` |
| `POST` empty body | 400, `btp.ErrorEnvelope` with `CodeInvalidRequest` |
| Goroutine calls `MarkDone` | fake has `doneCh chan string`; test blocks until channel receives; asserts job ID and `ReviewHTML` content |
| `GET /status` pending | 200, `text/html`, `hx-trigger="every 3s"` present, `hx-get` URL matches registered route |
| `GET /status` done | 200, `text/html`, fake's `ReviewHTML` content appears in body, no `hx-trigger` |
| `GET /status` unknown ID | 404, `btp.ErrorEnvelope` |
