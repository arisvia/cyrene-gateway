package translator

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGeminiToOpenAIRequest_Basic(t *testing.T) {
	geminiReq := map[string]any{
		"systemInstruction": map[string]any{
			"parts": []any{
				map[string]any{"text": "You are a senior engineer."},
			},
		},
		"contents": []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{"text": "Explain goroutines in Go."},
				},
			},
			map[string]any{
				"role": "model",
				"parts": []any{
					map[string]any{"text": "Goroutines are lightweight threads."},
				},
			},
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{"text": "How many can I run?"},
				},
			},
		},
		"generationConfig": map[string]any{
			"temperature":     0.7,
			"topP":            0.95,
			"maxOutputTokens": 2048,
			"thinkingConfig": map[string]any{
				"thinkingBudget": 2048,
			},
		},
	}

	openAIReq, err := GeminiToOpenAIRequest(geminiReq, "gemini-2.5-flash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if openAIReq["model"] != "gemini-2.5-flash" {
		t.Errorf("expected model=gemini-2.5-flash, got %v", openAIReq["model"])
	}

	msgs, ok := openAIReq["messages"].([]any)
	if !ok || len(msgs) != 4 {
		t.Fatalf("expected 4 messages (1 system + 3 history), got %d", len(msgs))
	}

	// Message 0: system
	m0 := msgs[0].(map[string]any)
	if m0["role"] != "system" || m0["content"] != "You are a senior engineer." {
		t.Errorf("unexpected m0: %+v", m0)
	}

	// Message 1: user
	m1 := msgs[1].(map[string]any)
	if m1["role"] != "user" || m1["content"] != "Explain goroutines in Go." {
		t.Errorf("unexpected m1: %+v", m1)
	}

	// Message 2: assistant
	m2 := msgs[2].(map[string]any)
	if m2["role"] != "assistant" || m2["content"] != "Goroutines are lightweight threads." {
		t.Errorf("unexpected m2: %+v", m2)
	}

	// Message 3: user
	m3 := msgs[3].(map[string]any)
	if m3["role"] != "user" || m3["content"] != "How many can I run?" {
		t.Errorf("unexpected m3: %+v", m3)
	}

	if openAIReq["temperature"] != 0.7 {
		t.Errorf("expected temperature=0.7, got %v", openAIReq["temperature"])
	}
	if openAIReq["top_p"] != 0.95 {
		t.Errorf("expected top_p=0.95, got %v", openAIReq["top_p"])
	}
	if openAIReq["max_tokens"] != 2048 {
		t.Errorf("expected max_tokens=2048, got %v", openAIReq["max_tokens"])
	}
	if openAIReq["reasoning_effort"] != "low" {
		t.Errorf("expected reasoning_effort=low, got %v", openAIReq["reasoning_effort"])
	}
}

func TestOpenAIToGeminiResponse(t *testing.T) {
	openAIResp := map[string]any{
		"id":      "chatcmpl-123",
		"object":  "chat.completion",
		"created": 1720000000,
		"model":   "gemini-2.5-flash",
		"choices": []any{
			map[string]any{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "Hello from Gemini via OpenAI!",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     15,
			"completion_tokens": 25,
			"total_tokens":      40,
		},
	}

	b, _ := json.Marshal(openAIResp)
	geminiBytes, err := OpenAIToGeminiResponse(b, "gemini-2.5-flash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var geminiResp map[string]any
	if err := json.Unmarshal(geminiBytes, &geminiResp); err != nil {
		t.Fatalf("failed to unmarshal gemini response: %v", err)
	}

	candidates, ok := geminiResp["candidates"].([]any)
	if !ok || len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %v", candidates)
	}

	c0 := candidates[0].(map[string]any)
	content := c0["content"].(map[string]any)
	if content["role"] != "model" {
		t.Errorf("expected role=model, got %v", content["role"])
	}

	parts := content["parts"].([]any)
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}

	p0 := parts[0].(map[string]any)
	if p0["text"] != "Hello from Gemini via OpenAI!" {
		t.Errorf("unexpected text: %v", p0["text"])
	}

	if c0["finishReason"] != "STOP" {
		t.Errorf("expected finishReason=STOP, got %v", c0["finishReason"])
	}

	meta := geminiResp["usageMetadata"].(map[string]any)
	if meta["promptTokenCount"] != 15.0 && meta["promptTokenCount"] != 15 {
		t.Errorf("expected promptTokenCount=15, got %v", meta["promptTokenCount"])
	}
}

