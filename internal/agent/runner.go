package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// Prompt pairs a German UI label with the compiled-in system prompt text.
type Prompt struct {
	Label string
	Text  string
}

// AllowedModels returns the set of model IDs the service accepts, mapped to
// a human-readable German label shown in the UI.
func AllowedModels() map[string]string {
	return map[string]string{
		string(anthropic.ModelClaudeOpus4_8):           "Opus 4.8 (beste Qualität)",
		string(anthropic.ModelClaudeSonnet4_6):         "Sonnet 4.6 (schneller, günstiger)",
		string(anthropic.ModelClaudeHaiku4_5_20251001): "Haiku 4.5 (am schnellsten &amp; günstigsten)",
	}
}

// reviewMaxTokens is the maximum output token budget for the review.
const reviewMaxTokens = int64(8192)

// reviewMaxToolLoops caps the tool-use iterations per review.
const reviewMaxToolLoops = 50

//go:embed prompts/review_pedantic.md
var promptPedantic string

//go:embed prompts/review_appreciative.md
var promptAppreciative string

//go:embed prompts/review_analytical.md
var promptAnalytical string

//go:embed prompts/review_guidelines_hf.md
var promptGuidelinesHF string

// AllowedPrompts returns the set of review styles the service accepts,
// mapped to their German UI label and compiled-in system prompt text.
func AllowedPrompts() map[string]Prompt {
	return map[string]Prompt{
		"review_pedantic":      {Label: "Pedantische Code-Review für erfahrene Entwickler*innen", Text: promptPedantic},
		"review_appreciative":  {Label: "Wertschätzende Code-Review mit praktischen Tipps für Newbies", Text: promptAppreciative},
		"review_analytical":    {Label: "Technisch-Analytische Code-Review (Selbst-Konsistenz des TA)", Text: promptAnalytical},
		"review_guidelines_hf": {Label: "Prüfung gegen HF-Entwicklungsrichtlinien", Text: promptGuidelinesHF},
	}
}

// Runner runs the Claude tool-use loop to produce an ABAP code review.
type Runner struct {
	tools  *Tools
	client anthropic.Client
}

// NewRunner creates a Runner with the given tools and Claude client.
// The client is passed by value because anthropic.NewClient returns a value type.
func NewRunner(tools *Tools, client anthropic.Client) *Runner {
	return &Runner{tools: tools, client: client}
}

// Run calls Claude with tool access, letting it autonomously fetch TR objects
// and source code, then returns the final markdown review text.
// model must be a non-empty key from AllowedModels(); promptKey must be a non-empty
// key from AllowedPrompts(). Callers are responsible for validation — Run does not
// default or substitute silently.
func (r *Runner) Run(ctx context.Context, trID, model, promptKey string) (string, error) {
	promptText := AllowedPrompts()[promptKey].Text
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(
			fmt.Sprintf("Please review transport request: %s", trID),
		)),
	}

	toolDefs := r.buildToolDefs()

	for range reviewMaxToolLoops {
		resp, err := r.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(model),
			MaxTokens: reviewMaxTokens,
			System: []anthropic.TextBlockParam{
				{
					Text:         promptText,
					CacheControl: anthropic.NewCacheControlEphemeralParam(),
				},
			},
			Tools:    toolDefs,
			Messages: messages,
		})
		if err != nil {
			return "", fmt.Errorf("claude api: %w", err)
		}

		messages = append(messages, resp.ToParam())

		if resp.StopReason == anthropic.StopReasonEndTurn || resp.StopReason == "max_tokens" {
			for _, block := range resp.Content {
				if block.Type == "text" {
					text := block.Text
					if resp.StopReason == "max_tokens" {
						text += "\n\n---\n*Review truncated: output token limit reached.*"
					}
					return text, nil
				}
			}
			return "", fmt.Errorf("no text block in response (stop_reason: %s)", resp.StopReason)
		}

		if resp.StopReason != anthropic.StopReasonToolUse {
			return "", fmt.Errorf("unexpected stop_reason: %s", resp.StopReason)
		}

		var toolResults []anthropic.ContentBlockParamUnion
		for _, block := range resp.Content {
			if block.Type != "tool_use" {
				continue
			}
			result, callErr := r.dispatch(ctx, block.Name, block.Input)
			if callErr != nil {
				result = fmt.Sprintf("error: %v", callErr)
			}
			toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, result, callErr != nil))
		}
		if len(toolResults) == 0 {
			return "", fmt.Errorf("stop_reason tool_use but no tool_use blocks in response")
		}
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}
	return "", fmt.Errorf("review did not complete within %d tool-use iterations", reviewMaxToolLoops)
}

