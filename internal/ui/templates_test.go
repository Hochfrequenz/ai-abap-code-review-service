// internal/ui/templates_test.go
package ui_test

import (
	"strings"
	"testing"
	"time"

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
