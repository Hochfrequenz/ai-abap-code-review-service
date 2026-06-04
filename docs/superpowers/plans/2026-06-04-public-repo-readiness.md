# Public Repository Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make README.md and add CONTRIBUTING.md so the repo is ready to be made public as a fork-first showcase template.

**Architecture:** Documentation-only changes — no Go code, no tests. Two files: `README.md` (multiple targeted edits) and new `CONTRIBUTING.md`.

**Tech Stack:** Markdown, git.

---

## File Map

| Action | Path | What changes |
|--------|------|-------------|
| Modify | `README.md` | Hero paragraph, demo GIF placeholder, Quick Start reorder + fix, Forking section, Architecture diagram, Customisation table |
| Create | `CONTRIBUTING.md` | Fork-first contribution guide |

---

## Task 1: Hero paragraph + demo GIF placeholder

**Files:**
- Modify: `README.md` lines 1–4

Replace the existing title and one-line description with the hero paragraph and GIF placeholder.

Current lines 1–4:
```
# AI ABAP Code Review Service

An AI-powered code review service for SAP ABAP, running on **SAP BTP Cloud Foundry**.
Users submit a transport request ID via a web UI; a Claude agent autonomously fetches ABAP source objects from the on-premise SAP system via ADT, and returns a structured, printable markdown review.
```

- [ ] **Step 1: Replace lines 1–4 of README.md with this content**

```markdown
# AI ABAP Code Review Service

**AI-powered ABAP code review on SAP BTP — fork it, deploy it in an afternoon.**

This service connects Claude (Anthropic's AI) to your SAP system's ADT API and reviews
transport requests automatically: ATC findings, naming conventions, dependency analysis,
code style. Review tone, depth, and language are fully customizable by editing Markdown
files — no code changes required. It runs on SAP BTP Cloud Foundry alongside your existing
services. There is no paid service or subscription — you bring your own Anthropic API key
and pay Anthropic directly per use (~$0.20 / ~€0.20 per review with Claude Sonnet,
~$0.10 / ~€0.10 with Haiku; approximate, see [Anthropic pricing](https://www.anthropic.com/pricing)).

<!-- DEMO GIF — see issue #40 -->
*Demo GIF coming soon.*

```

- [ ] **Step 2: Verify the file looks right**

```powershell
Get-Content README.md | Select-Object -First 15
```

Expected: hero paragraph followed by GIF placeholder comment and italicised text.

- [ ] **Step 3: Commit**

```powershell
git add README.md
git commit -m "docs: add hero paragraph and demo GIF placeholder to README"
```

---

## Task 2: Move Quick Start before Architecture and fix its content

**Files:**
- Modify: `README.md`

The current order is: Architecture → Quick Start. The new order is: Hero → GIF → Quick Start → Forking → Architecture.

The Quick Start also needs three fixes:
1. Add a prerequisites box at the top
2. Fix Step 1 to make apply-config the first thing (with better description)
3. Remove the stale step referencing `review_prompt.md` (file no longer exists)

- [ ] **Step 1: Delete the current `## Architecture` section and its content (lines 6–23), cut `## Quick start` section (lines 34–41), and paste them in the right order: Quick Start first, then Architecture**

The new README structure after the GIF placeholder:

```markdown
## Quick start

**Prerequisites:**
- SAP BTP subaccount with Cloud Foundry enabled + a CF user (SAP ID with CF org/space access)
- SAP system with ADT enabled + a technical user with transport read authorizations
  (`SAP_BC_TRANSPORT_ADMINISTRATOR` or equivalent — see [Operations notes](#operations-notes-hf-deployment)
  for known authorization edge cases on S/4HANA)
- Anthropic API key

**Step 1 — Fork and configure**

```bash
git clone https://github.com/your-org/ai-abap-code-review-service
# edit config.yml with your BTP landscape coordinates and SAP destination name
go run ./cmd/apply-config
```

`apply-config` rewrites module paths, Go import paths, manifest files, XSUAA security config,
CF deploy workflow, and destination-name constants throughout the codebase automatically —
you don't touch Go code. See `config.yml` for the full list of configurable fields.

2. Set `ANTHROPIC_API_KEY` in your CF environment:
   ```bash
   cf set-env <app-name> ANTHROPIC_API_KEY sk-ant-...
   ```
3. Cross-compile the binary (`make build-linux` or `.\scripts\build.ps1`), then:
   ```bash
   cf push --vars-file vars.yml
   ```
4. Open `https://<app-name>-web.<domain>/` and enter a transport request number.
```

Note: steps 3 (`Customize the review prompt: edit internal/agent/prompts/review_prompt.md`) and 4 (`Run go run ./cmd/apply-config to rewrite the tree`) from the original Quick Start are **removed** — `review_prompt.md` no longer exists and apply-config is now step 1.

Then paste the Architecture section after Quick Start (unchanged except the diagram label fix done in Task 4).

- [ ] **Step 2: Verify structure**

```powershell
Select-String "^## " README.md
```

Expected order: `## Quick start`, `## Architecture` (or later: after Forking section is added in Task 3), `## Why direct ADT wiring`, `## Local development`, `## How it works`, `## Deployed instance`, `## Operations notes`, `## Customisation`, `## License`.

- [ ] **Step 3: Commit**

