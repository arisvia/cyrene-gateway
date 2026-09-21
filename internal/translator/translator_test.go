package translator

import (
	"encoding/json"
	"strings"
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

func TestOpenAIToClaude_PreserveCacheControl(t *testing.T) {
	ephemeral := map[string]any{"type": "ephemeral"}
	body := map[string]any{
		"model": "gpt-4",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type":          "text",
						"text":          "Cached user prompt",
						"cache_control": ephemeral,
					},
				},
			},
			map[string]any{
				"role":          "tool",
				"tool_call_id":  "call_123",
				"content":       "Tool result text",
				"cache_control": ephemeral,
			},
		},
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":          "get_weather",
					"parameters":    map[string]any{"type": "object"},
					"cache_control": ephemeral,
				},
			},
		},
	}

	result, err := openAIToClaude("claude-sonnet-4-20250514", body, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify tools preserve cache_control
	tools := result["tools"].([]any)
	tool0 := tools[0].(map[string]any)
	if tool0["cache_control"] == nil {
		t.Errorf("expected tool to preserve cache_control")
	}

	// Verify message text block preserves cache_control
	msgs := result["messages"].([]any)
	userMsg := msgs[0].(map[string]any)
	userParts := userMsg["content"].([]any)
	part0 := userParts[0].(map[string]any)
	if part0["cache_control"] == nil {
		t.Errorf("expected user text block to preserve cache_control")
	}

	// Verify tool result block preserves cache_control
	toolMsg := msgs[1].(map[string]any)
	toolParts := toolMsg["content"].([]any)
	toolBlock := toolParts[0].(map[string]any)
	if toolBlock["cache_control"] == nil {
		t.Errorf("expected tool_result block to preserve cache_control")
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

func TestCleanJSONSchemaForGemini(t *testing.T) {
	// Schema with unsupported keywords, anyOf, const, additionalProperties
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"$schema":              "http://json-schema.org/draft-07/schema#",
		"properties": map[string]any{
			"name": map[string]any{
				"type":      "string",
				"minLength": float64(1),
				"maxLength": float64(100),
				"format":    "email",
			},
			"status": map[string]any{
				"const": "active",
			},
			"value": map[string]any{
				"anyOf": []any{
					map[string]any{"type": "string"},
					map[string]any{"type": "null"},
				},
			},
			"nested": map[string]any{
				"properties": map[string]any{
					"count": map[string]any{
						"type":    []any{"integer", "null"},
						"default": float64(0),
					},
				},
			},
			"empty_obj": map[string]any{
				"type": "object",
			},
		},
		"required": []any{"name", "nonexistent"},
	}

	cleaned := cleanJSONSchemaForGemini(schema)

	// Top-level unsupported keywords removed
	if _, ok := cleaned["additionalProperties"]; ok {
		t.Fatal("additionalProperties should be removed")
	}
	if _, ok := cleaned["$schema"]; ok {
		t.Fatal("$schema should be removed")
	}

	props := cleaned["properties"].(map[string]any)

	// name: constraints removed, type preserved
	name := props["name"].(map[string]any)
	if _, ok := name["minLength"]; ok {
		t.Fatal("minLength should be removed")
	}
	if _, ok := name["format"]; ok {
		t.Fatal("format should be removed")
	}
	if name["type"] != "string" {
		t.Fatalf("expected name type=string, got %v", name["type"])
	}

	// status: const converted to enum
	status := props["status"].(map[string]any)
	if _, ok := status["const"]; ok {
		t.Fatal("const should be removed")
	}
	enumArr, ok := status["enum"].([]any)
	if !ok || len(enumArr) != 1 || enumArr[0] != "active" {
		t.Fatalf("expected enum=[active], got %v", status["enum"])
	}
	if status["type"] != "string" {
		t.Fatalf("expected status type=string (inferred for enum), got %v", status["type"])
	}

	// value: anyOf flattened to best non-null schema
	value := props["value"].(map[string]any)
	if _, ok := value["anyOf"]; ok {
		t.Fatal("anyOf should be flattened")
	}
	if value["type"] != "string" {
		t.Fatalf("expected value type=string (from anyOf), got %v", value["type"])
	}

	// nested.count: type array flattened, default removed
	nested := props["nested"].(map[string]any)
	if _, ok := nested["type"]; !ok {
		// nested has properties, so ensureObjectType should add type=object
		t.Fatal("nested should have type=object inferred")
	}
	nestedProps := nested["properties"].(map[string]any)
	count := nestedProps["count"].(map[string]any)
	if count["type"] != "integer" {
		t.Fatalf("expected count type=integer (from array), got %v", count["type"])
	}
	if _, ok := count["default"]; ok {
		t.Fatal("default should be removed")
	}

	// empty_obj: placeholder added
	emptyObj := props["empty_obj"].(map[string]any)
	emptyProps, ok := emptyObj["properties"].(map[string]any)
	if !ok || len(emptyProps) == 0 {
		t.Fatal("empty object should get placeholder properties")
	}
	if _, ok := emptyProps["reason"]; !ok {
		t.Fatal("placeholder should have 'reason' property")
	}

	// required: nonexistent field removed
	reqArr, ok := cleaned["required"].([]any)
	if !ok || len(reqArr) != 1 {
		t.Fatalf("expected required=[name], got %v", cleaned["required"])
	}
	if reqArr[0] != "name" {
		t.Fatalf("expected required[0]=name, got %v", reqArr[0])
	}
}

func TestCleanJSONSchemaEmptyAfterRefStrip(t *testing.T) {
	// Simulates a schema where $ref/$defs removal left an empty {} node (9router@e3e3e23)
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"config": map[string]any{}, // empty after $ref strip
			"name":   map[string]any{"type": "string"},
		},
	}

	cleaned := cleanJSONSchemaForGemini(schema)
	props := cleaned["properties"].(map[string]any)

	config := props["config"].(map[string]any)
	if config["type"] != "object" {
		t.Fatalf("empty schema should be promoted to type=object, got %v", config["type"])
	}
	configProps, ok := config["properties"].(map[string]any)
	if !ok || len(configProps) == 0 {
		t.Fatal("empty schema should get placeholder properties")
	}
	if _, ok := configProps["reason"]; !ok {
		t.Fatal("placeholder should have 'reason' property")
	}
}

