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

func TestOpenAIToClaudeSSETranslator_ToolCalls(t *testing.T) {
	trans := NewOpenAIToClaudeSSETranslator("deepseek/deepseek-chat")

	// 1. Text chunk
	c1 := []byte(`{"id":"chatcmpl-tc1","choices":[{"index":0,"delta":{"role":"assistant","content":"Checking math..."},"finish_reason":null}]}`)
	out1, _, _ := trans.TranslateChunk(c1)
	s1 := string(out1)
	if !strings.Contains(s1, "event: content_block_start") || !strings.Contains(s1, "Checking math...") {
		t.Fatalf("expected text content in chunk1, got: %s", s1)
	}

	// 2. Tool call initiation chunk (text block should be stopped, tool block started)
	c2 := []byte(`{"id":"chatcmpl-tc1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_calc_99","type":"function","function":{"name":"calc","arguments":""}}]},"finish_reason":null}]}`)
	out2, _, _ := trans.TranslateChunk(c2)
	s2 := string(out2)
	if !strings.Contains(s2, "event: content_block_stop") {
		t.Errorf("expected prior text block to be closed, got: %s", s2)
	}
	if !strings.Contains(s2, "event: content_block_start") || !strings.Contains(s2, "tool_use") || !strings.Contains(s2, "call_calc_99") || !strings.Contains(s2, "calc") {
		t.Errorf("expected tool_use block start, got: %s", s2)
	}

	// 3. Tool call arguments delta chunk
	c3 := []byte(`{"id":"chatcmpl-tc1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"expr\":\"1+1\"}"}}]},"finish_reason":null}]}`)
	out3, _, _ := trans.TranslateChunk(c3)
	s3 := string(out3)
	if !strings.Contains(s3, "event: content_block_delta") || !strings.Contains(s3, "input_json_delta") || !strings.Contains(s3, "1+1") {
		t.Errorf("expected input_json_delta, got: %s", s3)
	}

	// 4. Finish reason chunk
	c4 := []byte(`{"id":"chatcmpl-tc1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
	trans.TranslateChunk(c4)

	// 5. [DONE] chunk
	doneOut, isDone, _ := trans.TranslateChunk([]byte("[DONE]"))
	if !isDone {
		t.Fatalf("expected isDone=true")
	}
	sDone := string(doneOut)
	if !strings.Contains(sDone, "event: content_block_stop") {
		t.Errorf("expected tool content_block_stop, got: %s", sDone)
	}
	if !strings.Contains(sDone, `"stop_reason":"tool_use"`) {
		t.Errorf("expected stop_reason tool_use, got: %s", sDone)
	}
	if !strings.Contains(sDone, "event: message_stop") {
		t.Errorf("expected message_stop, got: %s", sDone)
	}
}
func TestOpenAIToClaudeSSETranslator_EmptyOrImmediateDone(t *testing.T) {
	trans := NewOpenAIToClaudeSSETranslator("qoder/dfmodel")

	// Immediate [DONE] without any prior chunks
	out, done, err := trans.TranslateChunk([]byte("[DONE]"))
	if err != nil || !done {
		t.Fatalf("expected done=true, err=nil; got err=%v, done=%v", err, done)
	}
	s := string(out)
	startIdx := strings.Index(s, "event: message_start")
	deltaIdx := strings.Index(s, "event: message_delta")
	stopIdx := strings.Index(s, "event: message_stop")

	if startIdx < 0 {
		t.Fatalf("expected message_start even on immediate [DONE], got:\n%s", s)
	}
	if deltaIdx < 0 || deltaIdx <= startIdx {
		t.Fatalf("expected message_delta after message_start, got startIdx=%d, deltaIdx=%d", startIdx, deltaIdx)
	}
	if stopIdx < 0 || stopIdx <= deltaIdx {
		t.Fatalf("expected message_stop after message_delta, got deltaIdx=%d, stopIdx=%d", deltaIdx, stopIdx)
	}

	// Repeated [DONE] must be idempotent and return empty
	out2, done2, err2 := trans.TranslateChunk([]byte("[DONE]"))
	if err2 != nil || !done2 || len(out2) > 0 {
		t.Fatalf("repeated [DONE] should return nil and done=true, got len=%d, done=%v, err=%v", len(out2), done2, err2)
	}
}
