// internal/reviewstore/memory.go
package reviewstore

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
)

type memoryStore struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

func NewMemoryStore() JobStore {
	return &memoryStore{jobs: make(map[string]*Job)}
}

func (s *memoryStore) Create(_ context.Context, trID string) (*Job, error) {
	job := &Job{
		ID:        uuid.New().String(),
		TRID:      trID,
		Status:    JobStatusPending,
		CreatedAt: time.Now(),
	}
	s.mu.Lock()
	s.jobs[job.ID] = job
	cp := *job // copy before unlock — callers must not alias the stored pointer
	s.mu.Unlock()
	return &cp, nil
}

func (s *memoryStore) Get(_ context.Context, id string) (*Job, error) {
	s.mu.RLock()
	job, ok := s.jobs[id]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("job %q not found", id)
	}
	cp := *job
	if job.FinishedAt != nil {
		t := *job.FinishedAt
		cp.FinishedAt = &t
	}
	return &cp, nil
}

func (s *memoryStore) MarkRunning(_ context.Context, id string) error {
	return s.update(id, func(j *Job) { j.Status = JobStatusRunning })
}

func (s *memoryStore) MarkDone(_ context.Context, id string, reviewMarkdown string) error {
	html, err := renderMarkdown(reviewMarkdown)
	if err != nil {
		return err
	}
	now := time.Now()
	return s.update(id, func(j *Job) {
		j.Status = JobStatusDone
		j.ReviewHTML = html
		j.FinishedAt = &now
	})
}

func (s *memoryStore) MarkFailed(_ context.Context, id string, errMsg string) error {
	now := time.Now()
	return s.update(id, func(j *Job) {
		j.Status = JobStatusFailed
		j.ErrMsg = errMsg
		j.FinishedAt = &now
	})
}

func (s *memoryStore) update(id string, fn func(*Job)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return fmt.Errorf("job %q not found", id)
	}
	fn(job)
	return nil
}

func renderMarkdown(md string) (string, error) {
	var buf bytes.Buffer
	// html.WithUnsafe allows Claude-generated HTML (e.g. tables) to pass through.
	// Input is from the Claude API, not from an untrusted caller.
	mdRenderer := goldmark.New(
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)
	if err := mdRenderer.Convert([]byte(md), &buf); err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	return buf.String(), nil
}
