package translator

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenAIToClaudeMidSystemMessage(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4",
		"messages": []any{
			map[string]any{"role": "system", "content": "You are helpful."},
			map[string]any{"role": "user", "content": "Hello"},
			map[string]any{"role": "system", "content": "Remember rule 2."},
			map[string]any{"role": "user", "content": "Help me."},
		},
	}

	res, err := openAIToClaude("claude-3-7-sonnet", body, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sys, ok := res["system"].(string)
	if !ok || sys != "You are helpful." {
		t.Errorf("expected top-level system='You are helpful.', got %q", sys)
	}

	msgs, ok := res["messages"].([]any)
	if !ok || len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	// Verify second message was converted to [System Instruction] user block
	secondMsg := msgs[1].(map[string]any)
	if secondMsg["role"] != "user" {
		t.Errorf("expected mid-system role=user, got %v", secondMsg["role"])
	}
	contentArr := secondMsg["content"].([]any)
	firstBlock := contentArr[0].(map[string]any)
	text := firstBlock["text"].(string)
	if !strings.Contains(text, "[System Instruction]: Remember rule 2.") {
		t.Errorf("expected wrapped system instruction, got %q", text)
	}
}

func TestClaudeToolUseEmptyArgs(t *testing.T) {
	claudeResp := map[string]any{
		"id":   "msg_123",
		"type": "message",
		"role": "assistant",
		"content": []any{
			map[string]any{
				"type":  "tool_use",
				"id":    "tool_1",
				"name":  "get_weather",
				"input": nil, // nil/empty input
			},
		},
		"stop_reason": "tool_use",
	}
	data, _ := json.Marshal(claudeResp)
	result, err := claudeToOpenAI(data, "claude-3-7-sonnet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var openAIResp map[string]any
	json.Unmarshal(result, &openAIResp)

	choices := openAIResp["choices"].([]any)
	choice := choices[0].(map[string]any)
	msg := choice["message"].(map[string]any)
	toolCalls := msg["tool_calls"].([]any)
	tc := toolCalls[0].(map[string]any)
	fn := tc["function"].(map[string]any)
	if fn["arguments"] != "{}" {
		t.Errorf("expected arguments='{}', got %q", fn["arguments"])
	}
}
