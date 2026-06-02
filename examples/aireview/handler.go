package aireview

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/hochfrequenz/ai-abap-code-review-service/internal/btp"
	"github.com/hochfrequenz/ai-abap-code-review-service/internal/reviewstore"
	"github.com/hochfrequenz/ai-abap-code-review-service/internal/ui"
)

// ReviewRunner is the interface the handler uses to start a review.
// Satisfied by *agent.Runner in production and fakeRunner in tests.
type ReviewRunner interface {
	Run(ctx context.Context, trID string) (string, error)
}

type reviewRequest struct {
	TransportRequestID string `json:"transport_request_id" binding:"required"`
}

// Register attaches the two aireview routes to the JWT-guarded api group.
// rootCtx must be the server's root context (not a request context) so the
// goroutine continues after the HTTP response is written.
func Register(api *gin.RouterGroup, rootCtx context.Context, store reviewstore.JobStore, runner ReviewRunner, tmpl ui.Templates) {
	api.POST("/reviews", postReview(rootCtx, store, runner, tmpl))
	api.GET("/reviews/:id/status", getStatus(store, tmpl))
}

func postReview(rootCtx context.Context, store reviewstore.JobStore, runner ReviewRunner, _ ui.Templates) gin.HandlerFunc {
	return func(c *gin.Context) {
		// tmpl is unused: POST response is a hardcoded HTML bootstrap fragment
		var req reviewRequest
		if err := c.ShouldBindJSON(&req); err != nil {
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
			`<p>Review started — <a href="/reviews/%s">view results</a></p>`+
				`<div hx-get="/api/reviews/%s/status" hx-trigger="every 3s" hx-swap="outerHTML">⏳ Starting…</div>`,
			job.ID, job.ID,
		)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(fragment))
	}
}

func getStatus(store reviewstore.JobStore, tmpl ui.Templates) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		job, err := store.Get(c.Request.Context(), id)
		if err != nil {
			btp.AbortError(c, http.StatusNotFound, btp.CodeNotFound, "review not found", err)
			return
		}

		html, err := tmpl.RenderStatus(job)
		if err != nil {
			btp.AbortError(c, http.StatusInternalServerError, btp.CodeInternal, "render failed", err)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
	}
}