// dispatch routes a tool call by name to the appropriate handler.
// Adding a new tool: implement a handle* method and register it in toolHandlers.
func (r *Runner) dispatch(ctx context.Context, toolName string, input json.RawMessage) (string, error) {
	h, ok := r.toolHandlers(ctx)[toolName]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
	return h(input)
}

// toolHandlers returns a map of tool name → handler closed over ctx.
// Each handler unmarshals its specific args, calls the tool, and marshals the result.
func (r *Runner) toolHandlers(ctx context.Context) map[string]func(json.RawMessage) (string, error) {
	return map[string]func(json.RawMessage) (string, error){
		"list_tr_objects":      r.handleListTRObjects(ctx),
		"fetch_source":         r.handleFetchSource(ctx),
		"fetch_class_includes": r.handleFetchClassIncludes(ctx),
		"syntax_check":         r.handleSyntaxCheck(ctx),
		"get_object_info":      r.handleGetObjectInfo(ctx),
		"get_version_history":  r.handleGetVersionHistory(ctx),
		"where_used":           r.handleWhereUsed(ctx),
		"diff_active_inactive": r.handleDiffActiveInactive(ctx),
		"run_atc_check":        r.handleRunATCCheck(ctx),
	}
}

func marshalResult(v any) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal tool result: %w", err)
	}
	return string(out), nil
}

