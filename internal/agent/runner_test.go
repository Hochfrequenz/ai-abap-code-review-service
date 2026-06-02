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
