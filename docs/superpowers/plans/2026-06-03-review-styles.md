# Review Styles (Rezensions-Stile) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the single hard-coded system prompt with four selectable German review styles; users pick Rezensions-Stil + Modell + TR-ID before submitting.

**Architecture:** `AllowedPrompts() map[string]Prompt` in `internal/agent/runner.go` is the single source of truth (mirrors existing `AllowedModels()`). Each style is a `//go:embed` string var pointing to a German `.md` file. The handler validates the submitted key and passes it to `Run()`; the runner resolves key → text internally. A drift-guard test catches HTML/Go drift.

**Tech Stack:** Go `//go:embed`, Gin handler, HTMX form, existing drift-guard test pattern.

---

## File Map

| Action | Path | What changes |
|--------|------|-------------|
| Create | `internal/agent/prompts/review_pedantic.md` | New German prompt — pedantic/expert style |
| Create | `internal/agent/prompts/review_appreciative.md` | New German prompt — appreciative/newbie style |
| Create | `internal/agent/prompts/review_analytical.md` | New German prompt — analytical/self-consistency style |
| Create | `internal/agent/prompts/review_guidelines_hf.md` | New German prompt — HF guidelines style |
| Delete | `internal/agent/prompts/review_prompt.md` | Replaced by the four files above |
| Modify | `internal/agent/runner.go` | Add `Prompt` struct, 4 embed vars, `AllowedPrompts()`, extend `Run()` to 4-param |
| Modify | `internal/agent/runner_test.go` | Add 2 new tests; add 4th arg to 5 existing `runner.Run()` calls |
| Modify | `examples/aireview/handler.go` | Add `Prompt` field to `reviewRequest`, `allowedPromptKeys()`, validation block, update `ReviewRunner` interface + goroutine call |
| Modify | `examples/aireview/handler_test.go` | Add 2 new tests; add prompt field to 5 existing tests; update `fakeRunner` signature |
| Modify | `internal/ui/templates/index.html` | Add `<select id="prompt">` above model select |
| Modify | `internal/ui/templates_test.go` | Fix existing model drift-guard to scope to `#model` block; add prompt drift-guard |

`cmd/server/main.go` needs **no changes** — it passes `*agent.Runner` as `aireview.ReviewRunner` via implicit interface satisfaction; after both sides are updated to 4-param `Run()` it still compiles.

---

## Task 1: Create the four German prompt files

**Files:**
- Create: `internal/agent/prompts/review_pedantic.md`
- Create: `internal/agent/prompts/review_appreciative.md`
- Create: `internal/agent/prompts/review_analytical.md`
- Create: `internal/agent/prompts/review_guidelines_hf.md`

These must exist before Task 2 adds the `//go:embed` directives — the Go compiler rejects a missing embed target.
Do **not** delete `review_prompt.md` yet; that happens in Task 2.

- [ ] **Step 1: Create `internal/agent/prompts/review_pedantic.md`**

