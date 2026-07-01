package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/hochfrequenz/ai-abap-code-review-service/internal/agent"
)

func TestAllowedModels_ContainsOpusSonnetHaiku(t *testing.T) {
	models := agent.AllowedModels()
	if _, ok := models[string(anthropic.ModelClaudeOpus4_8)]; !ok {
		t.Error("AllowedModels must contain Opus 4.8")
	}
	if _, ok := models[string(anthropic.ModelClaudeSonnet4_6)]; !ok {
		t.Error("AllowedModels must contain Sonnet 4.6")
	}
	if len(models) < 2 {
		t.Errorf("expected at least 2 models, got %d", len(models))
	}
}

func TestRunner_UsesSpecifiedModel(t *testing.T) {
	var capturedModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if m, ok := body["model"].(string); ok {
			capturedModel = m
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_01", "type": "message", "role": "assistant",
			"model": body["model"], "stop_reason": "end_turn",
			"content": []map[string]any{{"type": "text", "text": "Review."}},
			"usage":   map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	fake := &fakeADTClient{trObjects: nil}
	tools := agent.NewTools(fake)
	claudeClient := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
	runner := agent.NewRunner(tools, claudeClient)

	_, _, err := runner.Run(context.Background(), "NPLK900014", string(anthropic.ModelClaudeSonnet4_6), "review_pedantic", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedModel != string(anthropic.ModelClaudeSonnet4_6) {
		t.Errorf("expected model %q, got %q", anthropic.ModelClaudeSonnet4_6, capturedModel)
	}
}

func TestRunner_ToolLoopAndFinalText(t *testing.T) {
	var calls []string
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		if callCount == 1 {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "msg_01",
				"type":        "message",
				"role":        "assistant",
				"model":       "claude-opus-4-8",
				"stop_reason": "tool_use",
				"content": []map[string]any{
					{
						"type":  "tool_use",
						"id":    "tool_01",
						"name":  "list_tr_objects",
						"input": map[string]any{"transport_request_id": "NPLK900014"},
					},
				},
				"usage": map[string]any{"input_tokens": 10, "output_tokens": 5},
			})
			calls = append(calls, "list_tr_objects")
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "msg_02",
			"type":        "message",
			"role":        "assistant",
			"model":       "claude-opus-4-8",
			"stop_reason": "end_turn",
			"content": []map[string]any{
				{"type": "text", "text": "# Code Review\n\nAll good."},
			},
			"usage": map[string]any{"input_tokens": 20, "output_tokens": 15},
		})
	}))
	defer srv.Close()

	fake := &fakeADTClient{trObjects: nil}
	tools := agent.NewTools(fake)

	claudeClient := anthropic.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test-key"),
	)

	runner := agent.NewRunner(tools, claudeClient)
	result, usage, err := runner.Run(context.Background(), "NPLK900014", "claude-opus-4-8", "review_pedantic", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(result, "Code Review") {
		t.Errorf("expected review text in result, got: %q", result)
	}
	// Token counts must accumulate across both loop iterations: 10+20=30 input, 5+15=20 output.
	if usage.InputTokens != 30 {
		t.Errorf("InputTokens: want 30, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 20 {
		t.Errorf("OutputTokens: want 20, got %d", usage.OutputTokens)
	}
	// Cost: (30*15 + 20*75) / 1_000_000 = (450 + 1500) / 1_000_000 = 0.00195
	if usage.EstimatedCostUSD < 0.001 || usage.EstimatedCostUSD > 0.01 {
		t.Errorf("EstimatedCostUSD out of expected range: %f", usage.EstimatedCostUSD)
	}
	if len(calls) != 1 || calls[0] != "list_tr_objects" {
		t.Errorf("expected list_tr_objects call, got: %v", calls)
	}
}

func TestRunner_DispatchTools(t *testing.T) {
	tests := []struct {
		name      string
		toolName  string
		toolInput map[string]any
		sources   map[string]string
	}{
		{
			name:      "fetch_source",
			toolName:  "fetch_source",
			toolInput: map[string]any{"object_uri": "/sap/bc/adt/oo/classes/zcl_foo"},
			sources:   map[string]string{"/sap/bc/adt/oo/classes/zcl_foo": "CLASS zcl_foo DEFINITION."},
		},
		{
			name:      "fetch_class_includes",
			toolName:  "fetch_class_includes",
			toolInput: map[string]any{"class_uri": "/sap/bc/adt/oo/classes/zcl_foo"},
			sources:   map[string]string{"/sap/bc/adt/oo/classes/zcl_foo/definitions": "DEFINITION."},
		},
		{
			name:      "syntax_check",
			toolName:  "syntax_check",
			toolInput: map[string]any{"object_uri": "/sap/bc/adt/oo/classes/zcl_foo"},
			sources:   map[string]string{}, // syntax_check doesn't use sources
		},
		{
			name:     "run_atc_check",
			toolName: "run_atc_check",
			toolInput: map[string]any{
				"object_uris":   []string{"/sap/bc/adt/oo/classes/zcl_foo"},
				"check_variant": "",
			},
			sources: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callCount++
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewDecoder(r.Body).Decode(&map[string]any{})
				if callCount == 1 {
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id": "msg_01", "type": "message", "role": "assistant",
						"model": "claude-opus-4-8", "stop_reason": "tool_use",
						"content": []map[string]any{{
							"type": "tool_use", "id": "t1", "name": tt.toolName, "input": tt.toolInput,
						}},
						"usage": map[string]any{"input_tokens": 10, "output_tokens": 5},
					})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": "msg_02", "type": "message", "role": "assistant",
					"model": "claude-opus-4-8", "stop_reason": "end_turn",
					"content": []map[string]any{{"type": "text", "text": "# Review\n\nLooks good."}},
					"usage":   map[string]any{"input_tokens": 20, "output_tokens": 10},
				})
			}))
			defer srv.Close()

			fake := &fakeADTClient{sources: tt.sources}
			tools := agent.NewTools(fake)
			claudeClient := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test"))
			runner := agent.NewRunner(tools, claudeClient)
			result, _, err := runner.Run(context.Background(), "NPLK900014", "claude-opus-4-8", "review_pedantic", "")
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if !strings.Contains(result, "Review") {
				t.Errorf("expected review, got: %q", result)
			}
		})
	}
}

