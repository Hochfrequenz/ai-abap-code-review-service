# Public Repository Readiness Design

## Goal

Make the ai-abap-code-review-service repository public-ready as a showcase and fork template. The primary purpose is to demonstrate how little effort it takes to bring real AI-powered code review to SAP BTP — and to give other SAP teams a working starting point for their own deployment.

## Positioning

This is a fork-first template, not a hosted SaaS. Anyone who benefits from it runs their own instance. No comparisons to other products in the documentation.

---

## Changes to README.md

### 1. Hero paragraph (top of file, replaces cold architecture open)

```markdown
**AI-powered ABAP code review on SAP BTP — fork it, deploy it in an afternoon.**

This service connects Claude (Anthropic's AI) to your SAP system's ADT API and reviews
transport requests automatically: ATC findings, naming conventions, dependency analysis,
code style. Review tone, depth, and language are fully customizable by editing Markdown
files — no code changes required. It runs on SAP BTP Cloud Foundry alongside your existing
services. There is no paid service or subscription — you bring your own Anthropic API key
and pay Anthropic directly per use (~$0.20 / ~€0.20 per review with Claude Sonnet,
~$0.10 / ~€0.10 with Haiku; approximate, see [Anthropic pricing](https://www.anthropic.com/pricing)).
```

Note: cost is displayed in USD (matching the codebase's `EstimatedCostUSD`) with EUR in
parentheses as an approximation. The `[Anthropic pricing]` link anchors the claim to the
authoritative source so it doesn't date badly.

### 2. Demo GIF placeholder (immediately after hero)

```markdown
<!-- DEMO GIF — see issue #40 -->
*Demo GIF coming soon.*
```

### 3. Quick Start moved up (immediately after demo placeholder)

Quick Start becomes the second major section, before Architecture. Restructured:

**Prerequisites box** at the top:
- SAP BTP subaccount with Cloud Foundry enabled + a CF user (SAP ID with CF org/space access)
- SAP system with ADT enabled + a technical user with transport read authorizations
  (`SAP_BC_TRANSPORT_ADMINISTRATOR` or equivalent — see Operations notes for known
  authorization edge cases on S/4HANA)
- Anthropic API key

**Step 1** becomes `apply-config`, not CF setup:

```markdown
**Step 1 — Fork and configure**

git clone https://github.com/your-org/ai-abap-code-review-service
# edit config.yml with your BTP coordinates, SAP destination name, CF org/space
go run ./cmd/apply-config
```

`apply-config` rewrites module paths, Go import paths, manifest files, XSUAA security
config, CF deploy workflow, and destination-name constants throughout the codebase
automatically — you don't touch Go code. See `config.yml` for the full list of
configurable fields.

**Step referencing `review_prompt.md` must be removed.** That file no longer exists;
it was replaced by the selectable review styles in `internal/agent/prompts/review_*.md`.
No manual prompt editing is needed as part of the initial setup — the four built-in styles
work out of the box.

Remaining steps (CF service creation, `cf push`, XSUAA setup) stay as-is.

### 4. Forking section (after Quick Start, before Architecture)

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

Note: removed the `→ See CLAUDE.md` link. `CLAUDE.md` is addressed to AI coding assistants,
not human fork authors. It should not be presented as a fork-author guide.

### 5. Architecture section — one update required

The architecture diagram currently references `claude-opus-4` as the fixed model. The model
is now user-selectable at review time (Opus 4.8, Sonnet 4.6, Haiku 4.5). Update the diagram
label to `Claude (selectable: Opus / Sonnet / Haiku)` or equivalent.

Otherwise the section stays as-is and moves to after the Forking section.

---

## New file: CONTRIBUTING.md

```markdown
# Contributing

This is a fork-first template — the best way to use it is to run your own instance.
That said, PRs are welcome for:

- New or improved review prompt styles (`internal/agent/prompts/`)
- Additional reviewable ABAP object types (`internal/agent/uri.go`)
- Bug fixes

Open an issue first for anything beyond a small fix. No CLA, no formal process.
```

---

## Out of scope

- CHANGELOG.md (not a library, not needed)
- SECURITY.md (deferred — the service handles BTP credentials and XSUAA tokens; adding a
  minimal responsible-disclosure contact is recommended but not blocking for initial public release)
- GitHub issue templates (can be added later if needed)
- English translation of `docs/btp-deploy-walkthrough.de.md` (the README Quick Start covers the essentials)
- Making the repo itself a GitHub Template Repository (separate decision for the repo owner)
- A separate human-facing `FORKING.md` (CLAUDE.md serves AI assistants; human fork-author
  guidance lives in the Quick Start and Forking sections of README)
