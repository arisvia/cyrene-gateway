package translator

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResponsesToOpenAIRequest_StringInput(t *testing.T) {
	req := map[string]any{
		"model":        "gpt-5.4",
		"instructions": "Be extremely succinct.",
		"input":        "What is the capital of France?",
		"temperature":  0.7,
		"stream":       false,
	}

	converted, err := ResponsesToOpenAIRequest(req)
	if err != nil {
		t.Fatalf("ResponsesToOpenAIRequest failed: %v", err)
	}

	if converted["model"] != "gpt-5.4" {
		t.Errorf("expected model gpt-5.4, got %v", converted["model"])
	}

	msgs, ok := converted["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %v", msgs)
	}

	devMsg := msgs[0].(map[string]any)
	if devMsg["role"] != "developer" || devMsg["content"] != "Be extremely succinct." {
		t.Errorf("unexpected developer message: %v", devMsg)
	}

	userMsg := msgs[1].(map[string]any)
	if userMsg["role"] != "user" || userMsg["content"] != "What is the capital of France?" {
		t.Errorf("unexpected user message: %v", userMsg)
	}
}

func TestResponsesToOpenAIRequest_ArrayInputAndTools(t *testing.T) {
	req := map[string]any{
		"model": "gpt-5.4",
		"input": []any{
			map[string]any{
				"role":    "user",
				"content": "Hello",
			},
			map[string]any{
				"role":    "assistant",
				"content": "Hi there!",
			},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "input_text",
						"text": "What is the weather?",
					},
				},
			},
		},
		"tools": []any{
			map[string]any{
				"type":        "function",
				"name":        "get_weather",
				"description": "Get current weather",
				"parameters": map[string]any{
					"type": "object",
				},
			},
		},
	}

	converted, err := ResponsesToOpenAIRequest(req)
	if err != nil {
		t.Fatalf("ResponsesToOpenAIRequest failed: %v", err)
	}

	msgs, ok := converted["messages"].([]any)
	if !ok || len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %v", msgs)
	}

	tools, ok := converted["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %v", tools)
	}
	toolMap := tools[0].(map[string]any)
	fn, ok := toolMap["function"].(map[string]any)
	if !ok || fn["name"] != "get_weather" {
		t.Errorf("expected nested function with name get_weather, got %v", toolMap)
	}
}

func TestOpenAIToResponsesResponse(t *testing.T) {
	openAIJSON := `{
		"id": "chatcmpl-999",
		"object": "chat.completion",
		"created": 1741487325,
		"model": "gpt-5.4",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "The capital is Paris."
				},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 12,
			"completion_tokens": 6,
			"total_tokens": 18
		}
	}`

	respBytes, err := OpenAIToResponsesResponse([]byte(openAIJSON), "gpt-5.4")
	if err != nil {
		t.Fatalf("OpenAIToResponsesResponse failed: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["object"] != "response" {
		t.Errorf("expected object 'response', got %v", resp["object"])
	}
	if resp["status"] != "completed" {
		t.Errorf("expected status 'completed', got %v", resp["status"])
	}
	if resp["output_text"] != "The capital is Paris." {
		t.Errorf("expected output_text 'The capital is Paris.', got %v", resp["output_text"])
	}

	usage := resp["usage"].(map[string]any)
	if usage["total_tokens"].(float64) != 18 {
		t.Errorf("expected total_tokens 18, got %v", usage["total_tokens"])
	}

	output := resp["output"].([]any)
	if len(output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(output))
	}
	item := output[0].(map[string]any)
	if item["type"] != "message" || item["role"] != "assistant" {
		t.Errorf("unexpected output item: %v", item)
	}
}

