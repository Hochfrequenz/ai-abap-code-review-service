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

type Job struct {
	ID         string
	TRID       string
	Status     JobStatus
	ReviewHTML string
	ErrMsg     string
	CreatedAt  time.Time
	FinishedAt *time.Time
}

type JobStore interface {
	Create(ctx context.Context, trID string) (*Job, error)
	Get(ctx context.Context, id string) (*Job, error)
	MarkRunning(ctx context.Context, id string) error
	MarkDone(ctx context.Context, id string, reviewHTML string) error
	MarkFailed(ctx context.Context, id string, errMsg string) error
}
