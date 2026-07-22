package translator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Format represents an API format type.
type Format string

const (
	FormatOpenAI    Format = "openai"
	FormatAnthropic Format = "anthropic"
	FormatGemini    Format = "gemini"
)

// TranslateRequest converts an OpenAI-format request body to the target provider format.
// Returns the translated body as a map, ready for JSON marshaling.
func TranslateRequest(targetFormat Format, model string, body map[string]any, stream bool) (map[string]any, error) {
	switch targetFormat {
	case FormatOpenAI:
		// Already OpenAI format, just set model
		body["model"] = model
		return body, nil
	case FormatAnthropic:
		return openAIToClaude(model, body, stream)
	case FormatGemini:
		return openAIToGemini(model, body, stream)
	default:
		return nil, fmt.Errorf("unsupported target format: %s", targetFormat)
	}
}

// TranslateResponse converts a provider response body back to OpenAI format.
func TranslateResponse(sourceFormat Format, data []byte, model string) ([]byte, error) {
	switch sourceFormat {
	case FormatOpenAI:
		return data, nil
	case FormatAnthropic:
		return claudeToOpenAI(data, model)
	case FormatGemini:
		return geminiToOpenAI(data, model)
	default:
		return data, nil
	}
}

// TranslateSSEChunk converts a single SSE data line from provider format to OpenAI SSE format.
// Returns the OpenAI-format SSE data payload (without "data: " prefix).
func TranslateSSEChunk(sourceFormat Format, data []byte, model string) ([]byte, bool, error) {
	switch sourceFormat {
	case FormatOpenAI:
		return data, false, nil
	case FormatAnthropic:
		return claudeSSEToOpenAI(data, model)
	case FormatGemini:
		return geminiSSEToOpenAI(data, model)
	default:
		return data, false, nil
	}
}

// --- OpenAI → Claude ---

func openAIToClaude(model string, body map[string]any, stream bool) (map[string]any, error) {
	result := map[string]any{
		"model":      model,
		"max_tokens": getMaxTokens(body),
		"stream":     stream,
	}

	if temp, ok := body["temperature"]; ok {
		result["temperature"] = temp
	}

	// Extract messages
	messages, _ := body["messages"].([]any)
	var systemParts []string
	var claudeMessages []any

	for _, msgRaw := range messages {
		msg, ok := msgRaw.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)

		switch role {
		case "system":
			text := extractText(msg["content"])
			if text != "" {
				systemParts = append(systemParts, text)
			}
		case "user", "assistant":
			claudeMsg := convertMessageToClaude(msg)
			if claudeMsg != nil {
				claudeMessages = append(claudeMessages, claudeMsg)
			}
		case "tool":
			// Tool results become user messages with tool_result content
			toolResult := map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": msg["tool_call_id"],
						"content":     extractText(msg["content"]),
					},
				},
			}
			claudeMessages = append(claudeMessages, toolResult)
		}
	}

	if len(systemParts) > 0 {
		result["system"] = strings.Join(systemParts, "\n")
	}
	if len(claudeMessages) > 0 {
		result["messages"] = claudeMessages
	}

	// Convert tools
	if tools, ok := body["tools"].([]any); ok && len(tools) > 0 {
		var claudeTools []any
		for _, t := range tools {
			tool, ok := t.(map[string]any)
			if !ok {
				continue
			}
			fn, ok := tool["function"].(map[string]any)
			if !ok {
				continue
			}
			claudeTools = append(claudeTools, map[string]any{
				"name":         fn["name"],
				"description":  fn["description"],
				"input_schema": fn["parameters"],
			})
		}
		if len(claudeTools) > 0 {
			result["tools"] = claudeTools
		}
	}

	// Tool choice
	if tc, ok := body["tool_choice"]; ok {
		result["tool_choice"] = convertToolChoiceToClaude(tc)
	}

	return result, nil
}