func TestOpenAIToGeminiSSETranslator(t *testing.T) {
	trans := NewOpenAIToGeminiSSETranslator("gemini-2.5-pro")

	// 1. Text delta chunk
	chunk1 := map[string]any{
		"id":      "chatcmpl-test",
		"object":  "chat.completion.chunk",
		"created": 1720000000,
		"model":   "gemini-2.5-pro",
		"choices": []any{
			map[string]any{
				"index": 0,
				"delta": map[string]any{
					"content": "Streaming token",
				},
				"finish_reason": nil,
			},
		},
	}
	b1, _ := json.Marshal(chunk1)
	out1, done1, err1 := trans.TranslateChunk(b1)
	if err1 != nil || done1 {
		t.Fatalf("unexpected chunk1 result: err=%v, done=%v", err1, done1)
	}
	if !strings.Contains(string(out1), "Streaming token") {
		t.Errorf("expected output to contain token, got %s", string(out1))
	}
	if !strings.Contains(string(out1), `"role":"model"`) {
		t.Errorf("expected output to contain role model, got %s", string(out1))
	}

	// 2. [DONE]
	outDone, done2, err2 := trans.TranslateChunk([]byte("[DONE]"))
	if err2 != nil || !done2 {
		t.Fatalf("unexpected done result: err=%v, done=%v", err2, done2)
	}
	if !strings.Contains(string(outDone), "STOP") {
		t.Errorf("expected finishReason STOP on DONE, got %s", string(outDone))
	}
}

