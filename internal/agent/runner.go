package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

//go:embed prompts/review_prompt.md
var systemPrompt string

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
func (r *Runner) Run(ctx context.Context, trID string) (string, error) {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(
			fmt.Sprintf("Please review transport request: %s", trID),
		)),
	}

	toolDefs := r.buildToolDefs()

	for {
		resp, err := r.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaudeOpus4_8,
			MaxTokens: 8192,
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Tools:    toolDefs,
			Messages: messages,
		})
		if err != nil {
			return "", fmt.Errorf("claude api: %w", err)
		}

		messages = append(messages, resp.ToParam())

		if resp.StopReason == anthropic.StopReasonEndTurn {
			for _, block := range resp.Content {
				if block.Type == "text" {
					return block.Text, nil
				}
			}
			return "", fmt.Errorf("end_turn but no text block in response")
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
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}
}

func (r *Runner) dispatch(ctx context.Context, toolName string, input json.RawMessage) (string, error) {
	switch toolName {
	case "list_tr_objects":
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
		out, _ := json.Marshal(objs)
		return string(out), nil

	case "fetch_source":
		var args struct {
			ObjectURI string `json:"object_uri"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		return r.tools.FetchSource(ctx, args.ObjectURI)

	case "fetch_class_includes":
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
		out, _ := json.Marshal(includes)
		return string(out), nil

	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
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
	}
}