```markdown
# ABAP Code-Review — Pedantisch (für erfahrene Entwickler*innen)

Du bist ein sehr erfahrener ABAP-Entwickler und führst eine strenge, pedantische
Code-Review eines SAP-Transportauftrags durch.
Schreibe deine Antwort vollständig auf Deutsch.
Technische SAP-Begriffe (z.B. ABAP, ADT, ATC, PROG, CLAS, INTF, SY-SUBRC, SELECT, CATCH)
bleiben auf Englisch.

## Vorgehensweise

1. Rufe `list_tr_objects` auf, um alle Objekte im Transport zu sehen.
2. Rufe für jedes Objekt mit nicht-leerer URI `get_object_info` auf.
3. Rufe für jedes Objekt mit nicht-leerer URI `diff_active_inactive` auf.
4. Rufe `run_atc_check` einmal für ALLE nicht-leeren URIs auf (`check_variant: ""`).
5. Rufe für jedes PROG-, CLAS- und INTF-Objekt `syntax_check` auf.
6. Rufe für PROG-, CLAS- und INTF-Objekte `fetch_source` auf.
7. Rufe für CLAS-Objekte `fetch_class_includes` auf (definitions, implementations, testclasses, macros).
8. Rufe `where_used` auf, wenn Objekte weit verbreitet sein könnten.
9. Rufe `get_version_history` bei auffälligen Objekten auf.
10. Schreibe nach dem Sammeln aller Informationen ein vollständiges Review.

## Review-Kriterien (vollständig — kein Befund ist zu klein)

- **ATC-Befunde:** Alle Befunde von `run_atc_check`, gruppiert nach Objekt und Schweregrad.
- **Korrektheit:** Logikfehler, Off-by-One, unbehandelte Ausnahmen, fehlende SY-SUBRC-Prüfungen, falsche Reihenfolge von Operationen.
- **Benennung:** Z/Y-Präfix, Schreibweise, Abkürzungen, zu kurze oder nichtssagende Namen, Inkonsistenz zwischen Objekten.
- **Modularität:** Methoden mit mehr als 40 Zeilen, zu viele Aufgaben pro Methode, fehlende Extraktion in Hilfsmethoden.
- **Fehlerbehandlung:** Leere CATCH-Blöcke, fehlende MESSAGE-Anweisungen, stillschweigendes Ignorieren von Fehlern.
- **Performance:** SELECT * statt Feldliste, fehlende WHERE-Klausel, SELECT in Schleifen, wiederholte identische Datenbankabfragen.
- **Sicherheit:** Dynamisches SQL ohne Escaping, fehlende Berechtigungsprüfungen (AUTHORITY-CHECK).
- **Testbarkeit:** Klassen ohne Unit-Tests, globaler Zustand, hartcodierte Systemwerte, fehlende Dependency Injection.
- **Clean ABAP:** Verwendung veralteter Sprachkonstrukte (z.B. `FORM`/`PERFORM` statt Methoden), implizite Typkonvertierungen.
- **Auswirkung:** `where_used`-Ergebnisse für alle geänderten Schnittstellen dokumentieren.

## Ausgabeformat

# Code-Review: <Transportauftragsnummer>

## Zusammenfassung
2–3 Sätze Gesamtbewertung. Anzahl der Befunde nach Schweregrad.

## ATC-Befunde
Alle SAP-ATC-Befunde nach Objekt ("1"=Fehler, "2"=Warnung, "3"=Info).
Falls keine: „Keine ATC-Befunde."

## Befunde

### <Objektname> (<Typ>)

**[Kritisch/Schwerwiegend/Gering/Hinweis]** Präziser Titel
Beschreibung. Konkrete Empfehlung mit Codebeispiel falls sinnvoll.

## Gesamtbewertung
Ein Absatz mit klarer Empfehlung (freigeben / zurückweisen / mit Auflagen freigeben).

Formuliere präzise und direkt. Beschönige keine Befunde. Liste jeden Befund einzeln auf.
```

- [ ] **Step 2: Create `internal/agent/prompts/review_appreciative.md`**

```markdown
# ABAP Code-Review — Wertschätzend (mit praktischen Tipps für Newbies)

Du bist ein erfahrener und einfühlsamer ABAP-Mentor und führst eine konstruktive
Code-Review eines SAP-Transportauftrags durch.
Schreibe deine Antwort vollständig auf Deutsch.
Technische SAP-Begriffe (z.B. ABAP, ADT, ATC, PROG, CLAS, INTF, SY-SUBRC, SELECT, CATCH)
bleiben auf Englisch.

## Vorgehensweise

1. Rufe `list_tr_objects` auf, um alle Objekte im Transport zu sehen.
2. Rufe für jedes Objekt mit nicht-leerer URI `get_object_info` auf.
3. Rufe für jedes Objekt mit nicht-leerer URI `diff_active_inactive` auf.
4. Rufe `run_atc_check` einmal für ALLE nicht-leeren URIs auf (`check_variant: ""`).
5. Rufe für jedes PROG-, CLAS- und INTF-Objekt `syntax_check` auf.
6. Rufe für PROG-, CLAS- und INTF-Objekte `fetch_source` auf.
7. Rufe für CLAS-Objekte `fetch_class_includes` auf (definitions, implementations, testclasses, macros).
8. Rufe `where_used` auf bei Objekten mit vielen möglichen Aufrufern.
9. Schreibe nach dem Sammeln aller Informationen ein wertschätzendes Review.

## Review-Kriterien (Fokus auf die wichtigsten Punkte)

Hebe zuerst hervor, was gut gemacht wurde. Dann benenne die wichtigsten Verbesserungspunkte —
beschränke dich auf die 3–5 relevantesten Befunde, nicht jede Kleinigkeit.
Erkläre bei jedem Befund das **Warum**: Was ist das Risiko? Was lernt man daraus?

- **ATC-Befunde:** Alle Fehler (Schweregrad 1). Warnungen (2) nur wenn relevant.
- **Korrektheit:** Echte Fehler, die zur Laufzeit auftreten könnten.
- **Fehlerbehandlung:** Ausnahmen, die stillschweigend ignoriert werden.
- **Performance:** Offensichtliche Performance-Fallen (SELECT * in Schleifen).
- **Verständlichkeit:** Code, der schwer zu lesen oder zu warten ist.

## Ausgabeformat

# Code-Review: <Transportauftragsnummer>

## Das lief gut ✓
2–3 Sätze über positive Aspekte des Codes (Struktur, Lesbarkeit, Tests, …).

## ATC-Befunde
Fehler und relevante Warnungen von SAP-ATC. Falls keine: „Keine ATC-Befunde."

## Verbesserungsvorschläge

### <Objektname>

**[Wichtig/Hinweis]** Freundlicher Titel
Was ist das Problem und warum ist es wichtig?
**Empfehlung:** Konkrete, umsetzbare Verbesserung mit Beispiel.
**Hintergrund:** Kurze Erklärung des zugrunde liegenden Prinzips (optional, aber hilfreich für Newbies).

## Fazit
Ein aufmunternder Absatz: Gesamteindruck und nächste Schritte.

Formuliere freundlich, ermutigend und lehrreich. Keine überwältigende Befundliste.
```

