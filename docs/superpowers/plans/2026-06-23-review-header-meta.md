# Review-Header mit TR-Metadaten Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Den Review-Kopf um Transportauftrags-Titel, Ersteller und die genutzten Review-Einstellungen (Stil + Modell) erweitern, sichtbar im HTML-Review und im gedruckten PDF.

**Architecture:** Die Metadaten werden auf der Go-Seite gehalten, nicht vom LLM erzeugt (Model/Prompt kennt das LLM nicht; Faktentreue-Regeln verbieten ungesicherte Angaben). TR-Titel + Ersteller liefert der Browser beim Submit aus der bereits geladenen TR-Liste mit (Approach B). Alle vier Felder werden beim Job-Anlegen über `reviewstore.JobMeta` persistiert und in `review.html` als Kopfblock gerendert.

**Tech Stack:** Go, gin, html/template, HTMX, In-Memory JobStore.

## Global Constraints

- gin-Handler hängen am JWT-geschützten `api`-Group; Fehler nur via `btp.AbortError` (hier nicht berührt).
- Handler dürfen nur auf Interfaces hängen, nicht auf `*btp.Service` (hier nicht berührt).
- `TRTitle`/`TRAuthor` sind clientseitig geliefert, untrusted, rein kosmetisch — niemals für Logik nutzen, nur als escaptes Template-Output rendern.
- `AllowedModels()`-Labels enthalten HTML-Entities (`&gt;1€`) für den `<option>`-Kontext — vor Speicherung in den Job via `html.UnescapeString` zu Klartext dekodieren, damit `html/template` korrekt re-escaped.

---

### Task 1: Store-Schema erweitern (erledigt)

**Files:**
- Modify: `internal/reviewstore/store.go`
- Modify: `internal/reviewstore/memory.go`

**Interfaces:**
- Produces: `reviewstore.JobMeta{TRID, TRTitle, TRAuthor, ModelLabel, PromptLabel string}`; `JobStore.Create(ctx, meta JobMeta) (*Job, error)`; `Job` mit neuen Feldern `TRTitle, TRAuthor, ModelLabel, PromptLabel`.

- [x] **Step 1:** Felder zu `Job` + `JobMeta`-Struct + Interface-Signatur in `store.go`.
- [x] **Step 2:** `memoryStore.Create` auf `JobMeta` umstellen, neue Felder durchreichen.

### Task 2: Handler füllt JobMeta (erledigt)

**Files:**
- Modify: `examples/aireview/handler.go`

**Interfaces:**
- Consumes: `reviewstore.JobMeta`, `agent.AllowedModels()`, `agent.AllowedPrompts()`.

- [x] **Step 1:** `reviewRequest` um `TRTitle`/`TRAuthor` (`form:"tr_title"`/`"tr_author"`, ohne binding) erweitern.
- [x] **Step 2:** `import "html"`; `store.Create` mit `JobMeta` aufrufen; Model-Label via `html.UnescapeString` dekodieren, Prompt-Label aus `AllowedPrompts()[…].Label`.

### Task 3: Tests an neue Signatur anpassen (TDD-Absicherung)

**Files:**
- Modify: `internal/reviewstore/memory_test.go`
- Modify: `examples/aireview/handler_test.go`

- [x] **Step 1:** In `memory_test.go` die vier `store.Create(ctx, "TRxxx")`-Aufrufe auf `reviewstore.JobMeta{TRID: "TRxxx", …}` umstellen; `TestCreate_*` um Assertions für `TRTitle/TRAuthor/ModelLabel/PromptLabel`-Round-Trip erweitern (+ `TestCreateThenGet_PreservesMetadata`).
- [ ] **Step 2:** `go test ./internal/reviewstore/...` — NICHT lokal ausführbar (kein Go installiert); Verifikation via CI.
- [x] **Step 3:** In `handler_test.go` `fakeStore.Create` auf `JobMeta` umstellen und alle Felder in `f.job` übernehmen.
- [ ] **Step 4:** `go test ./examples/aireview/...` — via CI.

### Task 4: Kopfblock im Template (erledigt — Test fehlt noch)

**Files:**
- Modify: `internal/ui/templates/review.html`
- Modify: `internal/ui/templates_test.go`

- [x] **Step 1:** Kopfblock `<header class="review-meta">` mit `h1` (Transportauftrag {TRID} — {TRTitle}) und Fakten-Zeile (Stil · Modell · Ersteller, leere Felder via `{{with}}` weggelassen) vor `{{.ReviewHTML}}`; greift nur im `done`-Zweig.
- [x] **Step 2:** CSS `.review-meta` / `.review-meta__facts` ergänzt; im `@media print` sichtbar (nur `nav, button` werden ausgeblendet).
- [x] **Step 3:** Tests `TestRenderReview_ContainsHeaderMeta` (+ `TestRenderReview_OmitsEmptyMeta`) geschrieben: Job mit allen Feldern → Output enthält TR-Titel, Ersteller, beide Labels; leere Felder werden weggelassen.

### Task 5: Browser liefert TR-Metadaten

**Files:**
- Modify: `internal/ui/templates/index.html`

- [x] **Step 1:** `htmx:configRequest`-Listener: bei Pfad `/api/reviews` die getrimmte/uppercased Nummer in `allTRs` nachschlagen und `parameters.tr_title`/`tr_author` setzen (leer, wenn nicht gefunden).
- [ ] **Step 2:** Verifikation via CI (kein lokales Go); `templates_test`-Drift-Guards unverändert (keine neue Option).

### Task 6: Verifikation & Commit

- [ ] **Step 1:** `go build ./...` → via CI (kein lokales Go).
- [ ] **Step 2:** `go test ./...` → via CI.
- [x] **Step 3:** Commit der Header-Änderungen.
- [ ] **Step 4:** Branch pushen, CI-Ergebnis (build/test/lint/fmt) abwarten und prüfen.