func TestGeminiToolSchemaSanitized(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4",
		"messages": []any{
			map[string]any{"role": "user", "content": "test"},
		},
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "search",
					"description": "Search the web",
					"parameters": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]any{
							"query": map[string]any{
								"type":      "string",
								"minLength": float64(1),
							},
						},
						"required": []any{"query"},
					},
				},
			},
		},
	}

	result, err := openAIToGemini("gemini-pro", body, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tools := result["tools"].([]any)
	toolGroup := tools[0].(map[string]any)
	decls := toolGroup["functionDeclarations"].([]any)
	decl := decls[0].(map[string]any)
	params := decl["parameters"].(map[string]any)

	if _, ok := params["additionalProperties"]; ok {
		t.Fatal("additionalProperties should be stripped from Gemini tool schema")
	}
	props := params["properties"].(map[string]any)
	query := props["query"].(map[string]any)
	if _, ok := query["minLength"]; ok {
		t.Fatal("minLength should be stripped from Gemini tool schema")
	}
}

// TestCleanJSONSchemaPropertyNameMapSafety verifies that the schema cleaner does
// not corrupt property-name maps: parameters named "title", "format", "properties"
// etc. must survive cleaning (9router#2884 regression test).
func TestCleanJSONSchemaPropertyNameMapSafety(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"page_id": map[string]any{
				"type": "string",
			},
			// Parameter named "properties" — must NOT get bogus type:"object" injected into name map
			"properties": map[string]any{
				"type":                 "object",
				"description":          "Page property values",
				"additionalProperties": true,
			},
			// Parameter named "title" — must NOT be deleted as a schema keyword
			"title": map[string]any{
				"type":        "string",
				"description": "Issue title",
			},
			// Parameter named "format" — must NOT be deleted
			"format": map[string]any{
				"type":        "string",
				"description": "Output format",
			},
			// Parameter named "default" — must NOT be deleted
			"default": map[string]any{
				"type":        "boolean",
				"description": "Whether this is the default",
			},
		},
		"required": []any{"page_id"},
	}

	cleaned := cleanJSONSchemaForGemini(schema)

	props, ok := cleaned["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should still be a map")
	}

	// "title" parameter must survive
	if _, ok := props["title"]; !ok {
		t.Fatal("parameter named 'title' was incorrectly deleted from property-name map")
	}
	// "format" parameter must survive
	if _, ok := props["format"]; !ok {
		t.Fatal("parameter named 'format' was incorrectly deleted from property-name map")
	}
	// "default" parameter must survive
	if _, ok := props["default"]; !ok {
		t.Fatal("parameter named 'default' was incorrectly deleted from property-name map")
	}
	// "properties" parameter must survive
	propParam, ok := props["properties"].(map[string]any)
	if !ok {
		t.Fatal("parameter named 'properties' was incorrectly deleted")
	}
	// additionalProperties inside the "properties" parameter schema should be removed
	if _, ok := propParam["additionalProperties"]; ok {
		t.Fatal("additionalProperties inside a real schema node should be removed")
	}
	if propParam["type"] != "object" {
		t.Fatalf("properties param should keep type=object, got %v", propParam["type"])
	}

	// The property-name map itself must NOT have a bogus "type" key injected
	if _, ok := props["type"]; ok {
		t.Fatal("bogus 'type' key was injected into the property-name map")
	}

	// page_id should be intact
	pageID := props["page_id"].(map[string]any)
	if pageID["type"] != "string" {
		t.Fatalf("page_id type should be string, got %v", pageID["type"])
	}
}

