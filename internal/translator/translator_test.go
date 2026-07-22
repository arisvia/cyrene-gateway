package translator

import (
	"encoding/json"
	"testing"
)

func TestOpenAIToClaudeRequest(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4",
		"messages": []any{
			map[string]any{"role": "system", "content": "You are helpful."},
			map[string]any{"role": "user", "content": "Hello"},
		},
		"temperature": 0.7,
		"max_tokens":  float64(1024),
	}

	result, err := openAIToClaude("claude-sonnet-4-20250514", body, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["model"] != "claude-sonnet-4-20250514" {
		t.Fatalf("expected model=claude-sonnet-4-20250514, got %v", result["model"])
	}
	if result["system"] != "You are helpful." {
		t.Fatalf("expected system prompt, got %v", result["system"])
	}
	if result["max_tokens"] != 1024 {
		t.Fatalf("expected max_tokens=1024, got %v", result["max_tokens"])
	}

	messages, ok := result["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("expected 1 message (user only), got %v", result["messages"])
	}
}

func TestOpenAIToClaudeWithTools(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4",
		"messages": []any{
			map[string]any{"role": "user", "content": "What's the weather?"},
		},
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "get_weather",
					"description": "Get weather info",
					"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
				},
			},
		},
		"tool_choice": "auto",
	}

	result, err := openAIToClaude("claude-sonnet-4-20250514", body, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tools, ok := result["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %v", result["tools"])
	}

	tool := tools[0].(map[string]any)
	if tool["name"] != "get_weather" {
		t.Fatalf("expected tool name=get_weather, got %v", tool["name"])
	}
	if _, ok := tool["input_schema"]; !ok {
		t.Fatal("expected input_schema in Claude tool format")
	}

	tc := result["tool_choice"].(map[string]any)
	if tc["type"] != "auto" {
		t.Fatalf("expected tool_choice type=auto, got %v", tc["type"])
	}
}

func TestOpenAIToGeminiRequest(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4",
		"messages": []any{
			map[string]any{"role": "system", "content": "You are helpful."},
			map[string]any{"role": "user", "content": "Hello"},
		},
		"temperature": 0.7,
		"max_tokens":  float64(2048),
	}

	result, err := openAIToGemini("gemini-pro", body, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check system instruction
	sysInstr, ok := result["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatalf("expected systemInstruction, got %v", result)
	}
	parts := sysInstr["parts"].([]any)
	if len(parts) != 1 {
		t.Fatalf("expected 1 system part, got %v", parts)
	}

	// Check contents
	contents, ok := result["contents"].([]any)
	if !ok || len(contents) != 1 {
		t.Fatalf("expected 1 content (user), got %v", result["contents"])
	}

	// Check generation config
	genConfig, ok := result["generationConfig"].(map[string]any)
	if !ok {
		t.Fatalf("expected generationConfig, got %v", result)
	}
	if genConfig["maxOutputTokens"] != 2048 {
		t.Fatalf("expected maxOutputTokens=2048, got %v", genConfig["maxOutputTokens"])
	}
}

func TestClaudeToOpenAIResponse(t *testing.T) {
	claudeResp := map[string]any{
		"id":   "msg_123",
		"type": "message",
		"role": "assistant",
		"content": []any{
			map[string]any{"type": "text", "text": "Hello!"},
		},
		"stop_reason": "end_turn",
		"usage": map[string]any{
			"input_tokens":  float64(10),
			"output_tokens": float64(5),
		},
	}
	data, _ := json.Marshal(claudeResp)

	result, err := claudeToOpenAI(data, "claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var openAIResp map[string]any
	json.Unmarshal(result, &openAIResp)

	if openAIResp["object"] != "chat.completion" {
		t.Fatalf("expected object=chat.completion, got %v", openAIResp["object"])
	}

	choices := openAIResp["choices"].([]any)
	choice := choices[0].(map[string]any)
	message := choice["message"].(map[string]any)
	if message["content"] != "Hello!" {
		t.Fatalf("expected content=Hello!, got %v", message["content"])
	}
	if choice["finish_reason"] != "stop" {
		t.Fatalf("expected finish_reason=stop, got %v", choice["finish_reason"])
	}

	usage := openAIResp["usage"].(map[string]any)
	if usage["prompt_tokens"] != float64(10) {
		t.Fatalf("expected prompt_tokens=10, got %v", usage["prompt_tokens"])
	}
}

func TestGeminiToOpenAIResponse(t *testing.T) {
	geminiResp := map[string]any{
		"candidates": []any{
			map[string]any{
				"content": map[string]any{
					"parts": []any{
						map[string]any{"text": "Hi there!"},
					},
				},
				"finishReason": "STOP",
			},
		},
		"usageMetadata": map[string]any{
			"promptTokenCount":     float64(8),
			"candidatesTokenCount": float64(3),
			"totalTokenCount":      float64(11),
		},
	}
	data, _ := json.Marshal(geminiResp)

	result, err := geminiToOpenAI(data, "gemini-pro")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var openAIResp map[string]any
	json.Unmarshal(result, &openAIResp)

	if openAIResp["object"] != "chat.completion" {
		t.Fatalf("expected object=chat.completion, got %v", openAIResp["object"])
	}

	choices := openAIResp["choices"].([]any)
	choice := choices[0].(map[string]any)
	message := choice["message"].(map[string]any)
	if message["content"] != "Hi there!" {
		t.Fatalf("expected content='Hi there!', got %v", message["content"])
	}
}

func TestClaudeSSEToOpenAI(t *testing.T) {
	// content_block_delta with text
	event := map[string]any{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]any{
			"type": "text_delta",
			"text": "Hello",
		},
	}
	data, _ := json.Marshal(event)

	result, isDone, err := claudeSSEToOpenAI(data, "claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDone {
		t.Fatal("should not be done")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	var chunk map[string]any
	json.Unmarshal(result, &chunk)
	if chunk["object"] != "chat.completion.chunk" {
		t.Fatalf("expected chunk object, got %v", chunk["object"])
	}
	choices := chunk["choices"].([]any)
	delta := choices[0].(map[string]any)["delta"].(map[string]any)
	if delta["content"] != "Hello" {
		t.Fatalf("expected delta content=Hello, got %v", delta["content"])
	}
}

func TestClaudeSSEMessageStop(t *testing.T) {
	event := map[string]any{"type": "message_stop"}
	data, _ := json.Marshal(event)

	result, isDone, err := claudeSSEToOpenAI(data, "claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isDone {
		t.Fatal("expected isDone=true for message_stop")
	}
	if string(result) != "[DONE]" {
		t.Fatalf("expected [DONE], got %s", result)
	}
}

func TestTranslateRequestOpenAIPassthrough(t *testing.T) {
	body := map[string]any{
		"model":    "gpt-4",
		"messages": []any{},
	}

	result, err := TranslateRequest(FormatOpenAI, "gpt-4-turbo", body, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["model"] != "gpt-4-turbo" {
		t.Fatalf("expected model=gpt-4-turbo, got %v", result["model"])
	}
}

func TestParseDataURI(t *testing.T) {
	mediaType, data := parseDataURI("data:image/png;base64,iVBORw0KGgo=")
	if mediaType != "image/png" {
		t.Fatalf("expected mediaType=image/png, got %s", mediaType)
	}
	if data != "iVBORw0KGgo=" {
		t.Fatalf("expected data=iVBORw0KGgo=, got %s", data)
	}
}
