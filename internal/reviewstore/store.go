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

// Job is one review job and its result. TRTitle, TRAuthor, ModelLabel,
// PromptLabel and UserComment are display-only metadata captured at creation
// time (see JobMeta) and rendered in the review header; ModelLabel is plain
// text (not HTML-encoded).
type Job struct {
	ID          string
	TRID        string
	TRTitle     string
	TRAuthor    string
	ModelLabel  string
	PromptLabel string
	UserComment string
	Status      JobStatus
	ReviewHTML  string
	ErrMsg      string
	Usage       TokenUsage
	CreatedAt   time.Time
	FinishedAt  *time.Time
}

// JobMeta carries the creation-time metadata for a review job: the transport
// request under review and the settings chosen for it. Only TRID is required;
// the remaining fields are display-only (rendered in the review header) and may
// be empty — e.g. TRTitle/TRAuthor are looked up client-side and absent when the
// typed TR number is not in the browser's loaded list. UserComment is free text
// the submitter typed (e.g. acceptance criteria); it is also fed to the LLM as
// review context — see agent.Runner.Run.
type JobMeta struct {
	TRID        string
	TRTitle     string
	TRAuthor    string
	ModelLabel  string
	PromptLabel string
	UserComment string
}

type JobStore interface {
	Create(ctx context.Context, meta JobMeta) (*Job, error)
	Get(ctx context.Context, id string) (*Job, error)
	MarkRunning(ctx context.Context, id string) error
	MarkDone(ctx context.Context, id string, reviewMarkdown string, usage TokenUsage) error
	MarkFailed(ctx context.Context, id string, errMsg string) error
}
