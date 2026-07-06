// internal/reviewstore/memory_test.go
package reviewstore_test

import (
	"context"
	"strings"
	"testing"

	"github.com/hochfrequenz/ai-abap-code-review-service/internal/reviewstore"
)

func TestCreate_ReturnsJobWithPendingStatus(t *testing.T) {
	store := reviewstore.NewMemoryStore()
	job, err := store.Create(context.Background(), reviewstore.JobMeta{
		TRID:        "NPLK900014",
		TRTitle:     "Fix invoice rounding",
		TRAuthor:    "JDOE",
		ModelLabel:  "Opus 4.8 (beste Qualität, >1€/Review)",
		PromptLabel: "Pedantische Code-Review",
		UserComment: "Bitte auf 2 Nachkommastellen runden.",
	})
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
	// Metadata must round-trip through Create.
	if job.TRTitle != "Fix invoice rounding" {
		t.Errorf("TRTitle: got %q", job.TRTitle)
	}
	if job.TRAuthor != "JDOE" {
		t.Errorf("TRAuthor: got %q", job.TRAuthor)
	}
	if job.ModelLabel != "Opus 4.8 (beste Qualität, >1€/Review)" {
		t.Errorf("ModelLabel: got %q", job.ModelLabel)
	}
	if job.PromptLabel != "Pedantische Code-Review" {
		t.Errorf("PromptLabel: got %q", job.PromptLabel)
	}
	if job.UserComment != "Bitte auf 2 Nachkommastellen runden." {
		t.Errorf("UserComment: got %q", job.UserComment)
	}
}

// metadata round-trip must also survive Get (stored copy carries the fields).
func TestCreateThenGet_PreservesMetadata(t *testing.T) {
	store := reviewstore.NewMemoryStore()
	job, _ := store.Create(context.Background(), reviewstore.JobMeta{
		TRID: "TR100", TRTitle: "Title", TRAuthor: "AUTH",
		ModelLabel: "ModelX", PromptLabel: "StyleY", UserComment: "CommentZ",
	})
	got, err := store.Get(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.TRTitle != "Title" || got.TRAuthor != "AUTH" || got.ModelLabel != "ModelX" || got.PromptLabel != "StyleY" || got.UserComment != "CommentZ" {
		t.Errorf("metadata not preserved on Get: %+v", got)
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
	job, _ := store.Create(context.Background(), reviewstore.JobMeta{TRID: "TR001"})

	err := store.MarkDone(context.Background(), job.ID, "# Review\n\nLooks good.", reviewstore.TokenUsage{})
	if err != nil {
		t.Fatalf("MarkDone: %v", err)
	}

	got, _ := store.Get(context.Background(), job.ID)
	if got.Status != reviewstore.JobStatusDone {
		t.Errorf("Status: got %q, want done", got.Status)
	}
	if !strings.Contains(got.ReviewHTML, "<h1>") {
		t.Errorf("ReviewHTML missing expected <h1> tag, got: %q", got.ReviewHTML)
	}
	if got.FinishedAt == nil {
		t.Error("FinishedAt should be set")
	}
}

func TestMarkFailed_StoresErrMsg(t *testing.T) {
	store := reviewstore.NewMemoryStore()
	job, _ := store.Create(context.Background(), reviewstore.JobMeta{TRID: "TR002"})

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
	job, _ := store.Create(context.Background(), reviewstore.JobMeta{TRID: "TR003"})
	_ = store.MarkRunning(context.Background(), job.ID)
	got, _ := store.Get(context.Background(), job.ID)
	if got.Status != reviewstore.JobStatusRunning {
		t.Errorf("Status: got %q, want running", got.Status)
	}
}