func TestClaudeToOpenAIUsageCacheFold(t *testing.T) {
	// Claude's input_tokens excludes cache counters; the translated OpenAI
	// response must fold them in so prompt_tokens matches real prompt size
	// (9router@41606a37), matching usage.ExtractFromClaude canonical totals.
	claudeResp := map[string]any{
		"id": "msg_cache",
		"content": []any{
			map[string]any{"type": "text", "text": "ok"},
		},
		"stop_reason": "end_turn",
		"usage": map[string]any{
			"input_tokens":                float64(2012),
			"cache_read_input_tokens":     float64(5332),
			"cache_creation_input_tokens": float64(100),
			"output_tokens":               float64(7),
		},
	}
	data, _ := json.Marshal(claudeResp)

	result, err := claudeToOpenAI(data, "claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	json.Unmarshal(result, &resp)

	usage, ok := resp["usage"].(map[string]any)
	if !ok {
		t.Fatal("usage missing from translated response")
	}
	if usage["prompt_tokens"] != float64(2012+5332+100) {
		t.Errorf("prompt_tokens should fold cache counters, got %v", usage["prompt_tokens"])
	}
	if usage["completion_tokens"] != float64(7) {
		t.Errorf("completion_tokens mismatch, got %v", usage["completion_tokens"])
	}
	if usage["total_tokens"] != float64(2012+5332+100+7) {
		t.Errorf("total_tokens mismatch, got %v", usage["total_tokens"])
	}
	details, ok := usage["prompt_tokens_details"].(map[string]any)
	if !ok {
		t.Fatal("prompt_tokens_details missing")
	}
	if details["cached_tokens"] != float64(5332) {
		t.Errorf("cached_tokens mismatch, got %v", details["cached_tokens"])
	}
	if details["cache_creation_tokens"] != float64(100) {
		t.Errorf("cache_creation_tokens mismatch, got %v", details["cache_creation_tokens"])
	}
}

func TestCleanJSONSchemaGeminiArrayKeywords(t *testing.T) {
	// Keywords the Gemini schema proto has no field for must be stripped or the
	// whole request fails with "Unknown name ...: Cannot find field"
	// (9router@2abe8b85).
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type":        "array",
				"uniqueItems": true,
				"contains": map[string]any{
					"type": "string",
				},
				"unevaluatedItems": true,
			},
			"ratio": map[string]any{
				"type":       "number",
				"multipleOf": float64(0.5),
			},
			"extra": map[string]any{
				"type":                  "object",
				"unevaluatedProperties": false,
				"contentSchema":         map[string]any{"type": "string"},
			},
		},
	}

	cleaned := cleanJSONSchemaForGemini(schema)

	for _, kw := range []string{"unevaluatedProperties", "unevaluatedItems", "contentSchema"} {
		if _, ok := cleaned[kw]; ok {
			t.Fatalf("top-level %s should be removed", kw)
		}
	}

	props := cleaned["properties"].(map[string]any)

	items := props["items"].(map[string]any)
	for _, kw := range []string{"uniqueItems", "contains", "unevaluatedItems"} {
		if _, ok := items[kw]; ok {
			t.Fatalf("items.%s should be removed", kw)
		}
	}
	if items["type"] != "array" {
		t.Fatalf("items type should remain array, got %v", items["type"])
	}

	ratio := props["ratio"].(map[string]any)
	if _, ok := ratio["multipleOf"]; ok {
		t.Fatal("multipleOf should be removed")
	}

	extra := props["extra"].(map[string]any)
	for _, kw := range []string{"unevaluatedProperties", "contentSchema"} {
		if _, ok := extra[kw]; ok {
			t.Fatalf("extra.%s should be removed", kw)
		}
	}
}

func TestNormalizeGeminiContents(t *testing.T) {
	// Case 1: Adjacent user turns merged
	contents := []any{
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "hello"}}},
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "world"}}},
		map[string]any{"role": "model", "parts": []any{map[string]any{"text": "reply"}}},
	}
	normalized := normalizeGeminiContents(contents)
	if len(normalized) != 2 {
		t.Fatalf("expected 2 turns after merging adjacent user turns, got %d", len(normalized))
	}
	firstParts := normalized[0].(map[string]any)["parts"].([]any)
	if len(firstParts) != 2 {
		t.Errorf("expected 2 parts in merged user turn, got %d", len(firstParts))
	}

	// Case 2: Starts with model turn -> prepends user placeholder
	modelFirst := []any{
		map[string]any{"role": "model", "parts": []any{map[string]any{"text": "greeting"}}},
	}
	normModelFirst := normalizeGeminiContents(modelFirst)
	if len(normModelFirst) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(normModelFirst))
	}
	if normModelFirst[0].(map[string]any)["role"] != "user" {
		t.Errorf("expected initial role to be user, got %v", normModelFirst[0].(map[string]any)["role"])
	}

	// Case 3: Empty parts dropped
	emptyParts := []any{
		map[string]any{"role": "user", "parts": []any{}},
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "real"}}},
	}
	normEmpty := normalizeGeminiContents(emptyParts)
	if len(normEmpty) != 1 {
		t.Fatalf("expected 1 turn after dropping empty parts, got %d", len(normEmpty))
	}
}

