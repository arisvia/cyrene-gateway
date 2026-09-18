package translator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ClaudeToOpenAIRequest converts an Anthropic /v1/messages JSON payload into an OpenAI /v1/chat/completions JSON payload.
func ClaudeToOpenAIRequest(claudeBody map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	// Model
	if m, ok := claudeBody["model"].(string); ok {
		result["model"] = m
	}

	// Stream
	if stream, ok := claudeBody["stream"].(bool); ok {
		result["stream"] = stream
	}

	// Temperature
	if temp, ok := claudeBody["temperature"]; ok {
		result["temperature"] = temp
	}

	// TopP
	if topP, ok := claudeBody["top_p"]; ok {
		result["top_p"] = topP
	}

	// MaxTokens
	if maxTokens, ok := claudeBody["max_tokens"]; ok {
		result["max_tokens"] = maxTokens
	}

	// StopSequences
	if stop, ok := claudeBody["stop_sequences"]; ok {
		result["stop"] = stop
	}

	// Messages conversion
	var openAIMessages []any

	// 1. System prompt
	if sysRaw, ok := claudeBody["system"]; ok {
		switch sys := sysRaw.(type) {
		case string:
			if sys != "" {
				openAIMessages = append(openAIMessages, map[string]any{
					"role":    "system",
					"content": sys,
				})
			}
		case []any:
			var parts []string
			for _, item := range sys {
				if m, ok := item.(map[string]any); ok {
					if text, ok := m["text"].(string); ok && text != "" {
						parts = append(parts, text)
					}
				}
			}
			if len(parts) > 0 {
				openAIMessages = append(openAIMessages, map[string]any{
					"role":    "system",
					"content": strings.Join(parts, "\n\n"),
				})
			}
		}
	}

	// 2. Chat messages
	if msgsRaw, ok := claudeBody["messages"].([]any); ok {
		for _, mRaw := range msgsRaw {
			m, ok := mRaw.(map[string]any)
			if !ok {
				continue
			}
			role, _ := m["role"].(string)
			if role == "" {
				role = "user"
			}

			contentRaw := m["content"]
			switch content := contentRaw.(type) {
			case string:
				openAIMessages = append(openAIMessages, map[string]any{
					"role":    role,
					"content": content,
				})
			case []any:
				// Content blocks
				var textParts []string
				var openAIParts []any
				var toolCalls []any
				var toolResults []map[string]any

				for _, blockRaw := range content {
					block, ok := blockRaw.(map[string]any)
					if !ok {
						continue
					}
					bType, _ := block["type"].(string)
					switch bType {
					case "text":
						if t, ok := block["text"].(string); ok {
							textParts = append(textParts, t)
							openAIParts = append(openAIParts, map[string]any{
								"type": "text",
								"text": t,
							})
						}
					case "image":
						if src, ok := block["source"].(map[string]any); ok {
							srcType, _ := src["type"].(string)
							mediaType, _ := src["media_type"].(string)
							data, _ := src["data"].(string)
							if srcType == "base64" && data != "" {
								dataURL := fmt.Sprintf("data:%s;base64,%s", mediaType, data)
								openAIParts = append(openAIParts, map[string]any{
									"type": "image_url",
									"image_url": map[string]any{
										"url": dataURL,
									},
								})
							}
						}
					case "tool_use":
						id, _ := block["id"].(string)
						name, _ := block["name"].(string)
						inputRaw := block["input"]
						argsStr := "{}"
						if inputRaw != nil {
							if bytes, err := json.Marshal(inputRaw); err == nil {
								argsStr = string(bytes)
							}
						}
						toolCalls = append(toolCalls, map[string]any{
							"id":   id,
							"type": "function",
							"function": map[string]any{
								"name":      name,
								"arguments": argsStr,
							},
						})
					case "tool_result":
						toolCallID, _ := block["tool_use_id"].(string)
						resContent := extractText(block["content"])
						toolResults = append(toolResults, map[string]any{
							"role":         "tool",
							"tool_call_id": toolCallID,
							"content":      resContent,
						})
					}
				}

				if len(toolResults) > 0 {
					for _, tr := range toolResults {
						openAIMessages = append(openAIMessages, tr)
					}
				} else if len(toolCalls) > 0 {
					msgObj := map[string]any{
						"role":       role,
						"tool_calls": toolCalls,
					}
					if len(textParts) > 0 {
						msgObj["content"] = strings.Join(textParts, "\n\n")
					}
					openAIMessages = append(openAIMessages, msgObj)
				} else if len(openAIParts) > 0 {
					hasNonText := false
					for _, p := range openAIParts {
						if pMap, ok := p.(map[string]any); ok && pMap["type"] != "text" {
							hasNonText = true
							break
						}
					}
					if hasNonText {
						openAIMessages = append(openAIMessages, map[string]any{
							"role":    role,
							"content": openAIParts,
						})
					} else {
						openAIMessages = append(openAIMessages, map[string]any{
							"role":    role,
							"content": strings.Join(textParts, "\n\n"),
						})
					}
				}
			}
		}
	}

	result["messages"] = openAIMessages

	// 3. Tools conversion
	if toolsRaw, ok := claudeBody["tools"].([]any); ok {
		var openAITools []any
		for _, tRaw := range toolsRaw {
			t, ok := tRaw.(map[string]any)
			if !ok {
				continue
			}
			name, _ := t["name"].(string)
			desc, _ := t["description"].(string)
			schema, _ := t["input_schema"]
			if schema == nil {
				schema = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			openAITools = append(openAITools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        name,
					"description": desc,
					"parameters":  schema,
				},
			})
		}
		if len(openAITools) > 0 {
			result["tools"] = openAITools
		}
	}

	// 4. Tool Choice conversion
	if tcRaw, ok := claudeBody["tool_choice"].(map[string]any); ok {
		tcType, _ := tcRaw["type"].(string)
		switch tcType {
		case "auto":
			result["tool_choice"] = "auto"
		case "any":
			result["tool_choice"] = "required"
		case "tool":
			if name, ok := tcRaw["name"].(string); ok && name != "" {
				result["tool_choice"] = map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": name,
					},
				}
			}
		}
	}

	return result, nil
}