func TestRunner_MaxTokens_ReturnsTruncatedReview(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewDecoder(r.Body).Decode(&map[string]any{})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_01", "type": "message", "role": "assistant",
			"model": "claude-opus-4-8", "stop_reason": "max_tokens",
			"content": []map[string]any{{"type": "text", "text": "# Partial Review"}},
			"usage":   map[string]any{"input_tokens": 100, "output_tokens": 8192},
		})
	}))
	defer srv.Close()

	fake := &fakeADTClient{trObjects: nil}
	tools := agent.NewTools(fake)
	claudeClient := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test"))
	runner := agent.NewRunner(tools, claudeClient)
	result, _, err := runner.Run(context.Background(), "NPLK900014", "claude-opus-4-8", "review_pedantic", "")
	if err != nil {
		t.Fatalf("expected partial result not error, got: %v", err)
	}
	if !strings.Contains(result, "truncated") {
		t.Errorf("expected truncation note, got: %q", result)
	}
}

func TestRunner_ConcatenatesTextBlocksBeforePreambleStripping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewDecoder(r.Body).Decode(&map[string]any{})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_01", "type": "message", "role": "assistant",
			"model": "claude-opus-4-8", "stop_reason": "end_turn",
			"content": []map[string]any{
				{"type": "text", "text": "Ich habe nun alle Quelltexte gesammelt."},
				{"type": "text", "text": "\n\n## Zusammenfassung\nAlles gut."},
			},
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	fake := &fakeADTClient{trObjects: nil}
	tools := agent.NewTools(fake)
	claudeClient := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test"))
	runner := agent.NewRunner(tools, claudeClient)

	result, _, err := runner.Run(context.Background(), "NPLK900014", "claude-opus-4-8", "review_pedantic", "")
	if err != nil {
		t.Fatalf("expected result not error, got: %v", err)
	}
	if result != "## Zusammenfassung\nAlles gut." {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestRunner_UnexpectedStopReason_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewDecoder(r.Body).Decode(&map[string]any{})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_01", "type": "message", "role": "assistant",
			"model": "claude-opus-4-8", "stop_reason": "stop_sequence",
			"content": []map[string]any{},
			"usage":   map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	fake := &fakeADTClient{trObjects: nil}
	tools := agent.NewTools(fake)
	claudeClient := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test"))
	runner := agent.NewRunner(tools, claudeClient)
	_, _, err := runner.Run(context.Background(), "NPLK900014", "claude-opus-4-8", "review_pedantic", "")
	if err == nil {
		t.Error("expected error for unexpected stop reason")
	}
}

