# Review Styles (Rezensions-Stile) Implementation Design

## Goal

Replace the single hard-coded system prompt with a set of selectable, named review styles.
Users choose a style (Rezensions-Stil) alongside the model and transport-request ID.
All styles are fixed markdown files compiled into the binary — users never write or edit system prompts.

## Architecture

The `AllowedPrompts()` function in `internal/agent/runner.go` is the single source of truth, mirroring the existing `AllowedModels()` pattern.
Each style is a separate embedded markdown file.
The handler validates the submitted style key and passes it to `Run()`; the runner resolves the key to the prompt text internally.

## Tech Stack

Go `//go:embed`, existing Gin handler pattern, existing HTMX form, existing drift-guard test pattern.

---

## Prompt Files

Four files live in `internal/agent/prompts/`.
The existing `review_prompt.md` is deleted; its content is redistributed into the new files as a starting point.

**All prompt markdown files must be written in German.** Claude responds in the language of the system prompt; German system prompts produce German reviews.

| File | Key | German label |
|------|-----|-------------|
| `review_pedantic.md` | `review_pedantic` | Pedantische Code-Review für erfahrene Entwickler\*innen |
| `review_appreciative.md` | `review_appreciative` | Wertschätzende Code-Review mit praktischen Tipps für Newbies |
| `review_analytical.md` | `review_analytical` | Technisch-Analytische Code-Review (Selbst-Konsistenz des TA) |
| `review_guidelines_hf.md` | `review_guidelines_hf` | Prüfung gegen HF-Entwicklungsrichtlinien |

Adding a new style later: add the `.md` file, add an embed var, add an entry in `AllowedPrompts()`, add an `<option>` in `index.html`. The drift-guard tests catch any missing step at CI time.

## Data Model (`internal/agent/runner.go`)

```go
// Prompt pairs a German UI label with the compiled-in system prompt text.
type Prompt struct {
    Label string
    Text  string
}

//go:embed prompts/review_pedantic.md
var promptPedantic string

//go:embed prompts/review_appreciative.md
var promptAppreciative string

//go:embed prompts/review_analytical.md
var promptAnalytical string

//go:embed prompts/review_guidelines_hf.md
var promptGuidelinesHF string

// AllowedPrompts returns the set of review styles the service accepts,
// mapped to their German UI label and compiled-in system prompt text.
func AllowedPrompts() map[string]Prompt {
    return map[string]Prompt{
        "review_pedantic":      {Label: "Pedantische Code-Review für erfahrene Entwickler*innen", Text: promptPedantic},
        "review_appreciative":  {Label: "Wertschätzende Code-Review mit praktischen Tipps für Newbies", Text: promptAppreciative},
        "review_analytical":    {Label: "Technisch-Analytische Code-Review (Selbst-Konsistenz des TA)", Text: promptAnalytical},
        "review_guidelines_hf": {Label: "Prüfung gegen HF-Entwicklungsrichtlinien", Text: promptGuidelinesHF},
    }
}
```

`Run()` signature extends with `promptKey string`:

```go
func (r *Runner) Run(ctx context.Context, trID, model, promptKey string) (string, error)
```

The runner looks up `AllowedPrompts()[promptKey].Text` and uses it as the system prompt.
No fallback — callers are responsible for validation (same contract as `model`).
The old `var systemPrompt string` embed is removed.

## Handler (`examples/aireview/handler.go`)

`reviewRequest` gains a `Prompt` field.
`binding:"required"` is omitted from both `Model` and `Prompt` — validation is handled exclusively by the `AllowedModels()` and `AllowedPrompts()` map lookups, which already reject empty strings (an empty string is not a key in either map).
This keeps the validation path consistent for both fields.

```go
type reviewRequest struct {
    TRID   string `json:"transport_request_id" form:"transport_request_id" binding:"required"`
    Model  string `json:"model"                form:"model"`
    Prompt string `json:"prompt"               form:"prompt"`
}
```

Validation block (after binding, before job creation) — prompt validated first, then model:

```go
if _, ok := agent.AllowedPrompts()[req.Prompt]; !ok {
    btp.AbortError(c, http.StatusBadRequest, btp.CodeInvalidRequest,
        fmt.Sprintf("Rezensions-Stil unbekannt %q — erlaubt: %s", req.Prompt, allowedPromptKeys()), nil)
    return
}
if _, ok := agent.AllowedModels()[req.Model]; !ok {
    btp.AbortError(c, http.StatusBadRequest, btp.CodeInvalidRequest,
        fmt.Sprintf("Modell fehlt oder unbekannt %q — erlaubt: %s", req.Model, allowedModelKeys()), nil)
    return
}
```