// OpenAIToClaudeResponse converts an OpenAI ChatCompletion JSON response into an Anthropic Message JSON response.
func OpenAIToClaudeResponse(data []byte, model string) ([]byte, error) {
	var openAIResp map[string]any
	if err := json.Unmarshal(data, &openAIResp); err != nil {
		return nil, err
	}

	id, _ := openAIResp["id"].(string)
	if id == "" {
		id = fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	if !strings.HasPrefix(id, "msg_") {
		id = "msg_" + id
	}

	respModel, _ := openAIResp["model"].(string)
	if respModel == "" {
		respModel = model
	}

	var contentBlocks []any
	stopReason := "end_turn"

	if choices, ok := openAIResp["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if fr, ok := choice["finish_reason"].(string); ok {
				switch fr {
				case "stop":
					stopReason = "end_turn"
				case "length":
					stopReason = "max_tokens"
				case "tool_calls", "function_call":
					stopReason = "tool_use"
				default:
					stopReason = "end_turn"
				}
			}

			if msg, ok := choice["message"].(map[string]any); ok {
				if text, ok := msg["content"].(string); ok && text != "" {
					contentBlocks = append(contentBlocks, map[string]any{
						"type": "text",
						"text": text,
					})
				}

				if tcList, ok := msg["tool_calls"].([]any); ok {
					for _, tcRaw := range tcList {
						if tc, ok := tcRaw.(map[string]any); ok {
							tcID, _ := tc["id"].(string)
							if fn, ok := tc["function"].(map[string]any); ok {
								fnName, _ := fn["name"].(string)
								argsStr, _ := fn["arguments"].(string)
								var parsedArgs any
								if err := json.Unmarshal([]byte(argsStr), &parsedArgs); err != nil {
									parsedArgs = map[string]any{}
								}
								contentBlocks = append(contentBlocks, map[string]any{
									"type":  "tool_use",
									"id":    tcID,
									"name":  fnName,
									"input": parsedArgs,
								})
							}
						}
					}
				}
			}
		}
	}

	inTokens := 0
	outTokens := 0
	if usageMap, ok := openAIResp["usage"].(map[string]any); ok {
		if pt, ok := usageMap["prompt_tokens"].(float64); ok {
			inTokens = int(pt)
		}
		if ct, ok := usageMap["completion_tokens"].(float64); ok {
			outTokens = int(ct)
		}
	}

	claudeResp := map[string]any{
		"id":            id,
		"type":          "message",
		"role":          "assistant",
		"model":         respModel,
		"content":       contentBlocks,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":  inTokens,
			"output_tokens": outTokens,
		},
	}

	return json.Marshal(claudeResp)
}