func convertMessageToClaude(msg map[string]any) map[string]any {
	role, _ := msg["role"].(string)
	claudeRole := role
	if role == "tool" {
		claudeRole = "user"
	}

	content := msg["content"]
	var blocks []any

	// Handle string content
	if text, ok := content.(string); ok && text != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": text})
	} else if arr, ok := content.([]any); ok {
		for _, part := range arr {
			p, ok := part.(map[string]any)
			if !ok {
				continue
			}
			switch p["type"] {
			case "text":
				blocks = append(blocks, map[string]any{"type": "text", "text": p["text"]})
			case "image_url":
				if imgURL, ok := p["image_url"].(map[string]any); ok {
					url, _ := imgURL["url"].(string)
					if strings.HasPrefix(url, "data:") {
						mediaType, data := parseDataURI(url)
						blocks = append(blocks, map[string]any{
							"type":   "image",
							"source": map[string]any{"type": "base64", "media_type": mediaType, "data": data},
						})
					} else {
						blocks = append(blocks, map[string]any{
							"type":   "image",
							"source": map[string]any{"type": "url", "url": url},
						})
					}
				}
			}
		}
	}

	// Handle tool_calls in assistant messages
	if toolCalls, ok := msg["tool_calls"].([]any); ok {
		for _, tc := range toolCalls {
			tcMap, ok := tc.(map[string]any)
			if !ok {
				continue
			}
			fn, ok := tcMap["function"].(map[string]any)
			if !ok {
				continue
			}
			var input any
			if args, ok := fn["arguments"].(string); ok {
				json.Unmarshal([]byte(args), &input)
			}
			blocks = append(blocks, map[string]any{
				"type":  "tool_use",
				"id":    tcMap["id"],
				"name":  fn["name"],
				"input": input,
			})
		}
	}

	if len(blocks) == 0 {
		return nil
	}

	return map[string]any{
		"role":    claudeRole,
		"content": blocks,
	}
}

func convertToolChoiceToClaude(tc any) map[string]any {
	switch v := tc.(type) {
	case string:
		switch v {
		case "auto":
			return map[string]any{"type": "auto"}
		case "none":
			return map[string]any{"type": "none"}
		case "required":
			return map[string]any{"type": "any"}
		default:
			return map[string]any{"type": "auto"}
		}
	case map[string]any:
		if fn, ok := v["function"].(map[string]any); ok {
			return map[string]any{"type": "tool", "name": fn["name"]}
		}
		if t, ok := v["type"].(string); ok {
			switch t {
			case "auto", "any", "tool", "none":
				return v
			}
		}
		return map[string]any{"type": "auto"}
	default:
		return map[string]any{"type": "auto"}
	}
}

// --- OpenAI → Gemini ---