func (r *Runner) handleListTRObjects(ctx context.Context) func(json.RawMessage) (string, error) {
	return func(input json.RawMessage) (string, error) {
		var args struct {
			TransportRequestID string `json:"transport_request_id"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		objs, err := r.tools.ListTRObjects(ctx, args.TransportRequestID)
		if err != nil {
			return "", err
		}
		return marshalResult(objs)
	}
}

func (r *Runner) handleFetchSource(ctx context.Context) func(json.RawMessage) (string, error) {
	return func(input json.RawMessage) (string, error) {
		var args struct {
			ObjectURI string `json:"object_uri"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		return r.tools.FetchSource(ctx, args.ObjectURI)
	}
}

func (r *Runner) handleFetchClassIncludes(ctx context.Context) func(json.RawMessage) (string, error) {
	return func(input json.RawMessage) (string, error) {
		var args struct {
			ClassURI string `json:"class_uri"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		includes, err := r.tools.FetchClassIncludes(ctx, args.ClassURI)
		if err != nil {
			return "", err
		}
		return marshalResult(includes)
	}
}

func (r *Runner) handleSyntaxCheck(ctx context.Context) func(json.RawMessage) (string, error) {
	return func(input json.RawMessage) (string, error) {
		var args struct {
			ObjectURI string `json:"object_uri"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		msgs, err := r.tools.SyntaxCheck(ctx, args.ObjectURI)
		if err != nil {
			return "", err
		}
		return marshalResult(msgs)
	}
}

func (r *Runner) handleGetObjectInfo(ctx context.Context) func(json.RawMessage) (string, error) {
	return func(input json.RawMessage) (string, error) {
		var args struct {
			ObjectURI string `json:"object_uri"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		info, err := r.tools.GetObjectInfo(ctx, args.ObjectURI)
		if err != nil {
			return "", err
		}
		return marshalResult(info)
	}
}

func (r *Runner) handleGetVersionHistory(ctx context.Context) func(json.RawMessage) (string, error) {
	return func(input json.RawMessage) (string, error) {
		var args struct {
			ObjectURI string `json:"object_uri"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		hist, err := r.tools.GetVersionHistory(ctx, args.ObjectURI)
		if err != nil {
			return "", err
		}
		return marshalResult(hist)
	}
}

func (r *Runner) handleWhereUsed(ctx context.Context) func(json.RawMessage) (string, error) {
	return func(input json.RawMessage) (string, error) {
		var args struct {
			ObjectURI string `json:"object_uri"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		callers, err := r.tools.WhereUsed(ctx, args.ObjectURI)
		if err != nil {
			return "", err
		}
		return marshalResult(callers)
	}
}

func (r *Runner) handleDiffActiveInactive(ctx context.Context) func(json.RawMessage) (string, error) {
	return func(input json.RawMessage) (string, error) {
		var args struct {
			ObjectURI string `json:"object_uri"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		diff, err := r.tools.DiffActiveInactive(ctx, args.ObjectURI)
		if err != nil {
			return "", err
		}
		return marshalResult(diff)
	}
}

func (r *Runner) handleRunATCCheck(ctx context.Context) func(json.RawMessage) (string, error) {
	return func(input json.RawMessage) (string, error) {
		var args struct {
			ObjectURIs   []string `json:"object_uris"`
			CheckVariant string   `json:"check_variant"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		if len(args.ObjectURIs) == 0 {
			return "[]", nil // nothing to check
		}
		result, err := r.tools.RunATCCheck(ctx, args.ObjectURIs, args.CheckVariant)
		if err != nil {
			return "", err
		}
		return marshalResult(result)
	}
}

func (r *Runner) buildToolDefs() []anthropic.ToolUnionParam {
	return []anthropic.ToolUnionParam{
		{
			OfTool: &anthropic.ToolParam{
				Name:        "list_tr_objects",
				Description: anthropic.String("List all objects in a SAP transport request. Returns objects with their ADT URIs. Objects with empty URI are unsupported types — skip them."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]any{
						"transport_request_id": map[string]any{"type": "string", "description": "The transport request number, e.g. NPLK900014"},
					},
					Required: []string{"transport_request_id"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "fetch_source",
				Description: anthropic.String("Fetch the main ABAP source code for an object using its ADT URI."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]any{
						"object_uri": map[string]any{"type": "string", "description": "The ADT URI of the object, e.g. /sap/bc/adt/oo/classes/zcl_example"},
					},
					Required: []string{"object_uri"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "fetch_class_includes",
				Description: anthropic.String("Fetch all available include sections of an ABAP class (definitions, implementations, testclasses, macros). Returns a map of include name to source code."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]any{
						"class_uri": map[string]any{"type": "string", "description": "The ADT URI of the class, e.g. /sap/bc/adt/oo/classes/zcl_example"},
					},
					Required: []string{"class_uri"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "syntax_check",
				Description: anthropic.String("Run an ADT syntax check on a saved ABAP object. Returns a list of syntax errors, warnings, and info messages with line/column positions. An empty list means no issues."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]any{
						"object_uri": map[string]any{"type": "string", "description": "ADT URI of the object to check"},
					},
					Required: []string{"object_uri"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "get_object_info",
				Description: anthropic.String("Get metadata for an ABAP object: type, name, description, and package. Useful for understanding what an object is before reading its source."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]any{
						"object_uri": map[string]any{"type": "string", "description": "ADT URI of the object"},
					},
					Required: []string{"object_uri"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "get_version_history",
				Description: anthropic.String("Get the version history of an ABAP object: who changed it, when, and in which transport. Provides context for the code review (e.g. recent churn, author patterns)."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]any{
						"object_uri": map[string]any{"type": "string", "description": "ADT URI of the object"},
					},
					Required: []string{"object_uri"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "where_used",
				Description: anthropic.String("Find all ABAP objects that reference the given object (callers, users). Useful for impact analysis: understanding how many callers depend on a changed interface or class."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]any{
						"object_uri": map[string]any{"type": "string", "description": "ADT URI of the object to analyse"},
					},
					Required: []string{"object_uri"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "diff_active_inactive",
				Description: anthropic.String("Show the diff between the active (released) version and the inactive (pending/unsaved) version of an ABAP object. HasChanges=false means the object has no pending edits. Use this to focus the review on what actually changed."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]any{
						"object_uri": map[string]any{"type": "string", "description": "ADT URI of the object"},
					},
					Required: []string{"object_uri"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "run_atc_check",
				Description: anthropic.String("Run SAP's ATC (ABAP Test Cockpit) static analysis on one or more objects. Returns prioritised findings (priority field: \"1\"=error, \"2\"=warning, \"3\"=info — string values, not integers) with check name and message. This is SAP's own quality gate — use it on all objects in the transport before writing the review."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]any{
						"object_uris":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of ADT URIs to check (PROG/CLAS/INTF only; skip empty URIs)"},
						"check_variant": map[string]any{"type": "string", "description": "ATC check variant name; pass empty string to use the system default"},
					},
					Required: []string{"object_uris", "check_variant"},
				},
			},
		},
	}
}