func TestCleanJSONSchemaGeminiPrefixItemsAndArrayItems(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tupleSingle": map[string]any{
				"type": "array",
				"prefixItems": []any{
					map[string]any{"type": "string"},
				},
			},
			"tupleMultiple": map[string]any{
				"type": "array",
				"prefixItems": []any{
					map[string]any{"type": "string"},
					map[string]any{"type": "number"},
				},
			},
			"bareArray": map[string]any{
				"type": "array",
			},
		},
	}

	cleaned := cleanJSONSchemaForGemini(schema)
	props := cleaned["properties"].(map[string]any)

	// tupleSingle should convert prefixItems to items
	single := props["tupleSingle"].(map[string]any)
	if _, ok := single["prefixItems"]; ok {
		t.Fatal("prefixItems should be removed from tupleSingle")
	}
	if items, ok := single["items"].(map[string]any); !ok || items["type"] != "string" {
		t.Fatalf("expected items.type=string, got %v", single["items"])
	}

	// tupleMultiple should convert prefixItems to items (flattened to single type for Gemini)
	multi := props["tupleMultiple"].(map[string]any)
	if _, ok := multi["prefixItems"]; ok {
		t.Fatal("prefixItems should be removed from tupleMultiple")
	}
	if items, ok := multi["items"].(map[string]any); !ok || items["type"] == nil {
		t.Fatalf("expected items with valid type, got %v", multi["items"])
	}

	// bareArray should have default items added
	bare := props["bareArray"].(map[string]any)
	if items, ok := bare["items"].(map[string]any); !ok || items["type"] != "string" {
		t.Fatalf("expected default items.type=string on bareArray, got %v", bare["items"])
	}
}

func TestSalvageOrphanedToolResults(t *testing.T) {
	// Case: tool message has a tool_call_id not declared in assistant message (truncated away)
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "What did the tool return?",
			},
			map[string]any{
				"role": "assistant",
				"tool_calls": []any{
					map[string]any{
						"id":   "call_valid_1",
						"type": "function",
						"function": map[string]any{
							"name": "search",
						},
					},
				},
			},
			map[string]any{
				"role":         "tool",
				"tool_call_id": "call_orphaned_123",
				"content":      "Orphaned tool result payload",
			},
		},
	}

	result, err := TranslateRequest(FormatAnthropic, "claude-3-7-sonnet", body, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := result["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	// Third message should be salvaged as user text, NOT type: "tool_result"
	thirdMsg := msgs[2].(map[string]any)
	if thirdMsg["role"] != "user" {
		t.Errorf("expected role 'user' for salvaged tool result, got %v", thirdMsg["role"])
	}
	parts := thirdMsg["content"].([]any)
	firstPart := parts[0].(map[string]any)
	if firstPart["type"] != "text" {
		t.Errorf("expected type 'text' for salvaged content, got %v", firstPart["type"])
	}
	text := firstPart["text"].(string)
	if !strings.Contains(text, "call_orphaned_123") || !strings.Contains(text, "Orphaned tool result payload") {
		t.Errorf("unexpected text content: %s", text)
	}
}

func TestParseSSEDataLine(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		isDone bool
		wantOK bool
	}{
		{"with space", "data: {\"foo\":\"bar\"}\n", "{\"foo\":\"bar\"}", false, true},
		{"without space", "data:{\"foo\":\"bar\"}\r\n", "{\"foo\":\"bar\"}", false, true},
		{"done with space", "data: [DONE]\n", "", true, true},
		{"done without space", "data:[DONE]\r\n", "", true, true},
		{"empty line", "\r\n", "", false, false},
		{"event line", "event: ping\n", "", false, false},
		{"empty data line", "data: \n", "", false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, isDone, ok := ParseSSEDataLine([]byte(tc.input))
			if ok != tc.wantOK {
				t.Errorf("ParseSSEDataLine(%q) ok = %v, want %v", tc.input, ok, tc.wantOK)
			}
			if isDone != tc.isDone {
				t.Errorf("ParseSSEDataLine(%q) isDone = %v, want %v", tc.input, isDone, tc.isDone)
			}
			if string(got) != tc.want {
				t.Errorf("ParseSSEDataLine(%q) = %q, want %q", tc.input, string(got), tc.want)
			}

			gotStr, isDoneStr, okStr := ParseSSEDataLineString(tc.input)
			if okStr != tc.wantOK || isDoneStr != tc.isDone || gotStr != tc.want {
				t.Errorf("ParseSSEDataLineString(%q) = (%q, %v, %v), want (%q, %v, %v)",
					tc.input, gotStr, isDoneStr, okStr, tc.want, tc.isDone, tc.wantOK)
			}
		})
	}
}

func TestGeminiToOpenAIResponse_Thought(t *testing.T) {
	geminiResp := map[string]any{
		"candidates": []any{
			map[string]any{
				"content": map[string]any{
					"parts": []any{
						map[string]any{
							"text":    "Thinking through the solution step by step...",
							"thought": true,
						},
						map[string]any{
							"text": "The final answer is 42.",
						},
					},
				},
				"finishReason": "STOP",
			},
		},
		"usageMetadata": map[string]any{
			"promptTokenCount":     float64(10),
			"candidatesTokenCount": float64(20),
			"totalTokenCount":      float64(30),
		},
	}
	data, _ := json.Marshal(geminiResp)

	result, err := geminiToOpenAI(data, "gemini-2.5-pro")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var openAIResp map[string]any
	json.Unmarshal(result, &openAIResp)

	choices := openAIResp["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "The final answer is 42." {
		t.Errorf("expected content to only contain final answer, got: %v", msg["content"])
	}
	if msg["reasoning_content"] != "Thinking through the solution step by step..." {
		t.Errorf("expected reasoning_content to contain thought, got: %v", msg["reasoning_content"])
	}
}