func TestRunner_Preflight_ADTError(t *testing.T) {
	fake := &fakeADTClient{trErr: errors.New("connection refused")}
	tools := agent.NewTools(fake)
	runner := agent.NewRunner(tools, anthropic.NewClient(option.WithAPIKey("test")))

	err := runner.Preflight(context.Background(), "NPLK000001")
	if err == nil {
		t.Fatal("expected error when ADT is unreachable, got nil")
	}
}

func TestRunner_Preflight_EmptyTR(t *testing.T) {
	fake := &fakeADTClient{
		trObjects:   []adt.TransportObject{},
		queryResult: &adt.QueryResult{Rows: [][]string{}},
	}
	tools := agent.NewTools(fake)
	runner := agent.NewRunner(tools, anthropic.NewClient(option.WithAPIKey("test")))

	err := runner.Preflight(context.Background(), "NPLK000001")
	if err == nil {
		t.Fatal("expected error for empty TR, got nil")
	}
}

func TestRunner_Preflight_AllEmptyURIs_TruelyEmpty(t *testing.T) {
	// ADT returns objects but none have URIs, and E071 also has no rows â†’ truly empty.
	fake := &fakeADTClient{
		trObjects:   []adt.TransportObject{{PgmID: "R3TR", Type: "TABU", Name: "T001"}},
		queryResult: &adt.QueryResult{Rows: [][]string{}},
	}
	tools := agent.NewTools(fake)
	runner := agent.NewRunner(tools, anthropic.NewClient(option.WithAPIKey("test")))

	err := runner.Preflight(context.Background(), "NPLK000001")
	if err == nil {
		t.Fatal("expected error when all URIs are empty and E071 empty, got nil")
	}
}

func TestRunner_Preflight_SystTransport(t *testing.T) {
	// ADT returns nothing (SYST type), but E071 has rows â†’ inform user, don't claim empty.
	fake := &fakeADTClient{
		trObjects:   []adt.TransportObject{},
		queryResult: &adt.QueryResult{Rows: [][]string{{"NPLK000001"}}},
	}
	tools := agent.NewTools(fake)
	runner := agent.NewRunner(tools, anthropic.NewClient(option.WithAPIKey("test")))

	err := runner.Preflight(context.Background(), "NPLK000001")
	if err == nil {
		t.Fatal("expected error for SYST transport, got nil")
	}
	if !strings.Contains(err.Error(), "SYST") {
		t.Errorf("error should mention SYST, got: %v", err)
	}
}

func TestRunner_Preflight_EmptyTR_E071AlsoEmpty(t *testing.T) {
	// ADT returns nothing and E071 is also empty â†’ transport has no objects at all.
	fake := &fakeADTClient{
		trObjects:   []adt.TransportObject{},
		queryResult: &adt.QueryResult{Rows: [][]string{}},
	}
	tools := agent.NewTools(fake)
	runner := agent.NewRunner(tools, anthropic.NewClient(option.WithAPIKey("test")))

	err := runner.Preflight(context.Background(), "NPLK000001")
	if err == nil {
		t.Fatal("expected error for empty TR, got nil")
	}
}