- [ ] **Step 3: Create `internal/agent/prompts/review_analytical.md`**

```markdown
# ABAP Code-Review — Technisch-Analytisch (Selbst-Konsistenz des Transportauftrags)

Du bist ein erfahrener ABAP-Architekt und analysierst die technische Konsistenz
eines SAP-Transportauftrags als Ganzes.
Schreibe deine Antwort vollständig auf Deutsch.
Technische SAP-Begriffe (z.B. ABAP, ADT, ATC, PROG, CLAS, INTF, SY-SUBRC, SELECT, CATCH)
bleiben auf Englisch.

## Vorgehensweise

1. Rufe `list_tr_objects` auf, um alle Objekte im Transport zu sehen.
2. Rufe für ALLE Objekte mit nicht-leerer URI `get_object_info` auf — du brauchst die vollständige Objektliste für die Konsistenzanalyse.
3. Rufe für jedes Objekt `diff_active_inactive` auf, um zu sehen, was sich tatsächlich geändert hat.
4. Rufe `run_atc_check` einmal für ALLE nicht-leeren URIs auf (`check_variant: ""`).
5. Rufe für PROG-, CLAS- und INTF-Objekte `fetch_source` und `fetch_class_includes` auf.
6. Rufe `syntax_check` für alle PROG-, CLAS- und INTF-Objekte auf.
7. Rufe `where_used` für alle geänderten Schnittstellen (INTF) und Klassen (CLAS) auf.
8. Rufe `get_version_history` für Objekte auf, die in mehreren Transporten gleichzeitig geändert wurden.
9. Analysiere nun die Konsistenz des Transports als Ganzes und schreibe das Review.

## Analyse-Schwerpunkte

**Selbst-Konsistenz des Transports:**
- Sind alle Abhängigkeiten zwischen den Objekten im Transport enthalten? (z.B. eine neue Klasse, die eine neue Schnittstelle implementiert — ist die Schnittstelle auch im Transport?)
- Verwenden die Objekte konsistente Datentypen, Strukturen und Konstanten?
- Sind Namenskonventionen innerhalb des Transports einheitlich?
- Gibt es zirkuläre Abhängigkeiten zwischen den transportierten Objekten?

**Auswirkungsanalyse:**
- Welche bestehenden Objekte außerhalb des Transports werden durch die Änderungen berührt? (`where_used`)
- Sind Schnittstellen-Änderungen abwärtskompatibel?
- Gibt es Objekte im Transport, die nicht mehr referenziert werden (tote Code-Pfade)?

**Technische Qualität:**
- ATC-Befunde als Qualitäts-Gate.
- Kritische Korrektheitsfehler (SY-SUBRC, unbehandelte Ausnahmen).
- Performance-Risiken in geänderten Codepfaden.

## Ausgabeformat

# Code-Review: <Transportauftragsnummer>

## Transport-Überblick
Kurze tabellarische Übersicht der Objekte im Transport (Name, Typ, Paket, hat Änderungen).

## ATC-Befunde
SAP-ATC-Befunde nach Objekt ("1"=Fehler, "2"=Warnung, "3"=Info).
Falls keine: „Keine ATC-Befunde."

## Konsistenz-Analyse

### Abhängigkeiten
Sind alle notwendigen Abhängigkeiten im Transport enthalten? Liste fehlende Abhängigkeiten auf.

### Datentyp-Konsistenz
Werden Datentypen und Strukturen konsistent verwendet?

### Auswirkungen außerhalb des Transports
Welche externen Objekte sind betroffen? Sind Schnittstellen-Änderungen kompatibel?

## Technische Befunde

### <Objektname> (<Typ>)
**[Kritisch/Schwerwiegend/Gering]** Titel
Beschreibung und Empfehlung.

## Freigabe-Empfehlung
Klare Aussage: Kann der Transport so freigegeben werden? Welche Risiken bestehen?

Analysiere systematisch und objektiv. Der Fokus liegt auf dem Transport als Einheit, nicht auf einzelnen Objekten.
```