func openAIToGemini(model string, body map[string]any, stream bool) (map[string]any, error) {
	result := map[string]any{
		"contents": []any{},
	}

	genConfig := map[string]any{}
	if temp, ok := body["temperature"]; ok {
		genConfig["temperature"] = temp
	}
	if topP, ok := body["top_p"]; ok {
		genConfig["topP"] = topP
	}
	if mt := getMaxTokens(body); mt > 0 {
		genConfig["maxOutputTokens"] = mt
	}
	if len(genConfig) > 0 {
		result["generationConfig"] = genConfig
	}

	messages, _ := body["messages"].([]any)
	var contents []any

	for _, msgRaw := range messages {
		msg, ok := msgRaw.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)

		switch role {
		case "system":
			text := extractText(msg["content"])
			if text != "" {
				result["systemInstruction"] = map[string]any{
					"parts": []any{map[string]any{"text": text}},
				}
			}
		case "user":
			parts := convertContentToGeminiParts(msg["content"])
			if len(parts) > 0 {
				contents = append(contents, map[string]any{"role": "user", "parts": parts})
			}
		case "assistant":
			parts := convertContentToGeminiParts(msg["content"])
			// Handle tool calls
			if toolCalls, ok := msg["tool_calls"].([]any); ok {
				for _, tc := range toolCalls {
					tcMap, ok := tc.(map[string]any)
					if !ok {
						continue
					}
					fn, ok := tcMap["function"].(map[string]any)
					if !ok {
						continue
					}
					var args any
					if argsStr, ok := fn["arguments"].(string); ok {
						json.Unmarshal([]byte(argsStr), &args)
					}
					parts = append(parts, map[string]any{
						"functionCall": map[string]any{
							"name": fn["name"],
							"args": args,
						},
					})
				}
			}
			if len(parts) > 0 {
				contents = append(contents, map[string]any{"role": "model", "parts": parts})
			}
		case "tool":
			toolCallID, _ := msg["tool_call_id"].(string)
			content := extractText(msg["content"])
			var resp any
			json.Unmarshal([]byte(content), &resp)
			if resp == nil {
				resp = map[string]any{"result": content}
			}
			contents = append(contents, map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{
						"functionResponse": map[string]any{
							"name":     toolCallID,
							"response": map[string]any{"result": resp},
						},
					},
				},
			})
		}
	}

	result["contents"] = contents

	// Convert tools
	if tools, ok := body["tools"].([]any); ok && len(tools) > 0 {
		var declarations []any
		for _, t := range tools {
			tool, ok := t.(map[string]any)
			if !ok {
				continue
			}
			fn, ok := tool["function"].(map[string]any)
			if !ok {
				continue
			}
			declarations = append(declarations, map[string]any{
				"name":        fn["name"],
				"description": fn["description"],
				"parameters":  fn["parameters"],
			})
		}
		if len(declarations) > 0 {
			result["tools"] = []any{map[string]any{"functionDeclarations": declarations}}
		}
	}

	return result, nil
}

func convertContentToGeminiParts(content any) []any {
	var parts []any
	switch c := content.(type) {
	case string:
		if c != "" {
			parts = append(parts, map[string]any{"text": c})
		}
	case []any:
		for _, part := range c {
			p, ok := part.(map[string]any)
			if !ok {
				continue
			}
			switch p["type"] {
			case "text":
				parts = append(parts, map[string]any{"text": p["text"]})
			case "image_url":
				if imgURL, ok := p["image_url"].(map[string]any); ok {
					url, _ := imgURL["url"].(string)
					if strings.HasPrefix(url, "data:") {
						mediaType, data := parseDataURI(url)
						parts = append(parts, map[string]any{
							"inlineData": map[string]any{"mimeType": mediaType, "data": data},
						})
					}
				}
			}
		}
	}
	return parts
}

// --- Claude → OpenAI response ---

func claudeToOpenAI(data []byte, model string) ([]byte, error) {
	var claudeResp map[string]any
	if err := json.Unmarshal(data, &claudeResp); err != nil {
		return data, nil
	}

	// Build OpenAI response
	openAIResp := map[string]any{
		"id":      claudeResp["id"],
		"object":  "chat.completion",
		"model":   model,
		"choices": []any{},
	}

	// Convert content blocks to message
	var content strings.Builder
	var toolCalls []any

	if contentBlocks, ok := claudeResp["content"].([]any); ok {
		for _, block := range contentBlocks {
			b, ok := block.(map[string]any)
			if !ok {
				continue
			}
			switch b["type"] {
			case "text":
				if text, ok := b["text"].(string); ok {
					content.WriteString(text)
				}
			case "tool_use":
				args, _ := json.Marshal(b["input"])
				toolCalls = append(toolCalls, map[string]any{
					"id":   b["id"],
					"type": "function",
					"function": map[string]any{
						"name":      b["name"],
						"arguments": string(args),
					},
				})
			}
		}
	}

	message := map[string]any{
		"role":    "assistant",
		"content": content.String(),
	}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}

	finishReason := "stop"
	if stopReason, ok := claudeResp["stop_reason"].(string); ok {
		finishReason = claudeStopToOpenAI(stopReason)
	}

	openAIResp["choices"] = []any{
		map[string]any{
			"index":         0,
			"message":       message,
			"finish_reason": finishReason,
		},
	}

	// Usage
	if usage, ok := claudeResp["usage"].(map[string]any); ok {
		openAIResp["usage"] = map[string]any{
			"prompt_tokens":     usage["input_tokens"],
			"completion_tokens": usage["output_tokens"],
			"total_tokens":      addNumbers(usage["input_tokens"], usage["output_tokens"]),
		}
	}

	return json.Marshal(openAIResp)
}

