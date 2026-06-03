package aireview

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/gin-gonic/gin"

	"github.com/hochfrequenz/ai-abap-code-review-service/internal/btp"
	"github.com/hochfrequenz/ai-abap-code-review-service/internal/reviewstore"
	"github.com/hochfrequenz/ai-abap-code-review-service/internal/ui"
)

// ReviewRunner is the single point of coupling between the HTTP handler and the
// AI backend. Swap the implementation in cmd/server/main.go to replace Claude
// with a different AI provider (e.g. OpenAI, Gemini) without touching the
// handler or any other layer.
type ReviewRunner interface {
	Run(ctx context.Context, trID string) (string, error)
}

// TransportRequestLister retrieves open CTS transport requests from SAP ADT.
// Satisfied by adtler.Client in production; use a one-method fake in tests.
type TransportRequestLister interface {
	GetTransportRequests(ctx context.Context, user, status string) ([]adt.TransportRequest, error)
}

type reviewRequest struct {
	// TransportRequestID is a SAP CTS transport request number.
	// Format: 2-letter system prefix + K + 6 digits, all uppercase — e.g. NPLK900014.
	// form tag covers HTMX's default application/x-www-form-urlencoded submissions;
	// json tag covers direct API calls with Content-Type: application/json.
	TransportRequestID string `json:"transport_request_id" form:"transport_request_id" binding:"required,uppercase,min=9,max=10"`
}

const contentTypeHTML = "text/html; charset=utf-8"

// Register attaches the aireview routes to the JWT-guarded api group.
// rootCtx must be the server's root context (not a request context) so the
// goroutine continues after the HTTP response is written.
func Register(api *gin.RouterGroup, rootCtx context.Context, store reviewstore.JobStore, runner ReviewRunner, lister TransportRequestLister, tmpl ui.Templates) {
	api.POST("/reviews", postReview(rootCtx, store, runner, tmpl))
	api.GET("/reviews/:id/status", getStatus(store, tmpl))
	api.GET("/transport-requests", getTransportRequests(lister))
}

func postReview(rootCtx context.Context, store reviewstore.JobStore, runner ReviewRunner, _ ui.Templates) gin.HandlerFunc {
	return func(c *gin.Context) {
		// tmpl is unused: POST response is a hardcoded HTML bootstrap fragment
		var req reviewRequest
		// ShouldBind auto-detects content type: handles both form-encoded (HTMX default)
		// and JSON (direct API calls).
		if err := c.ShouldBind(&req); err != nil {
			btp.AbortError(c, http.StatusBadRequest, btp.CodeInvalidRequest, "transport_request_id is required", nil)
			return
		}

		job, err := store.Create(c.Request.Context(), req.TransportRequestID)
		if err != nil {
			btp.AbortError(c, http.StatusInternalServerError, btp.CodeInternal, "failed to create review job", err)
			return
		}

		// Use context.WithoutCancel so the goroutine outlives the HTTP response.
		go func(ctx context.Context, jobID, trID string) {
			_ = store.MarkRunning(ctx, jobID)
			md, runErr := runner.Run(ctx, trID)
			if runErr != nil {
				_ = store.MarkFailed(ctx, jobID, runErr.Error())
				return
			}
			_ = store.MarkDone(ctx, jobID, md)
		}(context.WithoutCancel(rootCtx), job.ID, job.TRID)

		fragment := fmt.Sprintf(
			`<p>Review gestartet — <a href="/reviews/%s">Ergebnisse anzeigen</a></p>`+
				`<div hx-get="/api/reviews/%s/status" hx-trigger="every 3s" hx-swap="outerHTML">⏳ Wird gestartet…</div>`,
			job.ID, job.ID,
		)
		c.Data(http.StatusOK, contentTypeHTML, []byte(fragment))
	}
}

func getStatus(store reviewstore.JobStore, tmpl ui.Templates) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		job, err := store.Get(c.Request.Context(), id)
		if err != nil {
			// store.Get returns an error only when the id is not found — the in-memory
			// store never fails for any other reason. A persistent store implementation
			// should map "not found" to a sentinel error and propagate other errors as
			// CodeInternal.
			btp.AbortError(c, http.StatusNotFound, btp.CodeNotFound, "review not found", err)
			return
		}

		html, err := tmpl.RenderStatus(job)
		if err != nil {
			btp.AbortError(c, http.StatusInternalServerError, btp.CodeInternal, "render failed", err)
			return
		}
		c.Data(http.StatusOK, contentTypeHTML, []byte(html))
	}
}

// getTransportRequests returns open (modifiable) transport requests as HTML
// <option> elements for a <datalist>, sorted by number descending (most recent first).
// On ADT error it returns an empty 200 so the form stays usable.
func getTransportRequests(lister TransportRequestLister) gin.HandlerFunc {
	return func(c *gin.Context) {
		if lister == nil {
			c.Data(http.StatusOK, contentTypeHTML, nil)
			return
		}
		// DEBUG rc10: return hardcoded option to test if HTMX datalist works at all.
		// If this shows in the browser dropdown, the issue is backend (SAP auth/XML).
		// If it doesn't show, the issue is frontend (HTMX/datalist).
		c.Data(http.StatusOK, contentTypeHTML, []byte(`<option value="TESTK900001">TESTK900001 — Hardcoded Test TR (TestUser)</option>`))
		return
		// nolint:govet — unreachable code intentional for debug
		trs, err := lister.GetTransportRequests(c.Request.Context(), "", "D")
		if err != nil {
			slog.InfoContext(c.Request.Context(), "transport-requests fetch failed", "err", err)
			c.Data(http.StatusOK, contentTypeHTML, nil)
			return
		}
		sort.SliceStable(trs, func(i, j int) bool { return trs[i].Number > trs[j].Number })
		var b strings.Builder
		for _, tr := range trs {
			fmt.Fprintf(&b, "<option value=\"%s\">%s — %s (%s)</option>\n",
				html.EscapeString(tr.Number),
				html.EscapeString(tr.Number),
				html.EscapeString(tr.Description),
				html.EscapeString(tr.Owner))
		}
		c.Data(http.StatusOK, contentTypeHTML, []byte(b.String()))
	}
}