- [ ] **Step 4: Create `internal/agent/prompts/review_guidelines_hf.md`**

```markdown
# ABAP Code-Review — Prüfung gegen HF-Entwicklungsrichtlinien

Du bist ein erfahrener ABAP-Entwickler bei Hochfrequenz und prüfst einen
SAP-Transportauftrag auf Einhaltung der Hochfrequenz-Entwicklungsrichtlinien
und der Clean-ABAP-Prinzipien.
Schreibe deine Antwort vollständig auf Deutsch.
Technische SAP-Begriffe (z.B. ABAP, ADT, ATC, PROG, CLAS, INTF, SY-SUBRC, SELECT, CATCH)
bleiben auf Englisch.

## Vorgehensweise

1. Rufe `list_tr_objects` auf, um alle Objekte im Transport zu sehen.
2. Rufe für jedes Objekt mit nicht-leerer URI `get_object_info` auf.
3. Rufe für jedes Objekt mit nicht-leerer URI `diff_active_inactive` auf.
4. Rufe `run_atc_check` einmal für ALLE nicht-leeren URIs auf (`check_variant: ""`).
5. Rufe für jedes PROG-, CLAS- und INTF-Objekt `syntax_check` auf.
6. Rufe für PROG-, CLAS- und INTF-Objekte `fetch_source` auf.
7. Rufe für CLAS-Objekte `fetch_class_includes` auf (definitions, implementations, testclasses, macros).
8. Rufe bei Bedarf `where_used` und `get_version_history` auf.
9. Schreibe das Review mit Bezug auf konkrete Richtlinien.

## Prüfkriterien nach HF-Richtlinien und Clean ABAP

### Objektorientierung (Clean ABAP)
- Keine prozedurale Programmierung (`FORM`/`PERFORM`) — ausschließlich Methoden.
- Klassen sind entweder abstrakt oder final — keine offenen Vererbungshierarchien ohne Grund.
- Keine öffentlichen Instanzattribute — ausschließlich Getter/Setter-Methoden.
- Methoden haben einen einzigen Zweck und sind kurz (< 20 Zeilen Richtwert).

### Fehlerbehandlung (Clean ABAP)
- Ausnahmen werden mit `cx_`-Klassen geworfen, nicht mit `SY-SUBRC`-Rückgabewerten.
- Keine leeren CATCH-Blöcke.
- Fehlermeldungen sind für den Endbenutzer verständlich.

### Benennung (HF-Konventionen)
- Z-Präfix für alle kundeneigenen Entwicklungen.
- Sprechende Namen auf Englisch oder Deutsch (konsistent im Objekt).
- Keine einbuchstabigen Variablennamen außer Schleifenzähler.
- Konstantennamen in UPPER_SNAKE_CASE.

### Datenbankzugriff
- Kein `SELECT *` — nur benötigte Felder.
- Kein `SELECT` in Schleifen — stattdessen Mengenoperationen oder gepufferte Daten.
- WHERE-Klausel immer vorhanden und selektiv.
- Berechtigungsprüfung (`AUTHORITY-CHECK`) bei direktem Datenbankzugriff auf sensible Daten.

### Testbarkeit
- Alle neuen Klassen haben zugehörige Unit-Tests (lokale Testklassen oder separate ABAP Unit).
- Kein globaler Zustand in Klassen (keine Klassenattribute, die den Programmzustand speichern).
- Abhängigkeiten werden per Injection übergeben, nicht intern instanziiert.

### ATC-Befunde
- Alle ATC-Befunde Schweregrad 1 (Fehler) sind Blocker.
- ATC-Befunde Schweregrad 2 (Warnung) müssen begründet oder behoben werden.

## Ausgabeformat

# Code-Review: <Transportauftragsnummer>

## Zusammenfassung
2–3 Sätze Gesamtbewertung. Werden die HF-Richtlinien eingehalten?

## ATC-Befunde
SAP-ATC-Befunde nach Objekt ("1"=Fehler, "2"=Warnung, "3"=Info).
Falls keine: „Keine ATC-Befunde."

## Richtlinien-Prüfung

### <Objektname> (<Typ>)

**[Verletzung/Warnung/Empfehlung]** Richtlinie: <Name der Richtlinie>
Was wurde gefunden? Welche Richtlinie wird verletzt?
**Empfehlung:** Konkreter Korrekturvorschlag.

## Gesamtbewertung
Klare Aussage: Entspricht der Transport den HF-Entwicklungsrichtlinien?
Freigabe-Empfehlung (freigeben / zurückweisen / mit Auflagen).

Beziehe dich bei jedem Befund explizit auf die verletzte Richtlinie.
```

