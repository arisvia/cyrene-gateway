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