// --- Gemini → OpenAI response ---

func geminiToOpenAI(data []byte, model string) ([]byte, error) {
	var geminiResp map[string]any
	if err := json.Unmarshal(data, &geminiResp); err != nil {
		return data, nil
	}

	openAIResp := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%s", model),
		"object":  "chat.completion",
		"model":   model,
		"choices": []any{},
	}

	var content strings.Builder
	var toolCalls []any

	if candidates, ok := geminiResp["candidates"].([]any); ok && len(candidates) > 0 {
		if candidate, ok := candidates[0].(map[string]any); ok {
			if contentParts, ok := candidate["content"].(map[string]any); ok {
				if parts, ok := contentParts["parts"].([]any); ok {
					for _, part := range parts {
						p, ok := part.(map[string]any)
						if !ok {
							continue
						}
						if text, ok := p["text"].(string); ok {
							content.WriteString(text)
						}
						if fc, ok := p["functionCall"].(map[string]any); ok {
							args, _ := json.Marshal(fc["args"])
							toolCalls = append(toolCalls, map[string]any{
								"id":   fmt.Sprintf("call_%v", fc["name"]),
								"type": "function",
								"function": map[string]any{
									"name":      fc["name"],
									"arguments": string(args),
								},
							})
						}
					}
				}
			}

			finishReason := "stop"
			if fr, ok := candidate["finishReason"].(string); ok {
				finishReason = geminiFinishToOpenAI(fr)
			}

			message := map[string]any{
				"role":    "assistant",
				"content": content.String(),
			}
			if len(toolCalls) > 0 {
				message["tool_calls"] = toolCalls
			}

			openAIResp["choices"] = []any{
				map[string]any{
					"index":         0,
					"message":       message,
					"finish_reason": finishReason,
				},
			}
		}
	}

	// Usage
	if usageMeta, ok := geminiResp["usageMetadata"].(map[string]any); ok {
		openAIResp["usage"] = map[string]any{
			"prompt_tokens":     usageMeta["promptTokenCount"],
			"completion_tokens": usageMeta["candidatesTokenCount"],
			"total_tokens":      usageMeta["totalTokenCount"],
		}
	}

	return json.Marshal(openAIResp)
}

// --- Claude SSE → OpenAI SSE ---

func claudeSSEToOpenAI(data []byte, model string) ([]byte, bool, error) {
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, false, nil
	}

	eventType, _ := event["type"].(string)

	switch eventType {
	case "content_block_delta":
		delta := map[string]any{}
		if d, ok := event["delta"].(map[string]any); ok {
			if d["type"] == "text_delta" {
				delta["content"] = d["text"]
			} else if d["type"] == "input_json_delta" {
				delta["tool_calls"] = []any{
					map[string]any{
						"index":    event["index"],
						"function": map[string]any{"arguments": d["partial_json"]},
					},
				}
			}
		}
		chunk := buildOpenAIChunk(model, delta, "")
		out, _ := json.Marshal(chunk)
		return out, false, nil

	case "content_block_start":
		if cb, ok := event["content_block"].(map[string]any); ok {
			if cb["type"] == "tool_use" {
				delta := map[string]any{
					"tool_calls": []any{
						map[string]any{
							"index": event["index"],
							"id":    cb["id"],
							"type":  "function",
							"function": map[string]any{
								"name":      cb["name"],
								"arguments": "",
							},
						},
					},
				}
				chunk := buildOpenAIChunk(model, delta, "")
				out, _ := json.Marshal(chunk)
				return out, false, nil
			}
		}
		return nil, false, nil

	case "message_delta":
		if d, ok := event["delta"].(map[string]any); ok {
			if stopReason, ok := d["stop_reason"].(string); ok {
				chunk := buildOpenAIChunk(model, map[string]any{}, claudeStopToOpenAI(stopReason))
				out, _ := json.Marshal(chunk)
				return out, false, nil
			}
		}
		return nil, false, nil

	case "message_stop":
		return []byte("[DONE]"), true, nil

	default:
		return nil, false, nil
	}
}