- [ ] **Step 5: Verify the files exist and are non-empty**

```powershell
Get-ChildItem internal/agent/prompts/
```

Expected: 5 files listed (`review_prompt.md` still present; 4 new ones added).

- [ ] **Step 6: Commit**

```powershell
git add internal/agent/prompts/review_pedantic.md `
        internal/agent/prompts/review_appreciative.md `
        internal/agent/prompts/review_analytical.md `
        internal/agent/prompts/review_guidelines_hf.md
git commit -m "feat: add four German review style prompt files"
```

---

## Task 2: Update `runner.go` and `runner_test.go`

**Files:**
- Modify: `internal/agent/runner.go`
- Modify: `internal/agent/runner_test.go`
- Delete: `internal/agent/prompts/review_prompt.md`

This task extends `Run()` to a 4-parameter signature and adds `AllowedPrompts()`.
All changes in this task must compile together — write new tests first, then fix compilation by updating the implementation and all call-sites.

- [ ] **Step 1: Add two new failing tests to `internal/agent/runner_test.go`**

Append after the existing `TestAllowedModels_ContainsOpusSonnetHaiku` test (around line 27):

```go
func TestAllowedPrompts_HasExpectedKeys(t *testing.T) {
	prompts := agent.AllowedPrompts()
	keys := []string{"review_pedantic", "review_appreciative", "review_analytical", "review_guidelines_hf"}
	for _, k := range keys {
		p, ok := prompts[k]
		if !ok {
			t.Errorf("AllowedPrompts must contain key %q", k)
			continue
		}
		if p.Label == "" {
			t.Errorf("AllowedPrompts[%q].Label must not be empty", k)
		}
		if p.Text == "" {
			t.Errorf("AllowedPrompts[%q].Text must not be empty", k)
		}
	}
}

