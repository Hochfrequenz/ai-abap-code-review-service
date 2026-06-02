package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/agent"
)

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
	result, err := runner.Run(context.Background(), "NPLK900014")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(result, "Code Review") {
		t.Errorf("expected review text in result, got: %q", result)
	}
	if len(calls) != 1 || calls[0] != "list_tr_objects" {
		t.Errorf("expected list_tr_objects call, got: %v", calls)
	}
}

func TestRunner_DispatchFetchSource(t *testing.T) {
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
					"type": "tool_use", "id": "t1", "name": "fetch_source",
					"input": map[string]any{"object_uri": "/sap/bc/adt/oo/classes/zcl_foo"},
				}},
				"usage": map[string]any{"input_tokens": 10, "output_tokens": 5},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_02", "type": "message", "role": "assistant",
			"model": "claude-opus-4-8", "stop_reason": "end_turn",
			"content": []map[string]any{{"type": "text", "text": "# Review\n\nSource looks good."}},
			"usage": map[string]any{"input_tokens": 20, "output_tokens": 10},
		})
	}))
	defer srv.Close()

	fake := &fakeADTClient{sources: map[string]string{
		"/sap/bc/adt/oo/classes/zcl_foo": "CLASS zcl_foo DEFINITION.",
	}}
	tools := agent.NewTools(fake)
	claudeClient := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test"))
	runner := agent.NewRunner(tools, claudeClient)
	result, err := runner.Run(context.Background(), "NPLK900014")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(result, "Review") {
		t.Errorf("expected review, got: %q", result)
	}
}

func TestRunner_DispatchFetchClassIncludes(t *testing.T) {
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
					"type": "tool_use", "id": "t1", "name": "fetch_class_includes",
					"input": map[string]any{"class_uri": "/sap/bc/adt/oo/classes/zcl_foo"},
				}},
				"usage": map[string]any{"input_tokens": 10, "output_tokens": 5},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_02", "type": "message", "role": "assistant",
			"model": "claude-opus-4-8", "stop_reason": "end_turn",
			"content": []map[string]any{{"type": "text", "text": "# Review\n\nIncludes look good."}},
			"usage": map[string]any{"input_tokens": 20, "output_tokens": 10},
		})
	}))
	defer srv.Close()

	fake := &fakeADTClient{sources: map[string]string{
		"/sap/bc/adt/oo/classes/zcl_foo/definitions": "DEFINITION.",
	}}
	tools := agent.NewTools(fake)
	claudeClient := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test"))
	runner := agent.NewRunner(tools, claudeClient)
	result, err := runner.Run(context.Background(), "NPLK900014")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(result, "Review") {
		t.Errorf("expected review, got: %q", result)
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
			"usage": map[string]any{"input_tokens": 100, "output_tokens": 8192},
		})
	}))
	defer srv.Close()

	fake := &fakeADTClient{trObjects: nil}
	tools := agent.NewTools(fake)
	claudeClient := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test"))
	runner := agent.NewRunner(tools, claudeClient)
	result, err := runner.Run(context.Background(), "NPLK900014")
	if err != nil {
		t.Fatalf("expected partial result not error, got: %v", err)
	}
	if !strings.Contains(result, "truncated") {
		t.Errorf("expected truncation note, got: %q", result)
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
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	fake := &fakeADTClient{trObjects: nil}
	tools := agent.NewTools(fake)
	claudeClient := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test"))
	runner := agent.NewRunner(tools, claudeClient)
	_, err := runner.Run(context.Background(), "NPLK900014")
	if err == nil {
		t.Error("expected error for unexpected stop reason")
	}
}
