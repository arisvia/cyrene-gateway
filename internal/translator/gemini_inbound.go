package translator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// GeminiToOpenAIRequest converts a Google Gemini generateContent JSON payload
// into an OpenAI /v1/chat/completions JSON payload.
func GeminiToOpenAIRequest(geminiBody map[string]any, model string) (map[string]any, error) {
	result := make(map[string]any)
	result["model"] = model

	var messages []any

	// 1. System instruction
	if sys, ok := geminiBody["systemInstruction"].(map[string]any); ok {
		var sysText strings.Builder
		if parts, ok := sys["parts"].([]any); ok {
			for _, p := range parts {
				if pm, ok := p.(map[string]any); ok {
					if t, ok := pm["text"].(string); ok {
						sysText.WriteString(t)
					}
				}
			}
		}
		if sysText.Len() > 0 {
			messages = append(messages, map[string]any{
				"role":    "system",
				"content": sysText.String(),
			})
		}
	} else if sysStr, ok := geminiBody["systemInstruction"].(string); ok && sysStr != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": sysStr,
		})
	}

	// 2. Contents (multi-turn history)
	if contents, ok := geminiBody["contents"].([]any); ok {
		pendingToolIDs := make(map[string][]string)
		for _, c := range contents {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			role, _ := cm["role"].(string)
			if role == "" || role == "user" {
				role = "user"
			} else if role == "model" {
				role = "assistant"
			}

			parts, _ := cm["parts"].([]any)
			explicitResponseIDs := make(map[string]bool)
			for _, p := range parts {
				if part, ok := p.(map[string]any); ok {
					if response, ok := part["functionResponse"].(map[string]any); ok {
						if id, _ := response["id"].(string); id != "" {
							explicitResponseIDs[id] = true
						}
					}
				}
			}
			var textParts []string
			var toolCalls []any
			var hasComplexParts bool
			var contentBlocks []any
			for _, p := range parts {
				pm, ok := p.(map[string]any)
				if !ok {
					continue
				}

				if t, ok := pm["text"].(string); ok {
					textParts = append(textParts, t)
					contentBlocks = append(contentBlocks, map[string]any{
						"type": "text",
						"text": t,
					})
				} else if fc, ok := pm["functionCall"].(map[string]any); ok {
					name, _ := fc["name"].(string)
					argsMap, _ := fc["args"].(map[string]any)
					argsBytes, _ := json.Marshal(argsMap)
					callID, _ := fc["id"].(string)
					if callID == "" {
						callID = "call_" + uuid.New().String()
					}
					if name != "" {
						pendingToolIDs[name] = append(pendingToolIDs[name], callID)
					}
					toolCalls = append(toolCalls, map[string]any{
						"id":   callID,
						"type": "function",
						"function": map[string]any{
							"name":      name,
							"arguments": string(argsBytes),
						},
					})
				} else if fr, ok := pm["functionResponse"].(map[string]any); ok {
					// Tool response turn (OpenAI requires tool_call_id on every role:tool message)
					name, _ := fr["name"].(string)
					callID, _ := fr["id"].(string)
					if callID == "" {
						for _, id := range pendingToolIDs[name] {
							if !explicitResponseIDs[id] {
								callID = id
								break
							}
						}
					}
					if callID != "" {
						for pendingName, ids := range pendingToolIDs {
							for i, id := range ids {
								if id == callID {
									pendingToolIDs[pendingName] = append(ids[:i], ids[i+1:]...)
									break
								}
							}
						}
					} else {
						callID = "call_" + uuid.New().String()
					}
					respData := fr["response"]
					var respStr string
					if s, ok := respData.(string); ok {
						respStr = s
					} else {
						b, _ := json.Marshal(respData)
						respStr = string(b)
					}
					messages = append(messages, map[string]any{
						"role":         "tool",
						"tool_call_id": callID,
						"name":         name,
						"content":      respStr,
					})
				} else if inline, ok := pm["inlineData"].(map[string]any); ok {
					hasComplexParts = true
					mimeType, _ := inline["mimeType"].(string)
					dataStr, _ := inline["data"].(string)
					contentBlocks = append(contentBlocks, map[string]any{
						"type": "image_url",
						"image_url": map[string]any{
							"url": fmt.Sprintf("data:%s;base64,%s", mimeType, dataStr),
						},
					})
				}
			}
			if len(toolCalls) > 0 {
				msg := map[string]any{
					"role":       "assistant",
					"tool_calls": toolCalls,
				}
				if len(textParts) > 0 {
					msg["content"] = strings.Join(textParts, "\n")
				}
				messages = append(messages, msg)
			} else if hasComplexParts {
				messages = append(messages, map[string]any{
					"role":    role,
					"content": contentBlocks,
				})
			} else if len(textParts) > 0 {
				messages = append(messages, map[string]any{
					"role":    role,
					"content": strings.Join(textParts, "\n"),
				})
			}
		}
	}

	result["messages"] = messages

	// 3. Generation config
	if gc, ok := geminiBody["generationConfig"].(map[string]any); ok {
		if temp, ok := gc["temperature"]; ok {
			result["temperature"] = temp
		}
		if topP, ok := gc["topP"]; ok {
			result["top_p"] = topP
		}
		if maxTok, ok := gc["maxOutputTokens"]; ok {
			result["max_tokens"] = maxTok
		}
		if stop, ok := gc["stopSequences"]; ok {
			result["stop"] = stop
		}
		if tc, ok := gc["thinkingConfig"].(map[string]any); ok {
			var budget float64
			var hasBudget bool
			switch b := tc["thinkingBudget"].(type) {
			case float64:
				budget = b
				hasBudget = true
			case int:
				budget = float64(b)
				hasBudget = true
			case int64:
				budget = float64(b)
				hasBudget = true
			}
			if hasBudget {
				if budget <= 0 {
					result["reasoning_effort"] = "none"
				} else if budget <= 2048 {
					result["reasoning_effort"] = "low"
				} else if budget <= 8192 {
					result["reasoning_effort"] = "medium"
				} else {
					result["reasoning_effort"] = "high"
				}
			}
		}
	}

	// 4. Tools (functionDeclarations)
	if toolsRaw, ok := geminiBody["tools"].([]any); ok {
		var openAITools []any
		for _, tItem := range toolsRaw {
			if tm, ok := tItem.(map[string]any); ok {
				if decls, ok := tm["functionDeclarations"].([]any); ok {
					for _, d := range decls {
						if dm, ok := d.(map[string]any); ok {
							fnObj := map[string]any{
								"name": dm["name"],
							}
							if desc, ok := dm["description"]; ok {
								fnObj["description"] = desc
							}
							if params, ok := dm["parameters"]; ok {
								fnObj["parameters"] = params
							}
							openAITools = append(openAITools, map[string]any{
								"type":     "function",
								"function": fnObj,
							})
						}
					}
				}
			}
		}
		if len(openAITools) > 0 {
			result["tools"] = openAITools
		}
	}

	// 5. Tool config
	if tc, ok := geminiBody["toolConfig"].(map[string]any); ok {
		if fcc, ok := tc["functionCallingConfig"].(map[string]any); ok {
			if mode, ok := fcc["mode"].(string); ok {
				switch strings.ToUpper(mode) {
				case "AUTO":
					result["tool_choice"] = "auto"
				case "ANY":
					result["tool_choice"] = "required"
				case "NONE":
					result["tool_choice"] = "none"
				}
			}
		}
	}

	return result, nil
}

