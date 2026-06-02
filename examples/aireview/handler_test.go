package aireview_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hochfrequenz/go-sap-btp-cf-template/examples/aireview"
	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/reviewstore"
	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/ui"
)

type fakeStore struct {
	job    *reviewstore.Job
	doneCh chan string
	getErr error
}

func newFakeStore(jobID string) *fakeStore {
	return &fakeStore{
		job: &reviewstore.Job{
			ID:        jobID,
			TRID:      "NPLK000001",
			Status:    reviewstore.JobStatusPending,
			CreatedAt: time.Now(),
		},
		doneCh: make(chan string, 1),
	}
}

func (f *fakeStore) Create(_ context.Context, trID string) (*reviewstore.Job, error) {
	f.job.TRID = trID
	return f.job, nil
}
func (f *fakeStore) Get(_ context.Context, _ string) (*reviewstore.Job, error) {
	return f.job, f.getErr
}
func (f *fakeStore) MarkRunning(_ context.Context, _ string) error { return nil }
func (f *fakeStore) MarkDone(_ context.Context, _ string, html string) error {
	f.job.Status = reviewstore.JobStatusDone
	f.job.ReviewHTML = html
	f.doneCh <- html
	return nil
}
func (f *fakeStore) MarkFailed(_ context.Context, _ string, errMsg string) error {
	f.job.Status = reviewstore.JobStatusFailed
	f.job.ErrMsg = errMsg
	return nil
}

type fakeRunner struct{}

func (f *fakeRunner) Run(_ context.Context, _ string) (string, error) {
	return "# Review\n\nAll good.", nil
}

func newRouter(store reviewstore.JobStore, runner aireview.ReviewRunner, tmpl ui.Templates) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	aireview.Register(api, context.Background(), store, runner, tmpl)
	return r
}

func TestPost_ValidBody_Returns200WithLink(t *testing.T) {
	store := newFakeStore("test-uuid-1")
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	body, _ := json.Marshal(map[string]string{"transport_request_id": "NPLK900014"})
	req := httptest.NewRequest(http.MethodPost, "/api/reviews", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type: got %q, want text/html", ct)
	}
	if !strings.Contains(w.Body.String(), "/reviews/") {
		t.Errorf("response should contain review link, got: %s", w.Body.String())
	}
}

func TestPost_EmptyBody_Returns400(t *testing.T) {
	store := newFakeStore("test-uuid-2")
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	req := httptest.NewRequest(http.MethodPost, "/api/reviews", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", w.Code)
	}
}

func TestPost_GoroutineCallsMarkDone(t *testing.T) {
	store := newFakeStore("test-uuid-3")
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	body, _ := json.Marshal(map[string]string{"transport_request_id": "NPLK900014"})
	req := httptest.NewRequest(http.MethodPost, "/api/reviews", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	select {
	case html := <-store.doneCh:
		if !strings.Contains(html, "Review") {
			t.Errorf("MarkDone called with unexpected HTML: %q", html)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for MarkDone to be called")
	}
}

func TestGetStatus_Pending_HasPolling(t *testing.T) {
	store := newFakeStore("test-uuid-4")
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	req := httptest.NewRequest(http.MethodGet, "/api/reviews/test-uuid-4/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `hx-trigger="every 3s"`) {
		t.Error("pending response must contain hx-trigger")
	}
	if !strings.Contains(body, `/api/reviews/test-uuid-4/status`) {
		t.Error("pending response must contain correct hx-get URL")
	}
}

func TestGetStatus_Done_HasContentNoPoll(t *testing.T) {
	store := newFakeStore("test-uuid-5")
	store.job.Status = reviewstore.JobStatusDone
	store.job.ReviewHTML = "<p>LGTM</p>"
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	req := httptest.NewRequest(http.MethodGet, "/api/reviews/test-uuid-5/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if strings.Contains(body, "hx-trigger") {
		t.Error("done response must not poll")
	}
	if !strings.Contains(body, "LGTM") {
		t.Errorf("done response must contain ReviewHTML, got: %s", body)
	}
}

func TestGetStatus_UnknownID_Returns404(t *testing.T) {
	store := newFakeStore("test-uuid-6")
	store.getErr = fmt.Errorf("job not found")
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	req := httptest.NewRequest(http.MethodGet, "/api/reviews/no-such-id/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", w.Code)
	}
}
