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

func TestOpenAIToClaudeSingleBlockContent(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": map[string]any{
					"type": "text",
					"text": "single block object content",
				},
			},
		},
	}

	res, err := openAIToClaude("claude-sonnet-4-20250514", body, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := res["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	msg := msgs[0].(map[string]any)
	content := msg["content"].([]any)
	block := content[0].(map[string]any)
	if block["type"] != "text" || block["text"] != "single block object content" {
		t.Errorf("unexpected content block: %+v", block)
	}
}

func TestOpenAIToClaudeVisionImageURL(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "What is in this image?"},
					map[string]any{
						"type": "image_url",
						"image_url": map[string]any{
							"url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
						},
					},
				},
			},
		},
	}

	res, err := openAIToClaude("claude-sonnet-4-20250514", body, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := res["messages"].([]any)
	msg := msgs[0].(map[string]any)
	content := msg["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(content))
	}

	imgBlock := content[1].(map[string]any)
	if imgBlock["type"] != "image" {
		t.Errorf("expected type=image, got %v", imgBlock["type"])
	}
	source, ok := imgBlock["source"].(map[string]any)
	if !ok || source["type"] != "base64" || source["media_type"] != "image/png" {
		t.Errorf("unexpected source block: %+v", source)
	}
}

func TestOpenAIToClaudeCacheControlBudget(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "block 1", "cache_control": map[string]any{"type": "ephemeral"}},
					map[string]any{"type": "text", "text": "block 2", "cache_control": map[string]any{"type": "ephemeral"}},
					map[string]any{"type": "text", "text": "block 3", "cache_control": map[string]any{"type": "ephemeral"}},
					map[string]any{"type": "text", "text": "block 4", "cache_control": map[string]any{"type": "ephemeral"}},
					map[string]any{"type": "text", "text": "block 5", "cache_control": map[string]any{"type": "ephemeral"}},
					map[string]any{"type": "text", "text": "block 6", "cache_control": map[string]any{"type": "ephemeral"}},
				},
			},
		},
	}

	res, err := openAIToClaude("claude-sonnet-4-20250514", body, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := res["messages"].([]any)
	msg := msgs[0].(map[string]any)
	content := msg["content"].([]any)

	cacheCount := 0
	for _, c := range content {
		cm := c.(map[string]any)
		if _, ok := cm["cache_control"]; ok {
			cacheCount++
		}
	}

	if cacheCount != 4 {
		t.Errorf("expected exactly 4 cache_control markers after trim, got %d", cacheCount)
	}
}

func TestGeminiSingleBlockContent(t *testing.T) {
	body := map[string]any{
		"model": "gemini-2.5-flash",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": map[string]any{
					"type": "text",
					"text": "single block gemini",
				},
			},
		},
	}

	res, err := openAIToGemini("gemini-2.5-flash", body, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	contents := res["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
	c := contents[0].(map[string]any)
	parts := c["parts"].([]any)
	p := parts[0].(map[string]any)
	if p["text"] != "single block gemini" {
		t.Errorf("expected text='single block gemini', got %v", p["text"])
	}
}

func TestOpenAIToClaudeConsecutiveToolMessagesMerged(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			map[string]any{"role": "user", "content": "run tools"},
			map[string]any{"role": "assistant", "content": "running"},
			map[string]any{"role": "tool", "tool_call_id": "call_1", "content": "result 1"},
			map[string]any{"role": "tool", "tool_call_id": "call_2", "content": "result 2"},
		},
	}

	res, err := openAIToClaude("claude-sonnet-4-20250514", body, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := res["messages"].([]any)
	// Expect 3 messages: user -> assistant -> user (containing both tool_results)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages with merged tool results, got %d", len(msgs))
	}

	toolUserMsg := msgs[2].(map[string]any)
	if toolUserMsg["role"] != "user" {
		t.Errorf("expected role=user, got %v", toolUserMsg["role"])
	}
	content := toolUserMsg["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("expected 2 tool_result blocks in single user message, got %d", len(content))
	}
	block1 := content[0].(map[string]any)
	block2 := content[1].(map[string]any)
	if block1["tool_use_id"] != "call_1" || block2["tool_use_id"] != "call_2" {
		t.Errorf("tool_use_id mismatch: block1=%v, block2=%v", block1["tool_use_id"], block2["tool_use_id"])
	}
}