func TestGeminiToOpenAIRequest_ToolsAndToolResponses(t *testing.T) {
	geminiReq := map[string]any{
		"contents": []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{"text": "What is the weather in Tokyo?"},
				},
			},
			map[string]any{
				"role": "model",
				"parts": []any{
					map[string]any{
						"functionCall": map[string]any{
							"id":   "call_weather_tokyo",
							"name": "get_weather",
							"args": map[string]any{"city": "Tokyo"},
						},
					},
				},
			},
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{
						"functionResponse": map[string]any{
							"name":     "get_weather",
							"response": map[string]any{"temp": 22},
						},
					},
				},
			},
		},
		"tools": []any{
			map[string]any{
				"functionDeclarations": []any{
					map[string]any{
						"name":        "get_weather",
						"description": "Get current weather",
						"parameters": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"city": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
		"toolConfig": map[string]any{
			"functionCallingConfig": map[string]any{
				"mode": "AUTO",
			},
		},
	}

	openAIReq, err := GeminiToOpenAIRequest(geminiReq, "gemini-2.5-flash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tools, ok := openAIReq["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool in translated request, got %v", tools)
	}
	t0 := tools[0].(map[string]any)
	fn := t0["function"].(map[string]any)
	if fn["name"] != "get_weather" {
		t.Errorf("expected tool name get_weather, got %v", fn["name"])
	}

	if openAIReq["tool_choice"] != "auto" {
		t.Errorf("expected tool_choice auto, got %v", openAIReq["tool_choice"])
	}

	msgs := openAIReq["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	// Assistant message with tool_calls
	m1 := msgs[1].(map[string]any)
	if m1["role"] != "assistant" {
		t.Errorf("expected assistant role, got %v", m1["role"])
	}
	tcs := m1["tool_calls"].([]any)
	if len(tcs) != 1 {
		t.Fatalf("expected 1 tool_call, got %d", len(tcs))
	}
	tc0 := tcs[0].(map[string]any)
	if tc0["id"] != "call_weather_tokyo" {
		t.Errorf("expected id call_weather_tokyo, got %v", tc0["id"])
	}

	// Tool message with matching tool_call_id
	m2 := msgs[2].(map[string]any)
	if m2["role"] != "tool" {
		t.Errorf("expected role tool, got %v", m2["role"])
	}
	if m2["tool_call_id"] != "call_weather_tokyo" {
		t.Errorf("expected tool_call_id to match call_weather_tokyo, got %v", m2["tool_call_id"])
	}
}

func TestOpenAIToGeminiSSETranslator_StreamingToolCalls(t *testing.T) {
	trans := NewOpenAIToGeminiSSETranslator("gemini-2.5-flash")

	// Chunk 1: Tool call declaration
	c1 := map[string]any{
		"choices": []any{
			map[string]any{
				"index": 0,
				"delta": map[string]any{
					"tool_calls": []any{
						map[string]any{
							"index": 0,
							"id":    "call_calc_42",
							"type":  "function",
							"function": map[string]any{
								"name":      "calculate",
								"arguments": "{\"expr\":",
							},
						},
					},
				},
				"finish_reason": nil,
			},
		},
	}
	b1, _ := json.Marshal(c1)
	out1, done1, err1 := trans.TranslateChunk(b1)
	if err1 != nil || done1 || len(out1) > 0 {
		t.Fatalf("delta tool call chunk should be buffered, got len=%d, done=%v, err=%v", len(out1), done1, err1)
	}

	// Chunk 2: Tool call args continuation
	c2 := map[string]any{
		"choices": []any{
			map[string]any{
				"index": 0,
				"delta": map[string]any{
					"tool_calls": []any{
						map[string]any{
							"index": 0,
							"function": map[string]any{
								"arguments": "\"2+2\"}",
							},
						},
					},
				},
				"finish_reason": nil,
			},
		},
	}
	b2, _ := json.Marshal(c2)
	out2, done2, err2 := trans.TranslateChunk(b2)
	if err2 != nil || done2 || len(out2) > 0 {
		t.Fatalf("args delta chunk should be buffered, got len=%d, done=%v, err=%v", len(out2), done2, err2)
	}

	// Chunk 3: finish_reason tool_calls
	c3 := map[string]any{
		"choices": []any{
			map[string]any{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": "tool_calls",
			},
		},
	}
	b3, _ := json.Marshal(c3)
	out3, done3, err3 := trans.TranslateChunk(b3)
	if err3 != nil || !done3 {
		t.Fatalf("expected finish chunk done=true, got done=%v, err=%v", done3, err3)
	}
	s3 := string(out3)
	if !strings.Contains(s3, "calculate") || !strings.Contains(s3, "2+2") {
		t.Fatalf("expected complete functionCall in output, got: %s", s3)
	}
	if !strings.Contains(s3, `"finishReason":"STOP"`) {
		t.Fatalf("expected finishReason STOP, got: %s", s3)
	}
}

func TestOpenAIToGeminiSSETranslator_UsageOnlyChunk(t *testing.T) {
	trans := NewOpenAIToGeminiSSETranslator("gemini-2.5-flash")

	// Usage-only chunk (choices: [], standard OpenAI stream_options format)
	usageChunk := map[string]any{
		"choices": []any{},
		"usage": map[string]any{
			"prompt_tokens":     42,
			"completion_tokens": 18,
			"total_tokens":      60,
		},
	}
	b, _ := json.Marshal(usageChunk)
	out, done, err := trans.TranslateChunk(b)
	if err != nil || done {
		t.Fatalf("expected usage chunk to be emitted without marking done, got done=%v, err=%v", done, err)
	}
	s := string(out)
	if !strings.Contains(s, "usageMetadata") || !strings.Contains(s, "42") || !strings.Contains(s, "18") {
		t.Fatalf("expected usageMetadata in emitted chunk, got: %s", s)
	}
}

func TestGeminiParallelToolRoundTrip(t *testing.T) {
	response := []byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_tokyo","type":"function","function":{"name":"weather","arguments":"{\"city\":\"Tokyo\"}"}},{"id":"call_paris","type":"function","function":{"name":"weather","arguments":"{\"city\":\"Paris\"}"}}]},"finish_reason":"tool_calls"}]}`)
	geminiBytes, err := OpenAIToGeminiResponse(response, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	history := decodeTranslatorPayload(t, geminiBytes)["candidates"].([]any)[0].(map[string]any)["content"].(map[string]any)
	parts := history["parts"].([]any)
	for i, id := range []string{"call_tokyo", "call_paris"} {
		if parts[i].(map[string]any)["functionCall"].(map[string]any)["id"] != id {
			t.Fatalf("Gemini output dropped call ID: %s", geminiBytes)
		}
	}
	for _, tc := range []struct {
		name, responses string
		want            []string
	}{
		{"idless_fifo", `[{"functionResponse":{"name":"weather","response":{"city":"Tokyo"}}},{"functionResponse":{"name":"weather","response":{"city":"Paris"}}}]`, []string{"call_tokyo", "call_paris"}},
		{"explicit_reversed", `[{"functionResponse":{"id":"call_paris","name":"weather","response":{"city":"Paris"}}},{"functionResponse":{"id":"call_tokyo","name":"weather","response":{"city":"Tokyo"}}}]`, []string{"call_paris", "call_tokyo"}},
		{"explicit_then_idless", `[{"functionResponse":{"id":"call_paris","name":"weather","response":{"city":"Paris"}}},{"functionResponse":{"name":"weather","response":{"city":"Tokyo"}}}]`, []string{"call_paris", "call_tokyo"}},
		{"idless_then_explicit", `[{"functionResponse":{"name":"weather","response":{"city":"Paris"}}},{"functionResponse":{"id":"call_tokyo","name":"weather","response":{"city":"Tokyo"}}}]`, []string{"call_paris", "call_tokyo"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results := decodeTranslatorPayload(t, []byte(`{"role":"user","parts":`+tc.responses+`}`))
			request := map[string]any{"contents": []any{
				map[string]any{"role": "user", "parts": []any{map[string]any{"text": "Compare weather"}}},
				history, results,
			}}
			openAI, err := GeminiToOpenAIRequest(request, "gemini")
			if err != nil {
				t.Fatal(err)
			}
			messages := openAI["messages"].([]any)
			if len(messages) != 4 {
				t.Fatalf("expected two calls and two results: %#v", openAI)
			}
			for i, id := range tc.want {
				result := messages[i+2].(map[string]any)
				if result["role"] != "tool" || result["tool_call_id"] != id {
					t.Fatalf("result %d associated with wrong call: %#v", i, openAI)
				}
			}
		})
	}
}

func TestGeminiToOpenAIRequestIDlessCallsAcrossTurns(t *testing.T) {
	body := decodeTranslatorPayload(t, []byte(`{"contents":[
		{"role":"model","parts":[{"functionCall":{"name":"weather","args":{"city":"Tokyo"}}},{"functionCall":{"name":"weather","args":{"city":"Paris"}}}]},
		{"role":"user","parts":[{"functionResponse":{"name":"weather","response":{"city":"Tokyo"}}}]},
		{"role":"user","parts":[{"functionResponse":{"name":"weather","response":{"city":"Paris"}}}]},
		{"role":"model","parts":[{"functionCall":{"name":"weather","args":{"city":"London"}}}]},
		{"role":"user","parts":[{"functionResponse":{"name":"weather","response":{"city":"London"}}}]}
	]}`))
	out, err := GeminiToOpenAIRequest(body, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	messages := out["messages"].([]any)
	if len(messages) != 5 {
		t.Fatalf("unexpected history: %#v", out)
	}
	calls := messages[0].(map[string]any)["tool_calls"].([]any)
	calls = append(calls, messages[3].(map[string]any)["tool_calls"].([]any)...)
	ids := make(map[string]bool)
	for i, resultIndex := range []int{1, 2, 4} {
		id, _ := calls[i].(map[string]any)["id"].(string)
		if id == "" || ids[id] || messages[resultIndex].(map[string]any)["tool_call_id"] != id {
			t.Fatalf("pending same-name calls not consumed once: %#v", out)
		}
		ids[id] = true
	}
}

func TestOpenAIToGeminiSSEPreservesParallelIDs(t *testing.T) {
	for _, terminal := range []string{"[DONE]", `{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`} {
		trans := NewOpenAIToGeminiSSETranslator("gemini")
		for _, input := range []string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"weather","arguments":"{\"city\":"}},{"index":1,"id":"call_b","function":{"name":"weather","arguments":"{\"city\":"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"\"Paris\"}"}},{"index":0,"function":{"arguments":"\"Tokyo\"}"}}]}}]}`,
		} {
			if out, done, err := trans.TranslateChunk([]byte(input)); err != nil || done || len(out) != 0 {
				t.Fatalf("partial tool arguments must remain buffered: %s done=%v err=%v", out, done, err)
			}
		}
		out, done, err := trans.TranslateChunk([]byte(terminal))
		if err != nil || !done {
			t.Fatalf("unexpected terminal: done=%v err=%v", done, err)
		}
		payload, _, ok := ParseSSEDataLine(out)
		if !ok {
			t.Fatalf("missing SSE data: %s", out)
		}
		parts := decodeTranslatorPayload(t, payload)["candidates"].([]any)[0].(map[string]any)["content"].(map[string]any)["parts"].([]any)
		if len(parts) != 2 {
			t.Fatalf("parallel calls lost: %s", out)
		}
		for i, id := range []string{"call_a", "call_b"} {
			fc := parts[i].(map[string]any)["functionCall"].(map[string]any)
			if fc["id"] != id || fc["name"] != "weather" || fc["args"].(map[string]any)["city"] != []string{"Tokyo", "Paris"}[i] {
				t.Fatalf("streamed functionCall lost identity or arguments: %s", out)
			}
		}
	}
}