func TestRunner_UsesSpecifiedPrompt(t *testing.T) {
	var capturedSystemPrompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if sys, ok := body["system"].([]any); ok && len(sys) > 0 {
			if block, ok := sys[0].(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					capturedSystemPrompt = text
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_01", "type": "message", "role": "assistant",
			"model": string(anthropic.ModelClaudeOpus4_8), "stop_reason": "end_turn",
			"content": []map[string]any{{"type": "text", "text": "Review."}},
			"usage":   map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	fake := &fakeADTClient{trObjects: nil}
	tools := agent.NewTools(fake)
	claudeClient := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
	runner := agent.NewRunner(tools, claudeClient)

	_, err := runner.Run(context.Background(), "NPLK900014", string(anthropic.ModelClaudeOpus4_8), "review_analytical")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := agent.AllowedPrompts()["review_analytical"].Text
	if capturedSystemPrompt != want {
		// min() is a builtin in Go 1.21+ (this module uses go 1.26)
		t.Errorf("wrong system prompt sent to Claude API\ngot:  %q\nwant: %q", capturedSystemPrompt[:min(80, len(capturedSystemPrompt))], want[:min(80, len(want))])
	}
}
```

- [ ] **Step 2: Try to run the new tests — confirm they fail to compile**

```powershell
go test ./internal/agent/... 2>&1 | Select-Object -First 20
```

Expected: compilation error — `agent.AllowedPrompts undefined` and `too many arguments in call to runner.Run` (once we add the 4th arg in step 1 tests).

- [ ] **Step 3: Update `internal/agent/runner.go`**

Replace the file's top section (everything before `type Runner struct`). The full replacement for lines 1–52 is:

```go
package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// Prompt pairs a German UI label with the compiled-in system prompt text.
type Prompt struct {
	Label string
	Text  string
}

// AllowedModels returns the set of model IDs the service accepts, mapped to
// a human-readable German label shown in the UI.
func AllowedModels() map[string]string {
	return map[string]string{
		string(anthropic.ModelClaudeOpus4_8):           "Opus 4.8 (beste Qualität)",
		string(anthropic.ModelClaudeSonnet4_6):         "Sonnet 4.6 (schneller, günstiger)",
		string(anthropic.ModelClaudeHaiku4_5_20251001): "Haiku 4.5 (am schnellsten &amp; günstigsten)",
	}
}

// reviewMaxTokens is the maximum output token budget for the review.
const reviewMaxTokens = int64(8192)

// reviewMaxToolLoops caps the tool-use iterations per review.
const reviewMaxToolLoops = 50

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

// Runner runs the Claude tool-use loop to produce an ABAP code review.
type Runner struct {
	tools  *Tools
	client anthropic.Client
}

// NewRunner creates a Runner with the given tools and Claude client.
func NewRunner(tools *Tools, client anthropic.Client) *Runner {
	return &Runner{tools: tools, client: client}
}

// Run calls Claude with tool access and returns the final markdown review text.
// model must be a non-empty key from AllowedModels(); promptKey must be a non-empty
// key from AllowedPrompts(). Callers are responsible for validation — Run does not
// default or substitute silently.
func (r *Runner) Run(ctx context.Context, trID, model, promptKey string) (string, error) {
	promptText := AllowedPrompts()[promptKey].Text
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(
			fmt.Sprintf("Please review transport request: %s", trID),
		)),
	}
```

Then in the `Messages.New` call, replace `Text: systemPrompt` with `Text: promptText`:

```go
		System: []anthropic.TextBlockParam{
			{
				Text:         promptText,
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			},
		},
```

The rest of `Run()` (the loop body) is unchanged **except** for one line inside the `Messages.New` call:
change `Text: systemPrompt` → `Text: promptText` (the old variable is removed; the new one is set at the top of `Run()`).
The `var systemPrompt string` embed directive (old line 34) is deleted as part of the top-section replacement above.

- [ ] **Step 4: Delete `internal/agent/prompts/review_prompt.md`**

```powershell
Remove-Item internal/agent/prompts/review_prompt.md
```

- [ ] **Step 5: Fix the 5 existing `runner.Run()` call-sites in `runner_test.go`**

Every call to `runner.Run(ctx, ...)` with 3 args needs a 4th `"review_pedantic"` argument.
The affected lines and their fixes:

| Test | Current call | Fix |
|------|-------------|-----|
| `TestRunner_UsesSpecifiedModel` (≈line 52) | `runner.Run(context.Background(), "NPLK900014", string(anthropic.ModelClaudeSonnet4_6))` | add `"review_pedantic"` |
| `TestRunner_ToolLoopAndFinalText` (≈line 118) | `runner.Run(context.Background(), "NPLK900014", "claude-opus-4-8")` | add `"review_pedantic"` |
| `TestRunner_DispatchTools` (≈line 196, inside range loop) | `runner.Run(context.Background(), "NPLK900014", "claude-opus-4-8")` | add `"review_pedantic"` |
| `TestRunner_MaxTokens_ReturnsTruncatedReview` (≈line 224) | `runner.Run(context.Background(), "NPLK900014", "claude-opus-4-8")` | add `"review_pedantic"` |
| `TestRunner_UnexpectedStopReason_ReturnsError` (≈line 250) | `runner.Run(context.Background(), "NPLK900014", "claude-opus-4-8")` | add `"review_pedantic"` |

- [ ] **Step 6: Run all agent tests — verify they pass**

```powershell
go test ./internal/agent/... -v 2>&1 | Select-Object -Last 30
```

Expected: all tests PASS including `TestAllowedPrompts_HasExpectedKeys` and `TestRunner_UsesSpecifiedPrompt`.

- [ ] **Step 7: Commit**

```powershell
git add internal/agent/runner.go internal/agent/runner_test.go
git rm internal/agent/prompts/review_prompt.md
git commit -m "feat: add AllowedPrompts() and extend Run() to accept promptKey"
```

---

## Task 3: Update handler and handler tests

**Files:**
- Modify: `examples/aireview/handler.go`
- Modify: `examples/aireview/handler_test.go`

This task adds `Prompt` to the request struct, adds validation, updates the interface, and wires the prompt key into the goroutine call.

- [ ] **Step 1: Write two new failing tests in `examples/aireview/handler_test.go`**

Append after `TestPost_EmptyModel_Returns400`:

```go
func TestPost_UnknownPrompt_Returns400(t *testing.T) {
	store := newFakeStore("00000000-0000-0000-0000-000000000096")
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	body, _ := json.Marshal(map[string]string{
		"transport_request_id": "NPLK900014",
		"model":                "claude-opus-4-8",
		"prompt":               "not-a-real-style",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/reviews", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown prompt must return 400, got %d — body: %s", w.Code, w.Body.String())
	}
}

func TestPost_EmptyPrompt_Returns400(t *testing.T) {
	store := newFakeStore("00000000-0000-0000-0000-000000000097")
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	body, _ := json.Marshal(map[string]string{
		"transport_request_id": "NPLK900014",
		"model":                "claude-opus-4-8",
		"prompt":               "",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/reviews", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty prompt must return 400, got %d — body: %s", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 2: Try to run — confirm tests fail to compile**

```powershell
go test ./examples/aireview/... 2>&1 | Select-Object -First 10
```

Expected: compilation error — `fakeRunner.Run` has wrong number of return arguments / interface mismatch after Task 2 changed `Runner.Run()`.

- [ ] **Step 3: Update `examples/aireview/handler.go`**

**3a.** Add `Prompt` to `reviewRequest` (the struct is around line 38). Keep `TransportRequestID` as the Go field name — it is used by name at `req.TransportRequestID` in the handler body. Remove `binding:"required"` from `Model` — validation for both `Model` and `Prompt` is done by the map-lookup blocks, which already reject empty strings:

```go
type reviewRequest struct {
	TransportRequestID string `json:"transport_request_id" form:"transport_request_id" binding:"required"`
	Model              string `json:"model"                form:"model"`
	Prompt             string `json:"prompt"               form:"prompt"`
}
```

**3b.** Add `allowedPromptKeys()` helper alongside the existing `allowedModelKeys()` (at the bottom of the file):

```go
func allowedPromptKeys() string {
	keys := make([]string, 0, len(agent.AllowedPrompts()))
	for k := range agent.AllowedPrompts() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
```

**3c.** Add the prompt validation block **before** the existing model validation block in `postReview`. The full pair of validation blocks becomes:

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

**3d.** Update the `ReviewRunner` interface (around line 25):

```go
type ReviewRunner interface {
	Run(ctx context.Context, trID, model, promptKey string) (string, error)
}
```

**3e.** Update the goroutine call to pass `req.Prompt`. Find the line `md, runErr := runner.Run(ctx, job.TRID, req.Model)` and change it to:

```go
md, runErr := runner.Run(ctx, job.TRID, req.Model, req.Prompt)
```

- [ ] **Step 4: Update `fakeRunner` in `examples/aireview/handler_test.go`**

Change the `fakeRunner.Run` signature from 3 string params to 4:

```go
type fakeRunner struct{}

func (f *fakeRunner) Run(_ context.Context, _, _, _ string) (string, error) {
	return "# Review\n\nAll good.", nil
}
```

- [ ] **Step 5: Add `"prompt": "review_pedantic"` to the 5 existing happy-path tests**

Each of these tests builds a JSON body or form body without a `prompt` field; they will now fail the prompt validation and return 400. Add the field:

| Test | What to change |
|------|---------------|
| `TestPost_ValidBody_Returns200WithLink` | Add `"prompt": "review_pedantic"` to `json.Marshal(map[string]string{...})` |
| `TestPost_FormEncoded_Returns200WithLink` | Append `&prompt=review_pedantic` to the form string |
| `TestPost_GoroutineCallsMarkDone` | Add `"prompt": "review_pedantic"` to `json.Marshal(map[string]string{...})` |
| `TestPost_UnknownModel_Returns400` | Add `"prompt": "review_pedantic"` so execution reaches the model-check branch |
| `TestPost_EmptyModel_Returns400` | Add `"prompt": "review_pedantic"` so execution reaches the model-check branch |

- [ ] **Step 6: Run all handler tests — verify they pass**

```powershell
go test ./examples/aireview/... -v 2>&1 | Select-Object -Last 30
```

Expected: all tests PASS including `TestPost_UnknownPrompt_Returns400` and `TestPost_EmptyPrompt_Returns400`.

- [ ] **Step 7: Run the full test suite**

```powershell
go test ./... 2>&1 | Select-Object -Last 20
```

Expected: all packages PASS. No compilation errors.

- [ ] **Step 8: Commit**

```powershell
git add examples/aireview/handler.go examples/aireview/handler_test.go
git commit -m "feat: add prompt validation and ReviewRunner interface update"
```

---

## Task 4: Update templates and drift-guard tests

**Files:**
- Modify: `internal/ui/templates/index.html`
- Modify: `internal/ui/templates_test.go`

This task adds the `<select id="prompt">` to the form and hardens both drift-guard tests to scope their regex to the correct `<select>` block.

- [ ] **Step 1: Fix the existing `TestModelSelectOptionsMatchAllowedModels` test**

The current implementation (around line 107) uses an unanchored regex that scans the entire rendered HTML. Once two `<select>` elements exist, model options and prompt options would cross-contaminate. Replace the body of `TestModelSelectOptionsMatchAllowedModels` with the scoped version, and extract a helper:

In `internal/ui/templates_test.go`, add this helper function and rewrite the existing test:

```go
// selectOptions extracts <option value="..."> entries from a single named <select> block.
func selectOptions(t *testing.T, html, selectID string) map[string]bool {
	t.Helper()
	open := `<select id="` + selectID + `"`
	start := strings.Index(html, open)
	if start == -1 {
		t.Fatalf("no <select id=%q> found in rendered HTML", selectID)
	}
	end := strings.Index(html[start:], "</select>")
	if end == -1 {
		t.Fatalf("no </select> closing tag found after <select id=%q>", selectID)
	}
	block := html[start : start+end]
	re := regexp.MustCompile(`<option value="([^"]+)"`)
	matches := re.FindAllStringSubmatch(block, -1)
	values := make(map[string]bool)
	for _, m := range matches {
		if len(m) > 1 {
			values[m[1]] = true
		}
	}
	return values
}

func TestModelSelectOptionsMatchAllowedModels(t *testing.T) {
	tmpl := ui.MustLoadTemplates()
	html, err := tmpl.RenderIndex()
	if err != nil {
		t.Fatalf("RenderIndex: %v", err)
	}
	htmlValues := selectOptions(t, html, "model")
	allowed := agent.AllowedModels()
	for modelID := range allowed {
		if !htmlValues[modelID] {
			t.Errorf("AllowedModels key %q has no matching <option value> in #model select", modelID)
		}
	}
	for htmlVal := range htmlValues {
		if _, ok := allowed[htmlVal]; !ok {
			t.Errorf("#model select has <option value=%q> which is not in AllowedModels()", htmlVal)
		}
	}
}
```

Also add the new prompt drift-guard test:

```go
func TestPromptSelectOptionsMatchAllowedPrompts(t *testing.T) {
	tmpl := ui.MustLoadTemplates()
	html, err := tmpl.RenderIndex()
	if err != nil {
		t.Fatalf("RenderIndex: %v", err)
	}
	htmlValues := selectOptions(t, html, "prompt")
	allowed := agent.AllowedPrompts()
	for promptID := range allowed {
		if !htmlValues[promptID] {
			t.Errorf("AllowedPrompts key %q has no matching <option value> in #prompt select", promptID)
		}
	}
	for htmlVal := range htmlValues {
		if _, ok := allowed[htmlVal]; !ok {
			t.Errorf("#prompt select has <option value=%q> which is not in AllowedPrompts()", htmlVal)
		}
	}
}
```

- [ ] **Step 2: Run templates tests — confirm the new drift-guard fails and the model test still passes**

```powershell
go test ./internal/ui/... -v -run "TestModelSelectOptionsMatchAllowedModels|TestPromptSelectOptionsMatchAllowedPrompts" 2>&1
```

Expected:
- `TestModelSelectOptionsMatchAllowedModels` — PASS (model select already exists)
- `TestPromptSelectOptionsMatchAllowedPrompts` — FAIL (`no <select id="prompt"> found`)

- [ ] **Step 3: Add `<select id="prompt">` to `internal/ui/templates/index.html`**

Inside the `<form>`, between the TR-ID input block (`</div>` closing the suggestions div) and the existing `<label for="model">`, insert:

```html
    <label for="prompt">Rezensions-Stil</label>
    <select id="prompt" name="prompt">
      <option value="review_pedantic" selected>Pedantische Code-Review für erfahrene Entwickler*innen</option>
      <option value="review_appreciative">Wertschätzende Code-Review mit praktischen Tipps für Newbies</option>
      <option value="review_analytical">Technisch-Analytische Code-Review (Selbst-Konsistenz des TA)</option>
      <option value="review_guidelines_hf">Prüfung gegen HF-Entwicklungsrichtlinien</option>
    </select>
```

The form order becomes: TR-ID input → Rezensions-Stil select → Modell select → Submit button.

- [ ] **Step 4: Run templates tests — verify both drift-guards pass**

```powershell
go test ./internal/ui/... -v 2>&1 | Select-Object -Last 20
```

Expected: all tests PASS.

- [ ] **Step 5: Run the full test suite one final time**

```powershell
go test ./... 2>&1
```

Expected: all packages PASS, zero failures.

- [ ] **Step 6: Commit**

```powershell
git add internal/ui/templates/index.html internal/ui/templates_test.go
git commit -m "feat: add Rezensions-Stil dropdown and scoped drift-guard tests"
```

---

## Done

After all four tasks, push to a feature branch and open a PR:

```powershell
git push origin HEAD
gh pr create --title "feat: selectable review styles (Rezensions-Stile)" --body "$(cat <<'EOF'
## Summary
- Replaces single hard-coded system prompt with 4 selectable German review styles
- AllowedPrompts() mirrors AllowedModels() as single source of truth
- Handler validates prompt key → 400 on unknown/empty
- UI: new Rezensions-Stil dropdown (TR-ID → Stil → Modell → Submit)
- Drift-guard tests for both selects, scoped to avoid cross-contamination

## Test plan
- [ ] `go test ./...` green
- [ ] Open the deployed service, verify dropdown shows 4 styles
- [ ] Submit a review with each style, confirm Claude responds in German

🤖 Generated with Claude Code
EOF
)"
```
