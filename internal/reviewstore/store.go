// internal/reviewstore/store.go
package reviewstore

import (
	"context"
	"time"
)

type JobStatus string

const (
	JobStatusPending JobStatus = "pending"
	JobStatusRunning JobStatus = "running"
	JobStatusDone    JobStatus = "done"
	JobStatusFailed  JobStatus = "failed"
)

// TokenUsage holds the Claude API token counts for one review and an estimated cost.
// Costs are approximate — prices change; treat as a rough guide, not an invoice.
type TokenUsage struct {
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	EstimatedCostUSD    float64
}

type Job struct {
	ID         string
	TRID       string
	Status     JobStatus
	ReviewHTML string
	ErrMsg     string
	Usage      TokenUsage
	CreatedAt  time.Time
	FinishedAt *time.Time
}

type JobStore interface {
	Create(ctx context.Context, trID string) (*Job, error)
	Get(ctx context.Context, id string) (*Job, error)
	MarkRunning(ctx context.Context, id string) error
	MarkDone(ctx context.Context, id string, reviewMarkdown string, usage TokenUsage) error
	MarkFailed(ctx context.Context, id string, errMsg string) error
}