```powershell
git add README.md
git commit -m "docs: move Quick Start before Architecture, add prerequisites, fix apply-config step"
```

---

## Task 3: Add Forking section

**Files:**
- Modify: `README.md` — insert new section after Quick Start, before Architecture

- [ ] **Step 1: Insert the Forking section between Quick Start and Architecture**

```markdown
## Forking this template

This repo is designed to be forked. The only things you customize:

- `config.yml` — your BTP coordinates, SAP system destination name, CF org/space
- `internal/agent/prompts/review_*.md` — review tone, criteria, output format
  (plain Markdown, no Go). The file `review_guidelines_hf.md` contains
  Hochfrequenz-specific coding guidelines — replace it with your own or delete it.
- `internal/agent/prompts/review_base.md` — shared tool-calling procedure (optional;
  only change this if you want to add or remove ADT tools from the review workflow)

Run `go run ./cmd/apply-config` once after editing `config.yml`. See `config.yml`
for the full list of fields it rewrites across the codebase.

For the underlying Go + SAP BTP Cloud Foundry template this service is built on,
see [Hochfrequenz/go-sap-btp-cf-template](https://github.com/Hochfrequenz/go-sap-btp-cf-template).
```

- [ ] **Step 2: Verify it appears between Quick Start and Architecture**

```powershell
Select-String "^## " README.md
```

Expected: `## Quick start` → `## Forking this template` → `## Architecture` → …

- [ ] **Step 3: Commit**

```powershell
git add README.md
git commit -m "docs: add Forking section with apply-config guidance and template repo link"
```

---

## Task 4: Fix Architecture diagram model label

**Files:**
- Modify: `README.md` — the mermaid block, currently line ~13 (shifts down after Task 2/3)

Current:
```
    Go -->|tool calls| Agent[Claude Agent\nclaude-opus-4]
```

Replace with:
```
    Go -->|tool calls| Agent[Claude Agent\nOpus / Sonnet / Haiku]
```

- [ ] **Step 1: Update the mermaid diagram label**

Find and replace `claude-opus-4` with `Opus / Sonnet / Haiku` in the mermaid block. Only one occurrence exists.

- [ ] **Step 2: Verify**

```powershell
Select-String "claude-opus-4" README.md
```

Expected: no matches.

- [ ] **Step 3: Commit**

```powershell
git add README.md
git commit -m "docs: update architecture diagram — model is now user-selectable"
```

---

## Task 5: Update Customisation table

**Files:**
- Modify: `README.md` — the Customisation table (currently lines 121–128, shifts after earlier tasks)

Current table:

```markdown
| What | Where |
| ---- | ----- |
| Review prompt | `internal/agent/prompts/review_prompt.md` |
| AI model | `reviewModel` constant in `internal/agent/runner.go` |
| Token budget | `reviewMaxTokens` constant in `internal/agent/runner.go` |
| Persistence (swap in-memory store) | implement `reviewstore.JobStore` in `internal/reviewstore/store.go` |
```

Replace with:

```markdown
| What | Where |
| ---- | ----- |
| Review style | Select in the UI; edit `internal/agent/prompts/review_*.md` for tone/criteria/format |
| Shared review procedure | `internal/agent/prompts/review_base.md` (tool-calling steps, ATC rule) |
| AI model | Select per review in the UI; models defined in `AllowedModels()` in `internal/agent/runner.go` |
| Token budget | `reviewMaxTokens` constant in `internal/agent/runner.go` |
| Persistence (swap in-memory store) | implement `reviewstore.JobStore` in `internal/reviewstore/store.go` |
```

Changes:
- Row 1: `review_prompt.md` (deleted file) → `review_*.md` with UI note
- Row 2: new row for `review_base.md`
- Row 3: `reviewModel` constant (removed) → UI + `AllowedModels()` note

- [ ] **Step 1: Replace the Customisation table**

- [ ] **Step 2: Verify no mention of `review_prompt.md` or `reviewModel` remains in README**

```powershell
Select-String "review_prompt|reviewModel" README.md
```

Expected: no matches.

- [ ] **Step 3: Commit**

```powershell
git add README.md
git commit -m "docs: update Customisation table — remove stale review_prompt.md and reviewModel references"
```

---

## Task 6: Create CONTRIBUTING.md

**Files:**
- Create: `CONTRIBUTING.md`

- [ ] **Step 1: Create CONTRIBUTING.md with this exact content**

```markdown
# Contributing

This is a fork-first template — the best way to use it is to run your own instance.
That said, PRs are welcome for:

- New or improved review prompt styles (`internal/agent/prompts/`)
- Additional reviewable ABAP object types (`internal/agent/uri.go`)
- Bug fixes

Open an issue first for anything beyond a small fix. No CLA, no formal process.
```

- [ ] **Step 2: Verify it exists and is non-empty**

```powershell
Get-Content CONTRIBUTING.md
```

Expected: the 8-line file above.

- [ ] **Step 3: Commit**

```powershell
git add CONTRIBUTING.md
git commit -m "docs: add CONTRIBUTING.md — fork-first contribution guide"
```

---

## Done

Open a PR from the working branch to main. No tests to run — this is documentation only. Verify by reading the full README once from top to bottom to confirm the flow reads naturally: hero → GIF placeholder → Quick Start (with prerequisites + correct steps) → Forking section → Architecture → … → Customisation (with updated table) → License.
