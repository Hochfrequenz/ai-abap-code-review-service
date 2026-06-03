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

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/gin-gonic/gin"
	"github.com/hochfrequenz/ai-abap-code-review-service/examples/aireview"
	"github.com/hochfrequenz/ai-abap-code-review-service/internal/reviewstore"
	"github.com/hochfrequenz/ai-abap-code-review-service/internal/ui"
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
func (f *fakeStore) MarkDone(_ context.Context, _ string, md string) error {
	f.job.Status = reviewstore.JobStatusDone
	f.job.ReviewHTML = md
	f.doneCh <- md
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

type fakeTransportRequestLister struct {
	requests []adt.TransportRequest
	err      error
}

func (f *fakeTransportRequestLister) GetTransportRequests(_ context.Context, _, _ string) ([]adt.TransportRequest, error) {
	return f.requests, f.err
}

func newRouterWithLister(store reviewstore.JobStore, runner aireview.ReviewRunner, lister aireview.TransportRequestLister, tmpl ui.Templates) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	aireview.Register(api, context.Background(), store, runner, lister, tmpl)
	return r
}

func newRouter(store reviewstore.JobStore, runner aireview.ReviewRunner, tmpl ui.Templates) *gin.Engine {
	return newRouterWithLister(store, runner, nil, tmpl)
}

func TestPost_ValidBody_Returns200WithLink(t *testing.T) {
	store := newFakeStore("00000000-0000-0000-0000-000000000001")
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

func TestPost_FormEncoded_Returns200WithLink(t *testing.T) {
	store := newFakeStore("00000000-0000-0000-0000-000000000099")
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	// HTMX submits forms as application/x-www-form-urlencoded by default.
	req := httptest.NewRequest(http.MethodPost, "/api/reviews",
		strings.NewReader("transport_request_id=NPLK900014"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "/reviews/") {
		t.Errorf("response should contain review link, got: %s", w.Body.String())
	}
}

func TestPost_EmptyBody_Returns400(t *testing.T) {
	store := newFakeStore("00000000-0000-0000-0000-000000000002")
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
	store := newFakeStore("00000000-0000-0000-0000-000000000003")
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
	store := newFakeStore("00000000-0000-0000-0000-000000000004")
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	req := httptest.NewRequest(http.MethodGet, "/api/reviews/00000000-0000-0000-0000-000000000004/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `hx-trigger="every 3s"`) {
		t.Error("pending response must contain hx-trigger")
	}
	if !strings.Contains(body, `/api/reviews/00000000-0000-0000-0000-000000000004/status`) {
		t.Error("pending response must contain correct hx-get URL")
	}
}

func TestGetStatus_Done_HasContentNoPoll(t *testing.T) {
	store := newFakeStore("00000000-0000-0000-0000-000000000005")
	store.job.Status = reviewstore.JobStatusDone
	store.job.ReviewHTML = "<p>LGTM</p>"
	tmpl := ui.MustLoadTemplates()
	r := newRouter(store, &fakeRunner{}, tmpl)

	req := httptest.NewRequest(http.MethodGet, "/api/reviews/00000000-0000-0000-0000-000000000005/status", nil)
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
	store := newFakeStore("00000000-0000-0000-0000-000000000006")
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

// openTR mirrors the JSON shape the handler sends to the browser.
type openTR struct {
	Number      string `json:"number"`
	Owner       string `json:"owner"`
	Description string `json:"description"`
}

func TestGetTransportRequests_ReturnsJSONSortedDescending(t *testing.T) {
	lister := &fakeTransportRequestLister{
		requests: []adt.TransportRequest{
			{Number: "NPLK900001", Description: "Old TR", Owner: "USER1"},
			{Number: "NPLK900014", Description: "New TR", Owner: "USER2"},
			{Number: "NPLK900007", Description: "Mid TR", Owner: "USER1"},
		},
	}
	store := newFakeStore("00000000-0000-0000-0000-000000000010")
	tmpl := ui.MustLoadTemplates()
	r := newRouterWithLister(store, &fakeRunner{}, lister, tmpl)

	req := httptest.NewRequest(http.MethodGet, "/api/transport-requests", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}
	var result []openTR
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v — body: %s", err, w.Body.String())
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result))
	}
	// Descending by number: 014 first, 007 second, 001 last.
	if result[0].Number != "NPLK900014" || result[1].Number != "NPLK900007" || result[2].Number != "NPLK900001" {
		t.Errorf("wrong sort order: %v", result)
	}
	// Owner and description must be present.
	if result[0].Owner != "USER2" || result[0].Description != "New TR" {
		t.Errorf("owner/description wrong: %+v", result[0])
	}
}

func TestGetTransportRequests_JSONSafeSpecialChars(t *testing.T) {
	lister := &fakeTransportRequestLister{
		requests: []adt.TransportRequest{
			{Number: "NPLK900014", Description: `TR & <fix> "bug"`, Owner: `U<S>ER`},
		},
	}
	store := newFakeStore("00000000-0000-0000-0000-000000000013")
	tmpl := ui.MustLoadTemplates()
	r := newRouterWithLister(store, &fakeRunner{}, lister, tmpl)

	req := httptest.NewRequest(http.MethodGet, "/api/transport-requests", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var result []openTR
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	// JSON unmarshaling should correctly decode the special characters.
	if result[0].Description != `TR & <fix> "bug"` {
		t.Errorf("description mangled: %q", result[0].Description)
	}
	if result[0].Owner != `U<S>ER` {
		t.Errorf("owner mangled: %q", result[0].Owner)
	}
}

func TestGetTransportRequests_ADTError_ReturnsEmptyArray(t *testing.T) {
	lister := &fakeTransportRequestLister{err: fmt.Errorf("ADT unreachable")}
	store := newFakeStore("00000000-0000-0000-0000-000000000011")
	tmpl := ui.MustLoadTemplates()
	r := newRouterWithLister(store, &fakeRunner{}, lister, tmpl)

	req := httptest.NewRequest(http.MethodGet, "/api/transport-requests", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("ADT error must not bubble as non-200, got %d", w.Code)
	}
	var result []openTR
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("expected valid JSON on error, got: %s", w.Body.String())
	}
	if len(result) != 0 {
		t.Errorf("expected empty array on ADT error, got %d items", len(result))
	}
}

func TestGetTransportRequests_NilLister_ReturnsEmptyArray(t *testing.T) {
	store := newFakeStore("00000000-0000-0000-0000-000000000012")
	tmpl := ui.MustLoadTemplates()
	r := newRouterWithLister(store, &fakeRunner{}, nil, tmpl)

	req := httptest.NewRequest(http.MethodGet, "/api/transport-requests", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("nil lister must return 200, got %d", w.Code)
	}
	var result []openTR
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("expected valid JSON for nil lister, got: %s", w.Body.String())
	}
	if len(result) != 0 {
		t.Errorf("nil lister must return empty array, got %d items", len(result))
	}
}
