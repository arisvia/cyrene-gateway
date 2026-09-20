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

func TestOpenAIToResponsesSSETranslatorToolCalls(t *testing.T) {
	trans := NewOpenAIToResponsesSSETranslator("gpt-5.4")

	// First chunk with role only (triggers response.created)
	chunk1 := []byte(`{"id":"chatcmpl-tc","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`)
	s1, _, _ := trans.TranslateChunk(chunk1)
	if !strings.Contains(string(s1), "response.created") {
		t.Errorf("expected response.created, got: %s", string(s1))
	}

	// Tool call: first chunk with id, name, initial args
	chunk2 := []byte(`{"id":"chatcmpl-tc","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":"{\"lo"}}]},"finish_reason":null}]}`)
	s2, _, _ := trans.TranslateChunk(chunk2)
	str2 := string(s2)
	if !strings.Contains(str2, "response.output_item.added") {
		t.Errorf("expected output_item.added for function_call, got: %s", str2)
	}
	if !strings.Contains(str2, "function_call") {
		t.Errorf("expected function_call type, got: %s", str2)
	}
	if !strings.Contains(str2, "response.function_call_arguments.delta") {
		t.Errorf("expected function_call_arguments.delta, got: %s", str2)
	}

	// Tool call: subsequent args delta
	chunk3 := []byte(`{"id":"chatcmpl-tc","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"c\":\"NYC\"}"}}]},"finish_reason":null}]}`)
	s3, _, _ := trans.TranslateChunk(chunk3)
	str3 := string(s3)
	if !strings.Contains(str3, "response.function_call_arguments.delta") {
		t.Errorf("expected function_call_arguments.delta, got: %s", str3)
	}
	// Should NOT re-emit output_item.added
	if strings.Contains(str3, "response.output_item.added") {
		t.Errorf("should not re-emit output_item.added, got: %s", str3)
	}

	// [DONE] — should emit function_call_arguments.done + output_item.done + response.done
	sDone, doneFinal, _ := trans.TranslateChunk([]byte("[DONE]"))
	if !doneFinal {
		t.Fatalf("expected done on [DONE]")
	}
	strDone := string(sDone)
	if !strings.Contains(strDone, "response.function_call_arguments.done") {
		t.Errorf("expected function_call_arguments.done, got: %s", strDone)
	}
	if !strings.Contains(strDone, "response.output_item.done") {
		t.Errorf("expected output_item.done, got: %s", strDone)
	}
	if !strings.Contains(strDone, "response.done") {
		t.Errorf("expected response.done, got: %s", strDone)
	}
	if !strings.Contains(strDone, "get_weather") {
		t.Errorf("expected function name in done, got: %s", strDone)
	}
}
func TestOpenAIToResponsesParallelToolCalls(t *testing.T) {
	trans := NewOpenAIToResponsesSSETranslator("gpt-5.4")

	// Two parallel tool calls, NO prior message text (indices 0 and 1)
	chunk := []byte(`{"id":"chatcmpl-parallel","choices":[{"index":0,"delta":{"tool_calls":[` +
		`{"index":0,"id":"call_weather","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"SF\"}"}},` +
		`{"index":1,"id":"call_stock","type":"function","function":{"name":"get_stock","arguments":"{\"sym\":\"AAPL\"}"}}` +
		`]},"finish_reason":null}]}`)

	s, _, err := trans.TranslateChunk(chunk)
	if err != nil {
		t.Fatalf("TranslateChunk failed: %v", err)
	}

	lines := strings.Split(string(s), "\n\n")
	var weatherAddedIdx, stockAddedIdx = -1, -1
	var weatherDeltaIdx, stockDeltaIdx = -1, -1

	for _, block := range lines {
		block = strings.TrimSpace(block)
		if !strings.HasPrefix(block, "event: ") {
			continue
		}
		parts := strings.SplitN(block, "\ndata: ", 2)
		if len(parts) != 2 {
			continue
		}
		eventName := strings.TrimPrefix(parts[0], "event: ")
		var data map[string]any
		if err := json.Unmarshal([]byte(parts[1]), &data); err != nil {
			continue
		}

		if eventName == "response.output_item.added" {
			item := data["item"].(map[string]any)
			idx := int(data["output_index"].(float64))
			if item["id"] == "call_weather" {
				weatherAddedIdx = idx
			} else if item["id"] == "call_stock" {
				stockAddedIdx = idx
			}
		} else if eventName == "response.function_call_arguments.delta" {
			idx := int(data["output_index"].(float64))
			if data["call_id"] == "call_weather" {
				weatherDeltaIdx = idx
			} else if data["call_id"] == "call_stock" {
				stockDeltaIdx = idx
			}
		}
	}

	// Assert numerical output_index values: weather=0, stock=1 (assigned sequentially)
	if weatherAddedIdx != 0 {
		t.Errorf("expected call_weather output_index=0, got %d", weatherAddedIdx)
	}
	if stockAddedIdx != 1 {
		t.Errorf("expected call_stock output_index=1, got %d", stockAddedIdx)
	}
	if weatherDeltaIdx != 0 {
		t.Errorf("expected call_weather delta output_index=0, got %d", weatherDeltaIdx)
	}
	if stockDeltaIdx != 1 {
		t.Errorf("expected call_stock delta output_index=1, got %d", stockDeltaIdx)
	}

	// Now emit [DONE] and assert output_index values in done events and response.done
	sDone, doneFinal, _ := trans.TranslateChunk([]byte("[DONE]"))
	if !doneFinal {
		t.Fatalf("expected done on [DONE]")
	}

	var weatherDoneIdx, stockDoneIdx = -1, -1
	var responseDoneData map[string]any
	for _, block := range strings.Split(string(sDone), "\n\n") {
		block = strings.TrimSpace(block)
		parts := strings.SplitN(block, "\ndata: ", 2)
		if len(parts) != 2 {
			continue
		}
		eventName := strings.TrimPrefix(parts[0], "event: ")
		var data map[string]any
		if err := json.Unmarshal([]byte(parts[1]), &data); err != nil {
			continue
		}
		if eventName == "response.function_call_arguments.done" {
			idx := int(data["output_index"].(float64))
			if data["call_id"] == "call_weather" {
				weatherDoneIdx = idx
			} else if data["call_id"] == "call_stock" {
				stockDoneIdx = idx
			}
		} else if eventName == "response.done" {
			responseDoneData = data
		}
	}

	if weatherDoneIdx != 0 {
		t.Errorf("expected weatherDoneIdx=0, got %d", weatherDoneIdx)
	}
	if stockDoneIdx != 1 {
		t.Errorf("expected stockDoneIdx=1, got %d", stockDoneIdx)
	}

	if responseDoneData != nil {
		resp := responseDoneData["response"].(map[string]any)
		output := resp["output"].([]any)
		if len(output) != 2 {
			t.Fatalf("expected 2 output items in response.done, got %d", len(output))
		}
		item0 := output[0].(map[string]any)
		item1 := output[1].(map[string]any)
		if item0["id"] != "call_weather" {
			t.Errorf("expected output[0] to be call_weather, got %v", item0["id"])
		}
		if item1["id"] != "call_stock" {
			t.Errorf("expected output[1] to be call_stock, got %v", item1["id"])
		}
	} else {
		t.Errorf("missing response.done event")
	}
}