func TestRunner_Preflight_HasReviewableObject(t *testing.T) {
	fake := &fakeADTClient{trObjects: []adt.TransportObject{
		{PgmID: "R3TR", Type: "TABU", Name: "T001"},
		{PgmID: "R3TR", Type: "CLAS", Name: "ZCL_EXAMPLE"},
	}}
	tools := agent.NewTools(fake)
	runner := agent.NewRunner(tools, anthropic.NewClient(option.WithAPIKey("test")))

	err := runner.Preflight(context.Background(), "NPLK000001")
	if err != nil {
		t.Fatalf("expected nil for TR with reviewable objects, got: %v", err)
	}
}

func TestAllowedPrompts_HasExpectedKeys(t *testing.T) {
	prompts := agent.AllowedPrompts()
	keys := []string{"review_pedantic", "review_appreciative", "review_analytical", "review_guidelines_hf", "review_clean_abap"}
	for _, k := range keys {
		p, ok := prompts[k]
		if !ok {
			t.Errorf("AllowedPrompts must contain key %q", k)
			continue
		}
		if p.Label == "" {
			t.Errorf("AllowedPrompts[%q].Label must not be empty", k)
		}
		if p.Text == "" {
			t.Errorf("AllowedPrompts[%q].Text must not be empty", k)
		}
	}
}