// OpenAIToGeminiResponse converts an OpenAI ChatCompletion JSON response
// into a Google Gemini generateContent response object.
func OpenAIToGeminiResponse(data []byte, model string) ([]byte, error) {
	var openAIResp map[string]any
	if err := json.Unmarshal(data, &openAIResp); err != nil {
		return nil, err
	}

	var parts []any
	var finishReason = "STOP"

	if choices, ok := openAIResp["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if msg, ok := choice["message"].(map[string]any); ok {
				if text, ok := msg["content"].(string); ok && text != "" {
					parts = append(parts, map[string]any{"text": text})
				}
				if toolCalls, ok := msg["tool_calls"].([]any); ok {
					for _, tcRaw := range toolCalls {
						if tc, ok := tcRaw.(map[string]any); ok {
							if fn, ok := tc["function"].(map[string]any); ok {
								name, _ := fn["name"].(string)
								argsStr, _ := fn["arguments"].(string)
								var argsMap map[string]any
								if err := json.Unmarshal([]byte(argsStr), &argsMap); err != nil {
									argsMap = map[string]any{"input": argsStr}
								}
								fc := map[string]any{"name": name, "args": argsMap}
								if id, ok := tc["id"].(string); ok && id != "" {
									fc["id"] = id
								}
								parts = append(parts, map[string]any{"functionCall": fc})
							}
						}
					}
				}
			}
			if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
				switch fr {
				case "length":
					finishReason = "MAX_TOKENS"
				case "content_filter":
					finishReason = "SAFETY"
				default:
					finishReason = "STOP"
				}
			}
		}
	}

	if parts == nil {
		parts = []any{map[string]any{"text": ""}}
	}

	candidate := map[string]any{
		"content": map[string]any{
			"role":  "model",
			"parts": parts,
		},
		"finishReason": finishReason,
		"index":        0,
	}

	geminiResp := map[string]any{
		"candidates":   []any{candidate},
		"modelVersion": model,
	}

	if usage, ok := openAIResp["usage"].(map[string]any); ok {
		pt, _ := usage["prompt_tokens"].(float64)
		ct, _ := usage["completion_tokens"].(float64)
		tt, _ := usage["total_tokens"].(float64)
		geminiResp["usageMetadata"] = map[string]any{
			"promptTokenCount":     int(pt),
			"candidatesTokenCount": int(ct),
			"totalTokenCount":      int(tt),
		}
	}

	return json.Marshal(geminiResp)
}

