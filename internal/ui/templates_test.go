// internal/ui/templates_test.go
package ui_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hochfrequenz/ai-abap-code-review-service/internal/agent"
	"github.com/hochfrequenz/ai-abap-code-review-service/internal/reviewstore"
	"github.com/hochfrequenz/ai-abap-code-review-service/internal/ui"
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

func TestRenderIndex_NoError(t *testing.T) {
	tmpl := ui.MustLoadTemplates()
	out, err := tmpl.RenderIndex()
	if err != nil {
		t.Fatalf("RenderIndex: %v", err)
	}
	if !strings.Contains(out, "hx-post") {
		t.Error("index page must contain HTMX form")
	}
}

func TestRenderReview_ContainsTRID(t *testing.T) {
	tmpl := ui.MustLoadTemplates()
	out, err := tmpl.RenderReview(doneJob())
	if err != nil {
		t.Fatalf("RenderReview: %v", err)
	}
	if !strings.Contains(out, "NPLK900014") {
		t.Error("review page must contain TRID")
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

// TestModelSelectOptionsMatchAllowedModels guards against drift between the
// hardcoded <option value="..."> entries in the #model select and agent.AllowedModels().
// If a model is added to AllowedModels() but not the HTML (or vice versa), this test fails.
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

// TestPromptSelectOptionsMatchAllowedPrompts guards against drift between the
// hardcoded <option value="..."> entries in the #prompt select and agent.AllowedPrompts().
// If a prompt is added to AllowedPrompts() but not the HTML (or vice versa), this test fails.
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