func TestGeminiSSEToOpenAI_ThoughtAndToolCall(t *testing.T) {
	// 1. Thought delta chunk
	thoughtChunk := map[string]any{
		"candidates": []any{
			map[string]any{
				"content": map[string]any{
					"parts": []any{
						map[string]any{
							"text":    "Reasoning in stream...",
							"thought": true,
						},
					},
				},
			},
		},
	}
	d1, _ := json.Marshal(thoughtChunk)
	out1, done1, err1 := geminiSSEToOpenAI(d1, "gemini-2.5-flash")
	if err1 != nil || done1 {
		t.Fatalf("thought chunk failed: err=%v, done=%v", err1, done1)
	}
	var c1 map[string]any
	json.Unmarshal(out1, &c1)
	delta1 := c1["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)
	if delta1["reasoning_content"] != "Reasoning in stream..." {
		t.Errorf("expected delta.reasoning_content, got: %v", delta1)
	}
	if delta1["content"] != nil {
		t.Errorf("expected nil content in thought chunk, got: %v", delta1["content"])
	}

	// 2. Tool call delta chunk with id
	toolChunk := map[string]any{
		"candidates": []any{
			map[string]any{
				"content": map[string]any{
					"parts": []any{
						map[string]any{
							"functionCall": map[string]any{
								"id":   "call_gemini_123",
								"name": "lookup_stock",
								"args": map[string]any{"ticker": "GOOG"},
							},
						},
					},
				},
			},
		},
	}
	d2, _ := json.Marshal(toolChunk)
	out2, _, _ := geminiSSEToOpenAI(d2, "gemini-2.5-flash")
	var c2 map[string]any
	json.Unmarshal(out2, &c2)
	delta2 := c2["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)
	tcList := delta2["tool_calls"].([]any)
	tc0 := tcList[0].(map[string]any)
	if tc0["id"] != "call_gemini_123" {
		t.Errorf("expected call_gemini_123 id preserved, got: %v", tc0["id"])
	}
}

func TestOpenAIToGemini_MultiTurnToolCallAndThinking(t *testing.T) {
	body := map[string]any{
		"model":            "gemini-2.5-pro",
		"reasoning_effort": "medium",
		"messages": []any{
			map[string]any{"role": "user", "content": "What is the price of GOOG?"},
			map[string]any{
				"role":    "assistant",
				"content": nil,
				"tool_calls": []any{
					map[string]any{
						"id":   "call_goog_99",
						"type": "function",
						"function": map[string]any{
							"name":      "get_quote",
							"arguments": `{"symbol":"GOOG"}`,
						},
					},
				},
			},
			map[string]any{
				"role":         "tool",
				"tool_call_id": "call_goog_99",
				"content":      `{"price": 180.5}`,
			},
		},
	}

	req, err := openAIToGemini("gemini-2.5-pro", body, false)
	if err != nil {
		t.Fatalf("openAIToGemini failed: %v", err)
	}

	// Verify generationConfig thinkingConfig
	genConfig := req["generationConfig"].(map[string]any)
	thinkingConfig := genConfig["thinkingConfig"].(map[string]any)
	if thinkingConfig["includeThoughts"] != true {
		t.Errorf("expected includeThoughts=true in thinkingConfig, got: %v", thinkingConfig)
	}
	if thinkingConfig["thinkingBudget"] != 8192 {
		t.Errorf("expected thinkingBudget=8192 for medium, got: %v", thinkingConfig["thinkingBudget"])
	}

	// Verify multi-turn contents
	contents := req["contents"].([]any)
	if len(contents) != 3 {
		t.Fatalf("expected 3 turns, got %d", len(contents))
	}

	// Turn 1 (model tool call)
	mTurn := contents[1].(map[string]any)
	if mTurn["role"] != "model" {
		t.Errorf("expected role model, got %v", mTurn["role"])
	}
	fcPart := mTurn["parts"].([]any)[0].(map[string]any)["functionCall"].(map[string]any)
	if fcPart["name"] != "get_quote" || fcPart["id"] != "call_goog_99" {
		t.Errorf("unexpected functionCall: %v", fcPart)
	}

	// Turn 2 (user tool result)
	uTurn := contents[2].(map[string]any)
	if uTurn["role"] != "user" {
		t.Errorf("expected role user, got %v", uTurn["role"])
	}
	frPart := uTurn["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
	// CRITICAL: Name MUST match function name "get_quote", not the ID
	if frPart["name"] != "get_quote" {
		t.Errorf("expected functionResponse name to match function name 'get_quote', got: %v", frPart["name"])
	}
	if frPart["id"] != "call_goog_99" {
		t.Errorf("expected functionResponse id to match 'call_goog_99', got: %v", frPart["id"])
	}
	respMap := frPart["response"].(map[string]any)
	if respMap["price"] != 180.5 {
		t.Errorf("expected parsed response map with price=180.5, got: %v", respMap)
	}
}

func decodeTranslatorPayload(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON %q: %v", data, err)
	}
	return result
}

func TestSSETranslatorGeminiParallelParts(t *testing.T) {
	input := []byte(`{"candidates":[{"content":{"parts":[{"text":"Hello "},{"text":"think ","thought":true},{"functionCall":{"name":"weather","args":{"city":"Tokyo"}}},{"text":"world"},{"text":"again","thought":true},{"functionCall":{"name":"weather","args":{"city":"Paris"}}}]}}]}`)
	for _, stateful := range []bool{false, true} {
		var out []byte
		var done bool
		var err error
		if stateful {
			out, done, err = NewSSETranslator(FormatGemini, "gemini").TranslateChunk(input)
		} else {
			out, done, err = TranslateSSEChunk(FormatGemini, input, "gemini")
		}
		if err != nil || done {
			t.Fatalf("stateful=%v: done=%v err=%v", stateful, done, err)
		}
		chunk := decodeTranslatorPayload(t, out)
		delta := chunk["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)
		if delta["content"] != "Hello world" || delta["reasoning_content"] != "think again" {
			t.Fatalf("parts overwritten: %s", out)
		}
		calls := delta["tool_calls"].([]any)
		if len(calls) != 2 {
			t.Fatalf("expected two parallel calls: %s", out)
		}
		ids := make(map[string]bool)
		for i, raw := range calls {
			call := raw.(map[string]any)
			id, _ := call["id"].(string)
			if id == "" || ids[id] || call["index"] != float64(i) {
				t.Fatalf("calls must have unique IDs and indices: %s", out)
			}
			ids[id] = true
			fn := call["function"].(map[string]any)
			args := decodeTranslatorPayload(t, []byte(fn["arguments"].(string)))
			if fn["name"] != "weather" || args["city"] != []string{"Tokyo", "Paris"}[i] {
				t.Fatalf("call arguments lost: %s", out)
			}
		}
	}
}

func TestSSETranslatorGeminiAcrossChunks(t *testing.T) {
	trans := NewSSETranslator(FormatGemini, "gemini")
	inputs := []string{
		`{"candidates":[{"content":{"parts":[{"functionCall":{"id":"call_a","name":"weather","args":""}}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"functionCall":{"id":"call_b","name":"weather","args":{}}}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"functionCall":{"id":"call_a","args":"{\"city\":\"Tokyo\"}"}}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"weather","args":{}}}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"weather","args":{}}}]}}]}`,
	}
	indices := []float64{0, 1, 0, 2, 3}
	ids := make(map[string]bool)
	for i, input := range inputs {
		out, done, err := trans.TranslateChunk([]byte(input))
		if err != nil || done {
			t.Fatalf("chunk %d: done=%v err=%v", i, done, err)
		}
		chunk := decodeTranslatorPayload(t, out)
		delta := chunk["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)
		call := delta["tool_calls"].([]any)[0].(map[string]any)
		if call["index"] != indices[i] {
			t.Fatalf("chunk %d: wrong stable index: %s", i, out)
		}
		if i == 2 {
			fn := call["function"].(map[string]any)
			if call["id"] != nil || fn["name"] != nil || fn["arguments"] != `{"city":"Tokyo"}` {
				t.Fatalf("continuation must not redeclare the call or quote arguments: %s", out)
			}
			continue
		}
		id, _ := call["id"].(string)
		if id == "" || ids[id] {
			t.Fatalf("missing or duplicate ID: %s", out)
		}
		ids[id] = true
		if i < 2 && id != []string{"call_a", "call_b"}[i] {
			t.Fatalf("upstream ID changed: %s", out)
		}
	}
	out, done, err := trans.TranslateChunk([]byte(`{"candidates":[{"finishReason":"STOP"}]}`))
	if err != nil || done {
		t.Fatalf("finish must allow following usage: done=%v err=%v", done, err)
	}
	if decodeTranslatorPayload(t, out)["choices"].([]any)[0].(map[string]any)["finish_reason"] != "tool_calls" {
		t.Fatalf("tool finish reason lost: %s", out)
	}
	other := NewSSETranslator(FormatGemini, "gemini")
	out, _, _ = other.TranslateChunk([]byte(inputs[1]))
	delta := decodeTranslatorPayload(t, out)["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)
	if delta["tool_calls"].([]any)[0].(map[string]any)["index"] != float64(0) {
		t.Fatal("tool indices leaked across requests")
	}
}

func TestSSETranslatorClaudeUsage(t *testing.T) {
	trans := NewSSETranslator(FormatAnthropic, "claude")
	cases := []struct {
		input              string
		prompt, completion int
		usageOnly          bool
	}{
		{`{"type":"message_start","message":{"usage":{"input_tokens":10,"output_tokens":0,"cache_read_input_tokens":3,"cache_creation_input_tokens":2}}}`, 15, 0, false},
		{`{"type":"message_delta","usage":{"output_tokens":2}}`, 15, 2, true},
		{`{"type":"message_delta","usage":{"input_tokens":12,"cache_read_input_tokens":4,"output_tokens":3}}`, 18, 3, true},
		{`{"type":"message_delta","usage":{"output_tokens":3}}`, 18, 3, true},
		{`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":4}}`, 18, 4, false},
	}
	for i, tc := range cases {
		out, done, err := trans.TranslateChunk([]byte(tc.input))
		if err != nil || done {
			t.Fatalf("chunk %d: done=%v err=%v", i, done, err)
		}
		chunk := decodeTranslatorPayload(t, out)
		usage := chunk["usage"].(map[string]any)
		if usage["prompt_tokens"] != float64(tc.prompt) || usage["completion_tokens"] != float64(tc.completion) || usage["total_tokens"] != float64(tc.prompt+tc.completion) {
			t.Fatalf("chunk %d lost cumulative usage: %s", i, out)
		}
		details := usage["prompt_tokens_details"].(map[string]any)
		if details["cache_creation_tokens"] != float64(2) {
			t.Fatalf("cache usage disappeared: %s", out)
		}
		if tc.usageOnly && len(chunk["choices"].([]any)) != 0 {
			t.Fatalf("usage-only event has choices: %s", out)
		}
		if i == 0 {
			text, done, err := trans.TranslateChunk([]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`))
			if err != nil || done || !strings.Contains(string(text), `"content":"hello"`) {
				t.Fatalf("text must stream before final usage: %s done=%v err=%v", text, done, err)
			}
		}
	}
	out, done, err := trans.TranslateChunk([]byte(`{"type":"message_stop"}`))
	if err != nil || !done || string(out) != "[DONE]" {
		t.Fatalf("unexpected terminal chunk: %s done=%v err=%v", out, done, err)
	}
	other := NewSSETranslator(FormatAnthropic, "claude")
	out, _, _ = other.TranslateChunk([]byte(cases[1].input))
	if decodeTranslatorPayload(t, out)["usage"].(map[string]any)["prompt_tokens"] != float64(0) {
		t.Fatal("usage leaked across requests")
	}
}

func TestSSETranslatorGeminiUsage(t *testing.T) {
	trans := NewSSETranslator(FormatGemini, "gemini")
	for _, input := range []string{
		`{"candidates":[{"content":{"parts":[{"text":"hello"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2,"totalTokenCount":15,"thoughtsTokenCount":3,"cachedContentTokenCount":4}}`,
		`{"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2,"totalTokenCount":15,"thoughtsTokenCount":3,"cachedContentTokenCount":4}}`,
		`{"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2,"thoughtsTokenCount":3,"cachedContentTokenCount":4}}`,
	} {
		out, done, err := trans.TranslateChunk([]byte(input))
		if err != nil || done {
			t.Fatalf("unexpected result: done=%v err=%v", done, err)
		}
		chunk := decodeTranslatorPayload(t, out)
		usage := chunk["usage"].(map[string]any)
		if usage["prompt_tokens"] != float64(10) || usage["completion_tokens"] != float64(5) || usage["total_tokens"] != float64(15) {
			t.Fatalf("usage lost: %s", out)
		}
		if usage["prompt_tokens_details"].(map[string]any)["cached_tokens"] != float64(4) || usage["completion_tokens_details"].(map[string]any)["reasoning_tokens"] != float64(3) {
			t.Fatalf("usage details lost: %s", out)
		}
		if !strings.Contains(input, "candidates") && len(chunk["choices"].([]any)) != 0 {
			t.Fatalf("usage-only chunk must have no choices: %s", out)
		}
	}
}

func TestSSETranslatorErrors(t *testing.T) {
	cases := []struct {
		name                      string
		format                    Format
		input, message, errorType string
	}{
		{"openai", FormatOpenAI, `{"error":{"message":"quota","type":"rate_limit_error","code":"quota","param":"model"}}`, "quota", "rate_limit_error"},
		{"anthropic", FormatAnthropic, `{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`, "busy", "overloaded_error"},
		{"gemini", FormatGemini, `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"quota"}}`, "quota", "RESOURCE_EXHAUSTED"},
		{"responses", FormatResponses, `{"type":"response.failed","response":{"error":{"code":"server_error","message":"failed"}}}`, "failed", "upstream_error"},
		{"responses_error", FormatResponses, `{"type":"error","message":"invalid","code":"invalid_request"}`, "invalid", "error"},
		{"string_error", FormatGemini, `{"error":"unavailable"}`, "unavailable", "upstream_error"},
		{"missing_detail", FormatResponses, `{"type":"response.failed","response":{}}`, "Upstream response failed", "upstream_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trans := NewSSETranslator(tc.format, "model")
			content := map[Format]string{
				FormatOpenAI:    `{"choices":[{"delta":{"content":"hello"}}]}`,
				FormatAnthropic: `{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`,
				FormatGemini:    `{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`,
				FormatResponses: `{"type":"response.output_text.delta","delta":"hello"}`,
			}
			if out, done, err := trans.TranslateChunk([]byte(content[tc.format])); err != nil || done || !strings.Contains(string(out), "hello") {
				t.Fatalf("text before error must stream immediately: %s done=%v err=%v", out, done, err)
			}
			out, done, err := trans.TranslateChunk([]byte(tc.input))
			if err != nil || !done {
				t.Fatalf("error must terminate with payload: done=%v err=%v", done, err)
			}
			payload := decodeTranslatorPayload(t, out)
			detail, ok := payload["error"].(map[string]any)
			if !ok || detail["message"] != tc.message || detail["type"] != tc.errorType || payload["choices"] != nil {
				t.Fatalf("unexpected error payload: %s", out)
			}
			if tc.name == "openai" && (detail["code"] != "quota" || detail["param"] != "model") {
				t.Fatalf("OpenAI error metadata lost: %s", out)
			}
			for _, input := range []string{tc.input, "[DONE]"} {
				out, done, err = trans.TranslateChunk([]byte(input))
				if err != nil || !done || len(out) != 0 {
					t.Fatalf("terminal payload emitted twice: %s done=%v err=%v", out, done, err)
				}
			}
			out, done, err = TranslateSSEChunk(tc.format, []byte(tc.input), "model")
			if err != nil || !done || len(out) == 0 {
				t.Fatalf("compatibility entry point dropped error: %s done=%v err=%v", out, done, err)
			}
		})
	}
}

func TestSSETranslatorResponsesState(t *testing.T) {
	trans := NewSSETranslator(FormatResponses, "model")
	for _, input := range []string{
		`{"type":"response.output_item.added","output_index":3,"item":{"type":"function_call","call_id":"call_a","name":"weather"}}`,
		`{"type":"response.output_item.added","output_index":9,"item":{"type":"function_call","call_id":"call_b","name":"weather"}}`,
	} {
		if _, done, err := trans.TranslateChunk([]byte(input)); err != nil || done {
			t.Fatalf("unexpected declaration: done=%v err=%v", done, err)
		}
	}
	out, done, err := trans.TranslateChunk([]byte(`{"type":"response.function_call_arguments.delta","output_index":9,"delta":"{}"}`))
	if err != nil || done {
		t.Fatalf("unexpected arguments: done=%v err=%v", done, err)
	}
	delta := decodeTranslatorPayload(t, out)["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)
	if delta["tool_calls"].([]any)[0].(map[string]any)["index"] != float64(1) {
		t.Fatalf("Responses delegate was recreated per chunk: %s", out)
	}
}

func TestOpenAIToGeminiMultipleSystemMessages(t *testing.T) {
	body := decodeTranslatorPayload(t, []byte(`{"messages":[{"role":"system","content":"first rule"},{"role":"system","content":[{"type":"text","text":"second rule"}]},{"role":"user","content":[{"type":"text","text":"look"},{"type":"image_url","image_url":{"url":"data:image/png;base64,aGVsbG8="}}]}]}`))
	out, err := openAIToGemini("gemini", body, false)
	if err != nil {
		t.Fatal(err)
	}
	parts := out["systemInstruction"].(map[string]any)["parts"].([]any)
	if len(parts) != 2 || parts[0].(map[string]any)["text"] != "first rule" || parts[1].(map[string]any)["text"] != "second rule" {
		t.Fatalf("system messages overwritten: %#v", out)
	}
	parts = out["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	if len(parts) != 2 || parts[0].(map[string]any)["text"] != "look" || parts[1].(map[string]any)["inlineData"].(map[string]any)["data"] != "aGVsbG8=" {
		t.Fatalf("ordinary image/text behavior changed: %#v", out)
	}
}

func TestGeminiToOpenAIResponseParallelIDs(t *testing.T) {
	out, err := geminiToOpenAI([]byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"weather","args":{}}},{"functionCall":{"name":"weather","args":{}}}]}}]}`), "gemini")
	if err != nil {
		t.Fatal(err)
	}
	msg := decodeTranslatorPayload(t, out)["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	calls := msg["tool_calls"].([]any)
	if len(calls) != 2 || calls[0].(map[string]any)["id"] == calls[1].(map[string]any)["id"] {
		t.Fatalf("same-name parallel calls must have distinct IDs: %s", out)
	}
}

func TestSSETranslatorDone(t *testing.T) {
	for _, format := range []Format{FormatOpenAI, FormatAnthropic, FormatGemini, FormatResponses} {
		trans := NewSSETranslator(format, "model")
		out, done, err := trans.TranslateChunk([]byte(" [DONE]\r\n"))
		if err != nil || !done || string(out) != "[DONE]" {
			t.Fatalf("%s terminal marker: %s done=%v err=%v", format, out, done, err)
		}
		out, done, err = trans.TranslateChunk([]byte("[DONE]"))
		if err != nil || !done || len(out) != 0 {
			t.Fatalf("%s repeated marker: %s done=%v err=%v", format, out, done, err)
		}
	}
}