type geminiStreamingToolCall struct {
	id   string
	name string
	args strings.Builder
}

// OpenAIToGeminiSSETranslator translates OpenAI ChatCompletion SSE chunks
// into Google Gemini streamGenerateContent SSE chunks.
type OpenAIToGeminiSSETranslator struct {
	toolCalls    map[int]*geminiStreamingToolCall
	Model        string
	toolOrder    []int
	done         bool
	failed       bool
	toolsEmitted bool
}

// NewOpenAIToGeminiSSETranslator creates a new streaming translator for Gemini.
func NewOpenAIToGeminiSSETranslator(model string) *OpenAIToGeminiSSETranslator {
	return &OpenAIToGeminiSSETranslator{
		Model:     model,
		toolCalls: make(map[int]*geminiStreamingToolCall),
	}
}

// TranslateChunk converts a single OpenAI SSE chunk into a Gemini SSE chunk.
func (t *OpenAIToGeminiSSETranslator) TranslateChunk(data []byte) ([]byte, bool, error) {
	if t.failed {
		return nil, true, nil
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		if t.done {
			return nil, true, nil
		}
		t.done = true
		var candidates []any
		if len(t.toolCalls) > 0 && !t.toolsEmitted {
			t.toolsEmitted = true
			var parts []any
			for _, idx := range t.toolOrder {
				tc := t.toolCalls[idx]
				var argsMap map[string]any
				argsStr := tc.args.String()
				if err := json.Unmarshal([]byte(argsStr), &argsMap); err != nil {
					argsMap = map[string]any{"input": argsStr}
				}
				parts = append(parts, map[string]any{
					"functionCall": map[string]any{
						"id":   tc.id,
						"name": tc.name,
						"args": argsMap,
					},
				})
			}
			candidates = append(candidates, map[string]any{
				"content": map[string]any{
					"role":  "model",
					"parts": parts,
				},
				"finishReason": "STOP",
				"index":        0,
			})
		} else {
			candidates = append(candidates, map[string]any{
				"finishReason": "STOP",
				"index":        0,
			})
		}
		donePayload := map[string]any{
			"candidates":   candidates,
			"modelVersion": t.Model,
		}
		b, _ := json.Marshal(donePayload)
		return []byte(fmt.Sprintf("data: %s\n\n", string(b))), true, nil
	}

	var chunk map[string]any
	if err := json.Unmarshal(data, &chunk); err != nil {
		return nil, false, nil
	}

	if upstreamError := chunk["error"]; upstreamError != nil {
		t.failed, t.done = true, true
		message := "Upstream stream failed"
		if detail, ok := upstreamError.(map[string]any); ok {
			if text, ok := detail["message"].(string); ok {
				message = text
			}
		} else if text, ok := upstreamError.(string); ok {
			message = text
		}
		payload, err := json.Marshal(map[string]any{"error": map[string]any{"code": 502, "status": "UNAVAILABLE", "message": message}})
		return []byte(fmt.Sprintf("data: %s\n\n", payload)), true, err
	}
	choices, _ := chunk["choices"].([]any)
	var usageMetadata map[string]any
	if usage, ok := chunk["usage"].(map[string]any); ok {
		pt, _ := usage["prompt_tokens"].(float64)
		ct, _ := usage["completion_tokens"].(float64)
		tt, _ := usage["total_tokens"].(float64)
		usageMetadata = map[string]any{
			"promptTokenCount":     int(pt),
			"candidatesTokenCount": int(ct),
			"totalTokenCount":      int(tt),
		}
	}

	if len(choices) == 0 {
		if usageMetadata != nil {
			geminiChunk := map[string]any{
				"candidates":    []any{},
				"usageMetadata": usageMetadata,
				"modelVersion":  t.Model,
			}
			b, _ := json.Marshal(geminiChunk)
			return []byte(fmt.Sprintf("data: %s\n\n", string(b))), false, nil
		}
		return nil, false, nil
	}

	choice, ok := choices[0].(map[string]any)
	if !ok {
		return nil, false, nil
	}

	delta, ok := choice["delta"].(map[string]any)
	if !ok {
		return nil, false, nil
	}

	// Accumulate streaming tool call deltas
	if tcList, ok := delta["tool_calls"].([]any); ok {
		for _, tcRaw := range tcList {
			if tc, ok := tcRaw.(map[string]any); ok {
				idx := 0
				if rawIdx, ok := tc["index"]; ok {
					if f, ok := rawIdx.(float64); ok {
						idx = int(f)
					}
				}
				st, exists := t.toolCalls[idx]
				if !exists {
					st = &geminiStreamingToolCall{}
					t.toolCalls[idx] = st
					t.toolOrder = append(t.toolOrder, idx)
				}
				if callID, ok := tc["id"].(string); ok && callID != "" {
					st.id = callID
				}
				if fn, ok := tc["function"].(map[string]any); ok {
					if name, ok := fn["name"].(string); ok && name != "" {
						st.name = name
					}
					if args, ok := fn["arguments"].(string); ok && args != "" {
						st.args.WriteString(args)
					}
				}
			}
		}
	}

	var parts []any
	if text, ok := delta["content"].(string); ok && text != "" {
		parts = append(parts, map[string]any{"text": text})
	}

	fr, _ := choice["finish_reason"].(string)
	geminiReason := "STOP"
	switch fr {
	case "length":
		geminiReason = "MAX_TOKENS"
	case "content_filter":
		geminiReason = "SAFETY"
	}
	if fr != "" {
		t.done = true
	}

	// If tool calls were accumulated and finishReason arrived, flush the functionCalls
	if fr != "" && len(t.toolCalls) > 0 && !t.toolsEmitted {
		t.toolsEmitted = true
		t.done = true
		for _, idx := range t.toolOrder {
			tc := t.toolCalls[idx]
			var argsMap map[string]any
			argsStr := tc.args.String()
			if err := json.Unmarshal([]byte(argsStr), &argsMap); err != nil {
				argsMap = map[string]any{"input": argsStr}
			}
			parts = append(parts, map[string]any{
				"functionCall": map[string]any{
					"id":   tc.id,
					"name": tc.name,
					"args": argsMap,
				},
			})
		}
	}

	if len(parts) == 0 {
		if fr != "" {
			donePayload := map[string]any{
				"candidates": []any{
					map[string]any{
						"finishReason": geminiReason,
						"index":        0,
					},
				},
				"modelVersion": t.Model,
			}
			if usageMetadata != nil {
				donePayload["usageMetadata"] = usageMetadata
			}
			b, _ := json.Marshal(donePayload)
			return []byte(fmt.Sprintf("data: %s\n\n", string(b))), true, nil
		}
		return nil, false, nil
	}

	candidate := map[string]any{
		"content": map[string]any{
			"role":  "model",
			"parts": parts,
		},
		"index": 0,
	}
	if fr != "" {
		candidate["finishReason"] = geminiReason
	}

	geminiChunk := map[string]any{
		"candidates":   []any{candidate},
		"modelVersion": t.Model,
	}
	if usageMetadata != nil {
		geminiChunk["usageMetadata"] = usageMetadata
	}

	b, _ := json.Marshal(geminiChunk)
	return []byte(fmt.Sprintf("data: %s\n\n", string(b))), fr != "", nil
}