// --- Gemini SSE → OpenAI SSE ---

func geminiSSEToOpenAI(data []byte, model string) ([]byte, bool, error) {
	var geminiChunk map[string]any
	if err := json.Unmarshal(data, &geminiChunk); err != nil {
		return nil, false, nil
	}

	delta := map[string]any{}
	finishReason := ""

	if candidates, ok := geminiChunk["candidates"].([]any); ok && len(candidates) > 0 {
		if candidate, ok := candidates[0].(map[string]any); ok {
			if contentParts, ok := candidate["content"].(map[string]any); ok {
				if parts, ok := contentParts["parts"].([]any); ok {
					for _, part := range parts {
						p, ok := part.(map[string]any)
						if !ok {
							continue
						}
						if text, ok := p["text"].(string); ok {
							delta["content"] = text
						}
						if fc, ok := p["functionCall"].(map[string]any); ok {
							args, _ := json.Marshal(fc["args"])
							delta["tool_calls"] = []any{
								map[string]any{
									"index": 0,
									"id":    fmt.Sprintf("call_%v", fc["name"]),
									"type":  "function",
									"function": map[string]any{
										"name":      fc["name"],
										"arguments": string(args),
									},
								},
							}
						}
					}
				}
			}
			if fr, ok := candidate["finishReason"].(string); ok {
				finishReason = geminiFinishToOpenAI(fr)
			}
		}
	}

	if len(delta) == 0 && finishReason == "" {
		return nil, false, nil
	}

	chunk := buildOpenAIChunk(model, delta, finishReason)
	out, _ := json.Marshal(chunk)
	return out, false, nil
}

// --- Helpers ---

func buildOpenAIChunk(model string, delta map[string]any, finishReason string) map[string]any {
	choice := map[string]any{
		"index": 0,
		"delta": delta,
	}
	if finishReason != "" {
		choice["finish_reason"] = finishReason
	} else {
		choice["finish_reason"] = nil
	}
	return map[string]any{
		"id":      "chatcmpl-stream",
		"object":  "chat.completion.chunk",
		"model":   model,
		"choices": []any{choice},
	}
}

func extractText(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, p := range c {
			if pm, ok := p.(map[string]any); ok {
				if t, ok := pm["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func getMaxTokens(body map[string]any) int {
	if mt, ok := body["max_tokens"]; ok {
		switch v := mt.(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
	}
	return 4096
}

func parseDataURI(uri string) (mediaType, data string) {
	// data:image/png;base64,xxxxx
	if !strings.HasPrefix(uri, "data:") {
		return "", ""
	}
	rest := uri[5:]
	commaIdx := strings.Index(rest, ",")
	if commaIdx < 0 {
		return "", ""
	}
	meta := rest[:commaIdx]
	data = rest[commaIdx+1:]
	// meta = "image/png;base64"
	semiIdx := strings.Index(meta, ";")
	if semiIdx > 0 {
		mediaType = meta[:semiIdx]
	} else {
		mediaType = meta
	}
	return mediaType, data
}

func claudeStopToOpenAI(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	default:
		return "stop"
	}
}

func geminiFinishToOpenAI(reason string) string {
	switch reason {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY":
		return "content_filter"
	default:
		return "stop"
	}
}

func addNumbers(a, b any) any {
	af, aok := a.(float64)
	bf, bok := b.(float64)
	if aok && bok {
		return af + bf
	}
	return 0
}