func TestOpenAIToResponsesSSETranslator(t *testing.T) {
	trans := NewOpenAIToResponsesSSETranslator("gpt-5.4")

	chunk1 := []byte(`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`)
	s1, done1, _ := trans.TranslateChunk(chunk1)
	if done1 {
		t.Errorf("chunk1 should not be done")
	}
	if !strings.Contains(string(s1), "event: response.created") {
		t.Errorf("expected response.created event in chunk 1, got: %s", string(s1))
	}

	chunk2 := []byte(`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"Paris "},"finish_reason":null}]}`)
	s2, _, _ := trans.TranslateChunk(chunk2)
	str2 := string(s2)
	if !strings.Contains(str2, "event: response.output_item.added") {
		t.Errorf("expected response.output_item.added in chunk 2, got: %s", str2)
	}
	if !strings.Contains(str2, "event: response.content_part.added") {
		t.Errorf("expected response.content_part.added in chunk 2, got: %s", str2)
	}
	if !strings.Contains(str2, "event: response.output_text.delta") {
		t.Errorf("expected response.output_text.delta in chunk 2, got: %s", str2)
	}

	chunkDone := []byte(`[DONE]`)
	sDone, doneFinal, _ := trans.TranslateChunk(chunkDone)
	if !doneFinal {
		t.Errorf("expected doneFinal true on [DONE]")
	}
	strDone := string(sDone)
	if !strings.Contains(strDone, "event: response.content_part.done") {
		t.Errorf("expected response.content_part.done on [DONE], got: %s", strDone)
	}
	if !strings.Contains(strDone, "event: response.output_item.done") {
		t.Errorf("expected response.output_item.done on [DONE], got: %s", strDone)
	}
	if !strings.Contains(strDone, "event: response.done") {
		t.Errorf("expected response.done on [DONE], got: %s", strDone)
	}
}
func TestOpenAIToResponsesRequest(t *testing.T) {
	openAIBody := map[string]any{
		"model": "gpt-5.4",
		"messages": []any{
			map[string]any{
				"role":    "system",
				"content": "System directive.",
			},
			map[string]any{
				"role":    "user",
				"content": "User question.",
			},
		},
		"temperature": 0.8,
	}

	res, err := OpenAIToResponsesRequest("gpt-5.4", openAIBody, false)
	if err != nil {
		t.Fatalf("OpenAIToResponsesRequest failed: %v", err)
	}

	if res["instructions"] != "System directive." {
		t.Errorf("expected instructions 'System directive.', got %v", res["instructions"])
	}

	input, ok := res["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("expected 1 input item, got %v", input)
	}
}

func TestResponsesToOpenAIResponse(t *testing.T) {
	respJSON := `{
		"id": "resp_test123",
		"object": "response",
		"status": "completed",
		"model": "gpt-5.4",
		"output_text": "Here is the answer",
		"usage": {
			"input_tokens": 15,
			"output_tokens": 7,
			"total_tokens": 22
		}
	}`

	converted, err := ResponsesToOpenAIResponse([]byte(respJSON), "gpt-5.4")
	if err != nil {
		t.Fatalf("ResponsesToOpenAIResponse failed: %v", err)
	}

	var chatResp map[string]any
	if err := json.Unmarshal(converted, &chatResp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	choices := chatResp["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "Here is the answer" {
		t.Errorf("expected content 'Here is the answer', got %v", msg["content"])
	}

	usage := chatResp["usage"].(map[string]any)
	if usage["prompt_tokens"].(float64) != 15 || usage["completion_tokens"].(float64) != 7 {
		t.Errorf("unexpected usage: %v", usage)
	}
}

func TestResponsesSSEToOpenAI(t *testing.T) {
	deltaLine := []byte(`{"type":"response.output_text.delta","delta":"streaming chunk"}`)
	chunk, done, err := ResponsesSSEToOpenAI(deltaLine, "gpt-5.4")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if done {
		t.Errorf("should not be done")
	}
	if !strings.Contains(string(chunk), "streaming chunk") {
		t.Errorf("expected chunk to contain 'streaming chunk', got: %s", string(chunk))
	}

	doneLine := []byte(`{"type":"response.done","response":{"usage":{"input_tokens":5,"output_tokens":3,"total_tokens":8}}}`)
	finalChunk, isDone, err := ResponsesSSEToOpenAI(doneLine, "gpt-5.4")
	if err != nil || !isDone {
		t.Fatalf("expected done true, got isDone=%v, err=%v", isDone, err)
	}
	if !strings.Contains(string(finalChunk), `"finish_reason":"stop"`) {
		t.Errorf("expected finish_reason stop, got: %s", string(finalChunk))
	}
}