func TestOpenAIToResponsesToolCallThenContent(t *testing.T) {
	trans := NewOpenAIToResponsesSSETranslator("gpt-5.4")

	// Chunk 1: Tool call arrives first
	chunk1 := []byte(`{"id":"chatcmpl-tc-first","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search","arguments":"{\"q\":\"test\"}"}}]},"finish_reason":null}]}`)
	s1, _, _ := trans.TranslateChunk(chunk1)
	if !strings.Contains(string(s1), `"output_index":0`) {
		t.Errorf("expected tool call at output_index:0, got %s", string(s1))
	}

	// Chunk 2: Content arrives second
	chunk2 := []byte(`{"id":"chatcmpl-tc-first","choices":[{"index":0,"delta":{"content":"Thinking about this..."}}],"finish_reason":null}`)
	s2, _, _ := trans.TranslateChunk(chunk2)
	if !strings.Contains(string(s2), `"output_index":1`) {
		t.Errorf("expected content at output_index:1, got %s", string(s2))
	}

	// Chunk 3: [DONE]
	sDone, _, _ := trans.TranslateChunk([]byte("[DONE]"))
	strDone := string(sDone)
	// Tool call done must stay at 0
	if !strings.Contains(strDone, `"output_index":0`) {
		t.Errorf("expected tool call done at output_index:0, got: %s", strDone)
	}
	// Message done must stay at 1
	if !strings.Contains(strDone, `"output_index":1`) {
		t.Errorf("expected message done at output_index:1, got: %s", strDone)
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

func TestResponsesSSEToOpenAITranslator(t *testing.T) {
	tr := NewResponsesSSEToOpenAITranslator("gpt-5.4")

	deltaLine := []byte(`{"type":"response.output_text.delta","delta":"streaming chunk"}`)
	chunk, done, err := tr.TranslateChunk(deltaLine)
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
	finalChunk, isDone, err := tr.TranslateChunk(doneLine)
	if err != nil || !isDone {
		t.Fatalf("expected done true, got isDone=%v, err=%v", isDone, err)
	}
	if !strings.Contains(string(finalChunk), `"finish_reason":"stop"`) {
		t.Errorf("expected finish_reason stop, got: %s", string(finalChunk))
	}

	// Test tool call events
	tr2 := NewResponsesSSEToOpenAITranslator("gpt-5.4")
	itemAdded := []byte(`{"type":"response.output_item.added","item":{"type":"function_call","name":"get_weather","call_id":"call_123"}}`)
	chunk2, done2, _ := tr2.TranslateChunk(itemAdded)
	if done2 {
		t.Errorf("should not be done after output_item.added")
	}
	if !strings.Contains(string(chunk2), "get_weather") {
		t.Errorf("expected tool call name in chunk, got: %s", string(chunk2))
	}
	if !strings.Contains(string(chunk2), "call_123") {
		t.Errorf("expected call_id in chunk, got: %s", string(chunk2))
	}

	argsDelta := []byte(`{"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"loc\":\"NYC\"}"}`)
	chunk3, done3, _ := tr2.TranslateChunk(argsDelta)
	if done3 {
		t.Errorf("should not be done after arguments delta")
	}
	if !strings.Contains(string(chunk3), `"arguments"`) {
		t.Errorf("expected arguments in chunk, got: %s", string(chunk3))
	}

	doneLine2 := []byte(`{"type":"response.done","response":{"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`)
	finalChunk2, isDone2, _ := tr2.TranslateChunk(doneLine2)
	if !isDone2 {
		t.Fatalf("expected done true")
	}
	if !strings.Contains(string(finalChunk2), `"finish_reason":"tool_calls"`) {
		t.Errorf("expected finish_reason tool_calls, got: %s", string(finalChunk2))
	}
}

func TestResponsesSSEToOpenAIParallelToolCalls(t *testing.T) {
	// Scenario A: Text item at output_index 0, then 2 tool calls at output_index 1 and 2
	trA := NewResponsesSSEToOpenAITranslator("gpt-5.4")

	// 1. Text delta at output_index 0
	textDelta := []byte(`{"type":"response.output_text.delta","output_index":0,"delta":"Let me check that for you."}`)
	chunkText, _, err := trA.TranslateChunk(textDelta)
	if err != nil {
		t.Fatalf("TranslateChunk failed: %v", err)
	}
	if !strings.Contains(string(chunkText), "Let me check that for you.") {
		t.Errorf("expected text content in chunk, got: %s", string(chunkText))
	}

	// 2. Tool call 1 added at output_index 1
	tc1Added := []byte(`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","name":"get_weather","call_id":"call_1"}}`)
	chunkTC1, _, _ := trA.TranslateChunk(tc1Added)
	var c1 map[string]any
	if err := json.Unmarshal(chunkTC1, &c1); err != nil {
		t.Fatalf("unmarshal chunkTC1 failed: %v", err)
	}
	delta1 := c1["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)
	tcs1 := delta1["tool_calls"].([]any)[0].(map[string]any)
	if int(tcs1["index"].(float64)) != 0 {
		t.Errorf("expected tool_call 1 to have index 0, got %v", tcs1["index"])
	}

	// 3. Tool call 2 added at output_index 2
	tc2Added := []byte(`{"type":"response.output_item.added","output_index":2,"item":{"type":"function_call","name":"get_stock","call_id":"call_2"}}`)
	chunkTC2, _, _ := trA.TranslateChunk(tc2Added)
	var c2 map[string]any
	if err := json.Unmarshal(chunkTC2, &c2); err != nil {
		t.Fatalf("unmarshal chunkTC2 failed: %v", err)
	}
	delta2 := c2["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)
	tcs2 := delta2["tool_calls"].([]any)[0].(map[string]any)
	if int(tcs2["index"].(float64)) != 1 {
		t.Errorf("expected tool_call 2 to have index 1, got %v", tcs2["index"])
	}

	// 4. Arguments delta for tool call 1 at output_index 1
	tc1ArgDelta := []byte(`{"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"loc\":\"SF\"}"}`)
	chunkArg1, _, _ := trA.TranslateChunk(tc1ArgDelta)
	var cArg1 map[string]any
	if err := json.Unmarshal(chunkArg1, &cArg1); err != nil {
		t.Fatalf("unmarshal chunkArg1 failed: %v", err)
	}
	deltaArg1 := cArg1["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)
	tcsArg1 := deltaArg1["tool_calls"].([]any)[0].(map[string]any)
	if int(tcsArg1["index"].(float64)) != 0 {
		t.Errorf("expected args delta for tc1 to have index 0, got %v", tcsArg1["index"])
	}

	// 5. Arguments delta for tool call 2 at output_index 2
	tc2ArgDelta := []byte(`{"type":"response.function_call_arguments.delta","output_index":2,"delta":"{\"sym\":\"GOOG\"}"}`)
	chunkArg2, _, _ := trA.TranslateChunk(tc2ArgDelta)
	var cArg2 map[string]any
	if err := json.Unmarshal(chunkArg2, &cArg2); err != nil {
		t.Fatalf("unmarshal chunkArg2 failed: %v", err)
	}
	deltaArg2 := cArg2["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)
	tcsArg2 := deltaArg2["tool_calls"].([]any)[0].(map[string]any)
	if int(tcsArg2["index"].(float64)) != 1 {
		t.Errorf("expected args delta for tc2 to have index 1, got %v", tcsArg2["index"])
	}

	// Scenario B: Pure tool-calls response (no text item at all, starting at output_index 0)
	trB := NewResponsesSSEToOpenAITranslator("gpt-5.4")
	bAdded0 := []byte(`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","name":"fn0","call_id":"call_0"}}`)
	cB0, _, _ := trB.TranslateChunk(bAdded0)
	var mapB0 map[string]any
	json.Unmarshal(cB0, &mapB0)
	idxB0 := int(mapB0["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["index"].(float64))
	if idxB0 != 0 {
		t.Errorf("Scenario B: expected tool_call at output_index 0 to map to tool_call index 0, got %d", idxB0)
	}

	bDelta0 := []byte(`{"type":"response.function_call_arguments.delta","output_index":0,"delta":"{}"}`)
	cD0, _, _ := trB.TranslateChunk(bDelta0)
	var mapD0 map[string]any
	json.Unmarshal(cD0, &mapD0)
	idxD0 := int(mapD0["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["index"].(float64))
	if idxD0 != 0 {
		t.Errorf("Scenario B: expected delta at output_index 0 to map to tool_call index 0, got %d", idxD0)
	}

	bAdded1 := []byte(`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","name":"fn1","call_id":"call_1"}}`)
	cB1, _, _ := trB.TranslateChunk(bAdded1)
	var mapB1 map[string]any
	json.Unmarshal(cB1, &mapB1)
	idxB1 := int(mapB1["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["index"].(float64))
	if idxB1 != 1 {
		t.Errorf("Scenario B: expected tool_call at output_index 1 to map to tool_call index 1, got %d", idxB1)
	}
}

func TestResponsesToOpenAIRequest_ToolCallsAndOutputs(t *testing.T) {
	reqMap := map[string]any{
		"model": "gpt-5.4",
		"input": []any{
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": "What is the weather in Tokyo?",
			},
			map[string]any{
				"type":      "function_call",
				"call_id":   "call_tokyo_1",
				"name":      "get_weather",
				"arguments": `{"city":"Tokyo"}`,
			},
			map[string]any{
				"type":    "function_call_output",
				"call_id": "call_tokyo_1",
				"output":  `{"temp":22}`,
			},
		},
	}

	openAIReq, err := ResponsesToOpenAIRequest(reqMap)
	if err != nil {
		t.Fatalf("ResponsesToOpenAIRequest failed: %v", err)
	}

	msgs, ok := openAIReq["messages"].([]any)
	if !ok || len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %v", len(msgs))
	}

	m0 := msgs[0].(map[string]any)
	if m0["role"] != "user" || m0["content"] != "What is the weather in Tokyo?" {
		t.Errorf("unexpected m0: %v", m0)
	}

	m1 := msgs[1].(map[string]any)
	if m1["role"] != "assistant" {
		t.Errorf("expected assistant role for m1, got %v", m1["role"])
	}
	tcList, ok := m1["tool_calls"].([]any)
	if !ok || len(tcList) != 1 {
		t.Fatalf("expected 1 tool call in m1, got: %v", tcList)
	}
	tc := tcList[0].(map[string]any)
	if tc["id"] != "call_tokyo_1" {
		t.Errorf("expected call_tokyo_1, got %v", tc["id"])
	}
	fn := tc["function"].(map[string]any)
	if fn["name"] != "get_weather" || fn["arguments"] != `{"city":"Tokyo"}` {
		t.Errorf("unexpected function in m1: %v", fn)
	}

	m2 := msgs[2].(map[string]any)
	if m2["role"] != "tool" || m2["tool_call_id"] != "call_tokyo_1" || m2["content"] != `{"temp":22}` {
		t.Errorf("unexpected tool message m2: %v", m2)
	}
}

func TestOpenAIToResponsesRequest_ToolCallsAndOutputs(t *testing.T) {
	openAIBody := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Hi",
			},
			map[string]any{
				"role":    "assistant",
				"content": nil,
				"tool_calls": []any{
					map[string]any{
						"id":   "call_calc_2",
						"type": "function",
						"function": map[string]any{
							"name":      "calc",
							"arguments": `{"expr":"5*5"}`,
						},
					},
				},
			},
			map[string]any{
				"role":         "tool",
				"tool_call_id": "call_calc_2",
				"content":      "25",
			},
		},
	}

	respReq, err := OpenAIToResponsesRequest("gpt-5.4", openAIBody, false)
	if err != nil {
		t.Fatalf("OpenAIToResponsesRequest failed: %v", err)
	}

	inputItems, ok := respReq["input"].([]any)
	if !ok || len(inputItems) != 3 {
		t.Fatalf("expected 3 input items, got %v", len(inputItems))
	}

	it1 := inputItems[1].(map[string]any)
	if it1["type"] != "function_call" || it1["call_id"] != "call_calc_2" || it1["name"] != "calc" || it1["arguments"] != `{"expr":"5*5"}` {
		t.Errorf("unexpected function_call item: %v", it1)
	}

	it2 := inputItems[2].(map[string]any)
	if it2["type"] != "function_call_output" || it2["call_id"] != "call_calc_2" || it2["output"] != "25" {
		t.Errorf("unexpected function_call_output item: %v", it2)
	}
}
