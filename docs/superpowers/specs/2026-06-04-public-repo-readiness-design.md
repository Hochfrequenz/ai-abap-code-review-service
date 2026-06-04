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
and pay Anthropic directly per use (~€0.20 per review with Claude Sonnet, ~€0.10 with Haiku).
```

### 2. Demo GIF placeholder (immediately after hero)

```markdown
<!-- DEMO GIF — see issue #40 -->
*Demo GIF coming soon.*
```

### 3. Quick Start moved up (immediately after demo placeholder)

Quick Start becomes the second major section, before Architecture. Restructured:

**Prerequisites box** at the top:
- SAP BTP subaccount with Cloud Foundry enabled + a CF user (SAP ID with CF org/space access)
- SAP system with ADT enabled + a technical user with transport read authorizations (`SAP_BC_TRANSPORT_ADMINISTRATOR` or equivalent)
- Anthropic API key

**Step 1** becomes `apply-config`, not CF setup:

```markdown
**Step 1 — Fork and configure**

```bash
git clone https://github.com/your-org/ai-abap-code-review-service
# edit config.yml with your BTP coordinates and SAP destination name
go run ./cmd/apply-config
```

`apply-config` rewrites module paths, manifest files, and destination-name constants
throughout the codebase automatically — you don't touch Go code.
```

Remaining steps (CF service creation, `cf push`, XSUAA setup) stay as-is.

### 4. Forking section (after Quick Start, before Architecture)

```markdown
## Forking this template

This repo is designed to be forked. The only things you customize:

- `config.yml` — your BTP coordinates, SAP system destination name, CF org/space
- `internal/agent/prompts/review_*.md` — review tone, language, criteria (plain Markdown, no Go)
- `internal/agent/prompts/review_base.md` — shared tool-calling procedure

Run `go run ./cmd/apply-config` once after editing `config.yml` — it rewrites module paths,
manifest files, and destination-name constants throughout the codebase automatically.

→ See [CLAUDE.md](CLAUDE.md) for the complete fork-author guide.
For the underlying Go + SAP BTP Cloud Foundry template this service is built on,
see [Hochfrequenz/go-sap-btp-cf-template](https://github.com/Hochfrequenz/go-sap-btp-cf-template).
```

### 5. Architecture section stays

No changes — it's already well-written. It moves to after the Forking section.

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
- SECURITY.md (not a security-critical public API)
- GitHub issue templates (can be added later if needed)
- English translation of `docs/btp-deploy-walkthrough.de.md` (the README Quick Start covers the essentials)
- Making the repo itself a GitHub Template Repository (separate decision for the repo owner)