`allowedPromptKeys()` is a sorted helper identical to `allowedModelKeys()`.
The sort is mandatory (not cosmetic) — test assertions on the error message text depend on deterministic ordering.

The `ReviewRunner` interface and goroutine call update to pass `req.Prompt`:

```go
type ReviewRunner interface {
    Run(ctx context.Context, trID, model, promptKey string) (string, error)
}
// goroutine: runner.Run(ctx, job.TRID, req.Model, req.Prompt)
```

## UI (`internal/ui/templates/index.html`)

A third `<select>` appears in the form.
Order: TR-ID input → Rezensions-Stil select → Modell select → Submit button.

```html
<label for="prompt">Rezensions-Stil</label>
<select id="prompt" name="prompt">
  <option value="review_pedantic" selected>Pedantische Code-Review für erfahrene Entwickler*innen</option>
  <option value="review_appreciative">Wertschätzende Code-Review mit praktischen Tipps für Newbies</option>
  <option value="review_analytical">Technisch-Analytische Code-Review (Selbst-Konsistenz des TA)</option>
  <option value="review_guidelines_hf">Prüfung gegen HF-Entwicklungsrichtlinien</option>
</select>
```

The first option (`review_pedantic`) is pre-selected.

## Tests

### `internal/agent/runner_test.go`

- `TestAllowedPrompts_HasExpectedKeys` — asserts all 4 keys are present and each has non-empty `Label` and `Text`.
- `TestRunner_UsesSpecifiedPrompt` — stub server captures the system prompt from the request body (`body["system"].([]any)[0].(map[string]any)["text"].(string)`); asserts it equals `AllowedPrompts()["review_analytical"].Text`. Mirrors `TestRunner_UsesSpecifiedModel`.

All existing `runner.Run(ctx, trID, model)` calls in this file must be updated to the new 4-parameter signature by adding a `promptKey` argument — use any valid key, e.g. `"review_pedantic"`.
Affected tests: `TestRunner_UsesSpecifiedModel`, `TestRunner_ToolLoopAndFinalText`, `TestRunner_DispatchTools`, `TestRunner_MaxTokens_ReturnsTruncatedReview`, `TestRunner_UnexpectedStopReason_ReturnsError`.

### `internal/ui/templates_test.go`

- `TestPromptSelectOptionsMatchAllowedPrompts` — renders `index.html`, slices the HTML to the `<select id="prompt"...>...</select>` block, extracts `<option value="...">` only within that block, checks bidirectional match with `AllowedPrompts()` keys.
- `TestModelSelectOptionsMatchAllowedModels` (existing) — must be updated with the same scoping fix: slice to the `<select id="model"...>...</select>` block before extracting options, so the two selects do not cross-contaminate each other.

Both tests use the same helper: find the opening `<select id="X"`, find the next `</select>`, run the option regex only on the substring.

### `examples/aireview/handler_test.go`

New tests:
- `TestPost_UnknownPrompt_Returns400` — submits an unknown prompt key with a valid model, expects 400.
- `TestPost_EmptyPrompt_Returns400` — submits an empty prompt key with a valid model, expects 400.

Existing tests that need a `"prompt": "review_pedantic"` field added to their request body (because an absent prompt key fails validation):
- `TestPost_ValidBody_Returns200WithLink`
- `TestPost_FormEncoded_Returns200WithLink`
- `TestPost_GoroutineCallsMarkDone`
- `TestPost_UnknownModel_Returns400` — needs a valid prompt so execution reaches the model-check branch
- `TestPost_EmptyModel_Returns400` — same reason

Tests unaffected (no POST body):
- `TestPost_EmptyBody_Returns400` — `{}` fails `ShouldBind` on `TRID` before reaching prompt validation
- `TestGetStatus_*` — GET requests, no body

`fakeRunner.Run` signature updated to accept `promptKey string` as the fourth parameter.

## Out of Scope

- Free-text system prompt input (users never write system prompts).
- Custom user-turn messages (may be added later as a separate feature on top of the TR ID).
- Dynamic prompt loading at runtime (prompts are always compiled in).
