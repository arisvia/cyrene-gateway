package translator

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClaudeToOpenAIRequest_Basic(t *testing.T) {
	claudeBody := map[string]any{
		"model":  "qoder/efficient",
		"system": "You are a test assistant.",
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Hello world",
			},
		},
		"max_tokens":  float64(1024),
		"temperature": 0.7,
		"stream":      true,
	}

	openAIBody, err := ClaudeToOpenAIRequest(claudeBody)
	if err != nil {
		t.Fatalf("ClaudeToOpenAIRequest failed: %v", err)
	}

	if openAIBody["model"] != "qoder/efficient" {
		t.Errorf("expected model 'qoder/efficient', got %v", openAIBody["model"])
	}
	if openAIBody["stream"] != true {
		t.Errorf("expected stream true, got %v", openAIBody["stream"])
	}

	msgs, ok := openAIBody["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %v", msgs)
	}

	sysMsg := msgs[0].(map[string]any)
	if sysMsg["role"] != "system" || sysMsg["content"] != "You are a test assistant." {
		t.Errorf("unexpected system message: %v", sysMsg)
	}

	userMsg := msgs[1].(map[string]any)
	if userMsg["role"] != "user" || userMsg["content"] != "Hello world" {
		t.Errorf("unexpected user message: %v", userMsg)
	}
}

func TestOpenAIToClaudeResponse_Basic(t *testing.T) {
	openAIJSON := []byte(`{
		"id": "chatcmpl-test123",
		"object": "chat.completion",
		"model": "qoder/efficient",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Hello from OpenAI response!"
				},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 12,
			"completion_tokens": 8,
			"total_tokens": 20
		}
	}`)

	claudeBytes, err := OpenAIToClaudeResponse(openAIJSON, "qoder/efficient")
	if err != nil {
		t.Fatalf("OpenAIToClaudeResponse failed: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(claudeBytes, &resp); err != nil {
		t.Fatalf("unmarshal claude response failed: %v", err)
	}

	if resp["type"] != "message" {
		t.Errorf("expected type 'message', got %v", resp["type"])
	}
	if resp["stop_reason"] != "end_turn" {
		t.Errorf("expected stop_reason 'end_turn', got %v", resp["stop_reason"])
	}

	content := resp["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}
	block := content[0].(map[string]any)
	if block["type"] != "text" || block["text"] != "Hello from OpenAI response!" {
		t.Errorf("unexpected content block: %v", block)
	}

	usage := resp["usage"].(map[string]any)
	if usage["input_tokens"] != float64(12) || usage["output_tokens"] != float64(8) {
		t.Errorf("unexpected usage: %v", usage)
	}
}

func TestOpenAIToClaudeSSETranslator(t *testing.T) {
	trans := NewOpenAIToClaudeSSETranslator("qoder/efficient")

	chunk1 := []byte(`{"id":"chatcmpl-stream1","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`)
	out1, done1, err := trans.TranslateChunk(chunk1)
	if err != nil || done1 {
		t.Fatalf("chunk1 failed: err=%v, done=%v", err, done1)
	}

	s1 := string(out1)
	if !strings.Contains(s1, "event: message_start") {
		t.Errorf("expected message_start in chunk1, got: %s", s1)
	}
	if !strings.Contains(s1, "event: content_block_start") {
		t.Errorf("expected content_block_start in chunk1, got: %s", s1)
	}
	if !strings.Contains(s1, "event: content_block_delta") || !strings.Contains(s1, "Hello") {
		t.Errorf("expected content_block_delta with 'Hello' in chunk1, got: %s", s1)
	}

	chunk2 := []byte(`{"id":"chatcmpl-stream1","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`)
	out2, done2, err := trans.TranslateChunk(chunk2)
	if err != nil || done2 {
		t.Fatalf("chunk2 failed: err=%v, done=%v", err, done2)
	}
	s2 := string(out2)
	if !strings.Contains(s2, "event: content_block_delta") || !strings.Contains(s2, " world") {
		t.Errorf("expected delta with ' world', got: %s", s2)
	}

	doneChunk := []byte("[DONE]")
	outDone, done3, err := trans.TranslateChunk(doneChunk)
	if err != nil || !done3 {
		t.Fatalf("doneChunk failed: err=%v, done=%v", err, done3)
	}
	sDone := string(outDone)
	if !strings.Contains(sDone, "event: content_block_stop") {
		t.Errorf("expected content_block_stop, got: %s", sDone)
	}
	if !strings.Contains(sDone, "event: message_delta") {
		t.Errorf("expected message_delta, got: %s", sDone)
	}
	if !strings.Contains(sDone, "event: message_stop") {
		t.Errorf("expected message_stop, got: %s", sDone)
	}
}
