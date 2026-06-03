package aireview

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/gin-gonic/gin"

	"github.com/hochfrequenz/ai-abap-code-review-service/internal/agent"
	"github.com/hochfrequenz/ai-abap-code-review-service/internal/btp"
	"github.com/hochfrequenz/ai-abap-code-review-service/internal/reviewstore"
	"github.com/hochfrequenz/ai-abap-code-review-service/internal/ui"
)

// ReviewRunner is the single point of coupling between the HTTP handler and the
// AI backend. Swap the implementation in cmd/server/main.go to replace Claude
// with a different AI provider (e.g. OpenAI, Gemini) without touching the
// handler or any other layer.
// model must be a non-empty key from agent.AllowedModels(); promptKey must be a non-empty
// key from agent.AllowedPrompts(). Empty string is rejected with 400.
type ReviewRunner interface {
	// Preflight checks the transport request before any AI tokens are spent.
	// Returns a German user-facing error if the TR is unreachable or has no reviewable objects.
	Preflight(ctx context.Context, trID string) error
	Run(ctx context.Context, trID, model, promptKey string) (string, error)
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
	// Model is a Claude model ID from agent.AllowedModels().
	// The form <select> always submits a value; direct API calls must supply one.
	Model string `json:"model" form:"model"`
	// Prompt is the review style key from agent.AllowedPrompts().
	Prompt string `json:"prompt" form:"prompt"`
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

		// Prompt is required — must be a key from agent.AllowedPrompts().
		// No silent defaulting: the form always submits a value via the <select>.
		if _, ok := agent.AllowedPrompts()[req.Prompt]; !ok {
			btp.AbortError(c, http.StatusBadRequest, btp.CodeInvalidRequest,
				fmt.Sprintf("Rezensions-Stil unbekannt %q — erlaubt: %s", req.Prompt, allowedPromptKeys()), nil)
			return
		}
		// Model is required — must be a key from agent.AllowedModels().
		// No silent defaulting: the form always submits a value via the <select>.
		if _, ok := agent.AllowedModels()[req.Model]; !ok {
			btp.AbortError(c, http.StatusBadRequest, btp.CodeInvalidRequest,
				fmt.Sprintf("Modell fehlt oder unbekannt %q — erlaubt: %s", req.Model, allowedModelKeys()), nil)
			return
		}

		job, err := store.Create(c.Request.Context(), req.TransportRequestID)
		if err != nil {
			btp.AbortError(c, http.StatusInternalServerError, btp.CodeInternal, "failed to create review job", err)
			return
		}

		// Use context.WithoutCancel so the goroutine outlives the HTTP response.
		go func(ctx context.Context, jobID, trID, model, promptKey string) {
			_ = store.MarkRunning(ctx, jobID)
			if err := runner.Preflight(ctx, trID); err != nil {
				_ = store.MarkFailed(ctx, jobID, err.Error())
				return
			}
			md, runErr := runner.Run(ctx, trID, model, promptKey)
			if runErr != nil {
				_ = store.MarkFailed(ctx, jobID, runErr.Error())
				return
			}
			_ = store.MarkDone(ctx, jobID, md)
		}(context.WithoutCancel(rootCtx), job.ID, job.TRID, req.Model, req.Prompt)

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

// openTR is the JSON representation of a transport request sent to the browser.
// The browser-side JS filters these client-side by number, owner, and description.
type openTR struct {
	Number      string `json:"number"`
	Owner       string `json:"owner"`
	Description string `json:"description"`
}

// getTransportRequests returns all open (modifiable) transport requests as a JSON
// array, sorted by number descending (newest first). The browser fetches this once
// on page load and filters client-side — no round-trips needed while the user types.
// On ADT error it returns an empty JSON array so the form stays usable.
func getTransportRequests(lister TransportRequestLister) gin.HandlerFunc {
	return func(c *gin.Context) {
		if lister == nil {
			c.JSON(http.StatusOK, []openTR{})
			return
		}
		// Empty user = all users' open TRs ("D" = modifiable/open only).
		// Implementation uses RunQuery on E070/E07T instead of the ADT transport
		// organizer tree endpoint: the HF S/4 system uses KORRDEV="SYST"/"CUST"
		// for its requests, which the organizer tree ignores (it only handles "K").
		trs, err := lister.GetTransportRequests(c.Request.Context(), "", "D")
		if err != nil {
			slog.InfoContext(c.Request.Context(), "transport-requests fetch failed", "err", err)
			c.JSON(http.StatusOK, []openTR{})
			return
		}
		sort.SliceStable(trs, func(i, j int) bool { return trs[i].Number > trs[j].Number })
		result := make([]openTR, 0, len(trs))
		for _, tr := range trs {
			result = append(result, openTR{
				Number:      tr.Number,
				Owner:       tr.Owner,
				Description: tr.Description,
			})
		}
		c.JSON(http.StatusOK, result)
	}
}

func allowedModelKeys() string {
	keys := make([]string, 0, len(agent.AllowedModels()))
	for k := range agent.AllowedModels() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

func allowedPromptKeys() string {
	keys := make([]string, 0, len(agent.AllowedPrompts()))
	for k := range agent.AllowedPrompts() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
