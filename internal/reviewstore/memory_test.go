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
	job, err := store.Create(context.Background(), "NPLK900014")
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
	job, _ := store.Create(context.Background(), "TR001")

	err := store.MarkDone(context.Background(), job.ID, "# Review\n\nLooks good.")
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
	job, _ := store.Create(context.Background(), "TR002")

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
	job, _ := store.Create(context.Background(), "TR003")
	_ = store.MarkRunning(context.Background(), job.ID)
	got, _ := store.Get(context.Background(), job.ID)
	if got.Status != reviewstore.JobStatusRunning {
		t.Errorf("Status: got %q, want running", got.Status)
	}
}