func TestRunner_UsesSpecifiedPrompt(t *testing.T) {
	var capturedSystemPrompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if sys, ok := body["system"].([]any); ok && len(sys) > 0 {
			if block, ok := sys[0].(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					capturedSystemPrompt = text
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_01", "type": "message", "role": "assistant",
			"model": string(anthropic.ModelClaudeOpus4_8), "stop_reason": "end_turn",
			"content": []map[string]any{{"type": "text", "text": "Review."}},
			"usage":   map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	fake := &fakeADTClient{trObjects: nil}
	tools := agent.NewTools(fake)
	claudeClient := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
	runner := agent.NewRunner(tools, claudeClient)

	_, _, err := runner.Run(context.Background(), "NPLK900014", string(anthropic.ModelClaudeOpus4_8), "review_analytical", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := agent.AllowedPrompts()["review_analytical"].Text
	if capturedSystemPrompt != want {
		// min() is a builtin in Go 1.21+ (this module uses go 1.26)
		t.Errorf("wrong system prompt sent to Claude API\ngot:  %q\nwant: %q", capturedSystemPrompt[:min(80, len(capturedSystemPrompt))], want[:min(80, len(want))])
	}
}

// firstUserMessageText extracts the text of the first content block of the
// first message in a decoded Claude API request body — messages[0].content[0].text.
func firstUserMessageText(t *testing.T, body map[string]any) string {
	t.Helper()
	messages, ok := body["messages"].([]any)
	if !ok || len(messages) == 0 {
		t.Fatalf("no messages in request body: %v", body)
	}
	msg, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("messages[0] is not an object: %v", messages[0])
	}
	content, ok := msg["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("messages[0].content is not a non-empty array: %v", msg["content"])
	}
	block, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("messages[0].content[0] is not an object: %v", content[0])
	}
	text, ok := block["text"].(string)
	if !ok {
		t.Fatalf("messages[0].content[0].text is not a string: %v", block["text"])
	}
	return text
}

// TestRunner_UserComment_InMessageNotSystemPrompt guards the architectural
// requirement from issue #61: a user-supplied comment must reach Claude only
// through the message turn, never through the system prompt, so it can never
// carry system-prompt authority (e.g. override language/format/tool rules).
func TestRunner_UserComment_InMessageNotSystemPrompt(t *testing.T) {
	const comment = "Bitte pruefe, ob die Rabattberechnung auf 2 Nachkommastellen rundet (Sonderwunsch-Marker-ACME-42)."
	var capturedSystemPrompt, capturedUserMessage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if sys, ok := body["system"].([]any); ok && len(sys) > 0 {
			if block, ok := sys[0].(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					capturedSystemPrompt = text
				}
			}
		}
		capturedUserMessage = firstUserMessageText(t, body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_01", "type": "message", "role": "assistant",
			"model": "claude-opus-4-8", "stop_reason": "end_turn",
			"content": []map[string]any{{"type": "text", "text": "Review."}},
			"usage":   map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	fake := &fakeADTClient{trObjects: nil}
	tools := agent.NewTools(fake)
	claudeClient := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
	runner := agent.NewRunner(tools, claudeClient)

	_, _, err := runner.Run(context.Background(), "NPLK900014", "claude-opus-4-8", "review_pedantic", comment)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(capturedUserMessage, comment) {
		t.Errorf("user comment must appear in the message content, got: %q", capturedUserMessage)
	}
	if !strings.Contains(capturedUserMessage, "<user_comment>") {
		t.Errorf("user comment must be wrapped in a <user_comment> block, got: %q", capturedUserMessage)
	}
	if strings.Contains(capturedSystemPrompt, comment) {
		t.Error("user comment must NEVER appear in the system prompt — it must only carry message-level authority")
	}
	if !strings.Contains(capturedSystemPrompt, "<user_comment>") {
		t.Error("system prompt must explain how to handle the <user_comment> block when one is actually sent")
	}
}

// TestRunner_EmptyUserComment_MessageUnchanged locks in that the common case
// (no comment) is byte-identical to pre-feature behaviour — no dangling empty
// <user_comment> block or wasted tokens.
func TestRunner_EmptyUserComment_MessageUnchanged(t *testing.T) {
	var capturedUserMessage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		capturedUserMessage = firstUserMessageText(t, body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_01", "type": "message", "role": "assistant",
			"model": "claude-opus-4-8", "stop_reason": "end_turn",
			"content": []map[string]any{{"type": "text", "text": "Review."}},
			"usage":   map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	fake := &fakeADTClient{trObjects: nil}
	tools := agent.NewTools(fake)
	claudeClient := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
	runner := agent.NewRunner(tools, claudeClient)

	_, _, err := runner.Run(context.Background(), "NPLK900014", "claude-opus-4-8", "review_pedantic", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := "Please review transport request: NPLK900014"
	if capturedUserMessage != want {
		t.Errorf("message: got %q, want %q", capturedUserMessage, want)
	}
}

// TestRunner_EmptyUserComment_SystemPromptUnchanged locks in that reviews
// without a comment (today's common case) pay zero extra tokens/complexity
// for the comment-handling instructions — the system prompt sent to Claude
// must be byte-identical to the plain style prompt.
func TestRunner_EmptyUserComment_SystemPromptUnchanged(t *testing.T) {
	var capturedSystemPrompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if sys, ok := body["system"].([]any); ok && len(sys) > 0 {
			if block, ok := sys[0].(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					capturedSystemPrompt = text
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_01", "type": "message", "role": "assistant",
			"model": "claude-opus-4-8", "stop_reason": "end_turn",
			"content": []map[string]any{{"type": "text", "text": "Review."}},
			"usage":   map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	fake := &fakeADTClient{trObjects: nil}
	tools := agent.NewTools(fake)
	claudeClient := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
	runner := agent.NewRunner(tools, claudeClient)

	_, _, err := runner.Run(context.Background(), "NPLK900014", "claude-opus-4-8", "review_pedantic", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := agent.AllowedPrompts()["review_pedantic"].Text
	if capturedSystemPrompt != want {
		t.Errorf("system prompt must be unchanged when no comment is given\ngot:  %q\nwant: %q", capturedSystemPrompt[:min(80, len(capturedSystemPrompt))], want[:min(80, len(want))])
	}
	if strings.Contains(capturedSystemPrompt, "user_comment") {
		t.Error("system prompt must not mention user_comment handling at all when no comment is given")
	}
}