// OpenAIToClaudeSSETranslator manages streaming state to translate OpenAI SSE lines into Anthropic SSE lines.
type OpenAIToClaudeSSETranslator struct {
	Model        string
	started      bool
	msgID        string
	blockStarted bool
	blockIndex   int
	outputTokens int
	finishReason string
	inTokens     int
}

// NewOpenAIToClaudeSSETranslator creates a new stateful translator.
func NewOpenAIToClaudeSSETranslator(model string) *OpenAIToClaudeSSETranslator {
	return &OpenAIToClaudeSSETranslator{
		Model: model,
		msgID: fmt.Sprintf("msg_%d", time.Now().UnixNano()),
	}
}

// TranslateChunk takes an OpenAI SSE data payload (e.g. `{"id":"...","choices":[{"delta":{"content":"hi"}}]}` or `[DONE]`)
// and returns the corresponding Anthropic SSE events formatted as raw SSE text.
func (t *OpenAIToClaudeSSETranslator) TranslateChunk(data []byte) ([]byte, bool, error) {
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		var out bytes.Buffer
		if t.blockStarted {
			out.WriteString(fmt.Sprintf("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n", t.blockIndex))
			t.blockStarted = false
		}
		stopReason := "end_turn"
		if t.finishReason == "length" {
			stopReason = "max_tokens"
		} else if t.finishReason == "tool_calls" || t.finishReason == "function_call" {
			stopReason = "tool_use"
		}
		out.WriteString(fmt.Sprintf("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"%s\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":%d}}\n\n", stopReason, t.outputTokens))
		out.WriteString("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		return out.Bytes(), true, nil
	}

	var chunk map[string]any
	if err := json.Unmarshal(data, &chunk); err != nil {
		return nil, false, nil
	}

	var out bytes.Buffer

	// Check for custom id or usage
	if id, ok := chunk["id"].(string); ok && id != "" && !t.started {
		t.msgID = "msg_" + id
	}

	if usageMap, ok := chunk["usage"].(map[string]any); ok {
		if pt, ok := usageMap["prompt_tokens"].(float64); ok {
			t.inTokens = int(pt)
		}
		if ct, ok := usageMap["completion_tokens"].(float64); ok {
			t.outputTokens = int(ct)
		}
	}

	choices, _ := chunk["choices"].([]any)
	if len(choices) == 0 {
		return nil, false, nil
	}

	choice, ok := choices[0].(map[string]any)
	if !ok {
		return nil, false, nil
	}

	if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
		t.finishReason = fr
	}

	delta, ok := choice["delta"].(map[string]any)
	if !ok {
		return nil, false, nil
	}

	// 1. If this is the very first event, send message_start
	if !t.started {
		t.started = true
		startEvent := map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":      t.msgID,
				"type":    "message",
				"role":    "assistant",
				"content": []any{},
				"model":   t.Model,
				"usage": map[string]any{
					"input_tokens":  t.inTokens,
					"output_tokens": 0,
				},
			},
		}
		startBytes, _ := json.Marshal(startEvent)
		out.WriteString(fmt.Sprintf("event: message_start\ndata: %s\n\n", string(startBytes)))
	}

	// 2. Text delta or reasoning delta
	text, _ := delta["content"].(string)
	reasoning, _ := delta["reasoning_content"].(string)

	if text != "" || reasoning != "" {
		if !t.blockStarted {
			t.blockStarted = true
			startBlock := map[string]any{
				"type":  "content_block_start",
				"index": t.blockIndex,
				"content_block": map[string]any{
					"type": "text",
					"text": "",
				},
			}
			blockBytes, _ := json.Marshal(startBlock)
			out.WriteString(fmt.Sprintf("event: content_block_start\ndata: %s\n\n", string(blockBytes)))
		}

		if text != "" {
			t.outputTokens++
			deltaEvent := map[string]any{
				"type":  "content_block_delta",
				"index": t.blockIndex,
				"delta": map[string]any{
					"type": "text_delta",
					"text": text,
				},
			}
			deltaBytes, _ := json.Marshal(deltaEvent)
			out.WriteString(fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", string(deltaBytes)))
		} else if reasoning != "" {
			deltaEvent := map[string]any{
				"type":  "content_block_delta",
				"index": t.blockIndex,
				"delta": map[string]any{
					"type":     "thinking_delta",
					"thinking": reasoning,
				},
			}
			deltaBytes, _ := json.Marshal(deltaEvent)
			out.WriteString(fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", string(deltaBytes)))
		}
	}

	return out.Bytes(), false, nil
}
