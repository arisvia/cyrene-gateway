package translator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ResponsesToOpenAIRequest converts an OpenAI Responses API (/v1/responses) JSON payload
// into an OpenAI Chat Completions (/v1/chat/completions) JSON payload.
// It supports both input modes:
// Mode 1: simple string input ("input": "Hello world")
// Mode 2: conversation items array ("input": [{"role": "user", "content": "..."}])
func ResponsesToOpenAIRequest(responsesBody map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	// Model
	if m, ok := responsesBody["model"].(string); ok {
		result["model"] = m
	}

	// Stream
	if stream, ok := responsesBody["stream"].(bool); ok {
		result["stream"] = stream
	}

	// Temperature
	if temp, ok := responsesBody["temperature"]; ok {
		result["temperature"] = temp
	}

	// TopP
	if topP, ok := responsesBody["top_p"]; ok {
		result["top_p"] = topP
	}

	// MaxTokens / MaxOutputTokens
	if maxOut, ok := responsesBody["max_output_tokens"]; ok {
		result["max_tokens"] = maxOut
	} else if maxComp, ok := responsesBody["max_completion_tokens"]; ok {
		result["max_tokens"] = maxComp
	} else if maxTokens, ok := responsesBody["max_tokens"]; ok {
		result["max_tokens"] = maxTokens
	}

	// Reasoning effort
	if reasoning, ok := responsesBody["reasoning"].(map[string]any); ok {
		if effort, ok := reasoning["effort"].(string); ok && effort != "" {
			result["reasoning_effort"] = effort
		}
	} else if effort, ok := responsesBody["reasoning_effort"].(string); ok && effort != "" {
		result["reasoning_effort"] = effort
	}

	// Tools conversion
	if toolsRaw, ok := responsesBody["tools"].([]any); ok && len(toolsRaw) > 0 {
		var openAITools []any
		for _, t := range toolsRaw {
			if tm, ok := t.(map[string]any); ok {
				tType, _ := tm["type"].(string)
				if tType == "function" {
					if _, hasFn := tm["function"]; hasFn {
						openAITools = append(openAITools, tm)
					} else {
						// Responses API flat function tool: {"type": "function", "name": "...", "description": "...", "parameters": {...}}
						fnObj := map[string]any{}
						if name, ok := tm["name"]; ok {
							fnObj["name"] = name
						}
						if desc, ok := tm["description"]; ok {
							fnObj["description"] = desc
						}
						if params, ok := tm["parameters"]; ok {
							fnObj["parameters"] = params
						}
						if strict, ok := tm["strict"]; ok {
							fnObj["strict"] = strict
						}
						openAITools = append(openAITools, map[string]any{
							"type":     "function",
							"function": fnObj,
						})
					}
				} else {
					openAITools = append(openAITools, tm)
				}
			}
		}
		if len(openAITools) > 0 {
			result["tools"] = openAITools
		}
	}

	// ToolChoice
	if tc, ok := responsesBody["tool_choice"]; ok {
		result["tool_choice"] = tc
	}

	// Messages assembly
	var openAIMessages []any

	// 1. Instructions (Developer / System message)
	if instructions, ok := responsesBody["instructions"].(string); ok && instructions != "" {
		openAIMessages = append(openAIMessages, map[string]any{
			"role":    "developer",
			"content": instructions,
		})
	}

	// 2. Input
	if inputRaw, ok := responsesBody["input"]; ok {
		switch in := inputRaw.(type) {
		case string:
			// Mode 1: String input
			if in != "" {
				openAIMessages = append(openAIMessages, map[string]any{
					"role":    "user",
					"content": in,
				})
			}
		case []any:
			// Mode 2: Array of conversation items
			for _, item := range in {
				if itemMap, ok := item.(map[string]any); ok {
					role, _ := itemMap["role"].(string)
					if role == "" {
						role = "user"
					}
					// Check content
					if contentStr, ok := itemMap["content"].(string); ok {
						msg := map[string]any{
							"role":    role,
							"content": contentStr,
						}
						if tc, ok := itemMap["tool_calls"]; ok {
							msg["tool_calls"] = tc
						}
						if tcid, ok := itemMap["tool_call_id"]; ok {
							msg["tool_call_id"] = tcid
						}
						openAIMessages = append(openAIMessages, msg)
					} else if contentArr, ok := itemMap["content"].([]any); ok {
						// Complex parts
						var parts []any
						for _, p := range contentArr {
							if pm, ok := p.(map[string]any); ok {
								pType, _ := pm["type"].(string)
								switch pType {
								case "input_text", "text":
									text, _ := pm["text"].(string)
									parts = append(parts, map[string]any{
										"type": "text",
										"text": text,
									})
								case "input_image", "image_url":
									if imgURL, ok := pm["image_url"]; ok {
										parts = append(parts, map[string]any{
											"type":      "image_url",
											"image_url": imgURL,
										})
									}
								default:
									parts = append(parts, pm)
								}
							}
						}
						openAIMessages = append(openAIMessages, map[string]any{
							"role":    role,
							"content": parts,
						})
					} else if callID, ok := itemMap["call_id"].(string); ok && role == "tool" {
						// Function result item
						openAIMessages = append(openAIMessages, map[string]any{
							"role":         "tool",
							"tool_call_id": callID,
							"content":      itemMap["output"],
						})
					}
				}
			}
		}
	} else if msgs, ok := responsesBody["messages"].([]any); ok {
		// If caller directly sent standard messages array
		openAIMessages = append(openAIMessages, msgs...)
	}

	result["messages"] = openAIMessages
	return result, nil
}

// OpenAIToResponsesRequest converts an OpenAI ChatCompletion request into an OpenAI Responses API request.
func OpenAIToResponsesRequest(model string, body map[string]any, stream bool) (map[string]any, error) {
	result := make(map[string]any)
	result["model"] = model
	result["stream"] = stream

	if temp, ok := body["temperature"]; ok {
		result["temperature"] = temp
	}
	if topP, ok := body["top_p"]; ok {
		result["top_p"] = topP
	}
	if maxTokens, ok := body["max_tokens"]; ok {
		result["max_output_tokens"] = maxTokens
	}
	if effort, ok := body["reasoning_effort"].(string); ok && effort != "" {
		result["reasoning"] = map[string]any{"effort": effort}
	}
	if tools, ok := body["tools"]; ok {
		result["tools"] = tools
	}
	if tc, ok := body["tool_choice"]; ok {
		result["tool_choice"] = tc
	}

	var instructions string
	var inputItems []any

	if msgs, ok := body["messages"].([]any); ok {
		for _, m := range msgs {
			if mm, ok := m.(map[string]any); ok {
				role, _ := mm["role"].(string)
				content := mm["content"]

				if role == "system" || role == "developer" {
					if s, ok := content.(string); ok && s != "" {
						if instructions != "" {
							instructions += "\n\n" + s
						} else {
							instructions = s
						}
					}
				} else {
					inputItems = append(inputItems, mm)
				}
			}
		}
	}

	if instructions != "" {
		result["instructions"] = instructions
	}
	result["input"] = inputItems
	return result, nil
}

// OpenAIToResponsesResponse converts an OpenAI ChatCompletion JSON response
// into an OpenAI Responses API response object.
func OpenAIToResponsesResponse(data []byte, model string) ([]byte, error) {
	var openAIResp map[string]any
	if err := json.Unmarshal(data, &openAIResp); err != nil {
		return nil, err
	}

	id, _ := openAIResp["id"].(string)
	if id == "" {
		id = fmt.Sprintf("resp_%d", time.Now().UnixNano())
	}
	if !strings.HasPrefix(id, "resp_") {
		id = "resp_" + id
	}

	respModel, _ := openAIResp["model"].(string)
	if respModel == "" {
		respModel = model
	}

	var outputText string
	var outputItems []any

	if choices, ok := openAIResp["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if msg, ok := choice["message"].(map[string]any); ok {
				if text, ok := msg["content"].(string); ok && text != "" {
					outputText = text
					outputItems = append(outputItems, map[string]any{
						"id":     "msg_" + id,
						"type":   "message",
						"status": "completed",
						"role":   "assistant",
						"content": []any{
							map[string]any{
								"type": "output_text",
								"text": text,
							},
						},
					})
				}

				if tcList, ok := msg["tool_calls"].([]any); ok {
					for _, tcRaw := range tcList {
						if tc, ok := tcRaw.(map[string]any); ok {
							tcID, _ := tc["id"].(string)
							if fn, ok := tc["function"].(map[string]any); ok {
								fnName, _ := fn["name"].(string)
								argsStr, _ := fn["arguments"].(string)
								outputItems = append(outputItems, map[string]any{
									"id":        tcID,
									"type":      "function_call",
									"call_id":   tcID,
									"name":      fnName,
									"arguments": argsStr,
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
	totTokens := 0
	if usage, ok := openAIResp["usage"].(map[string]any); ok {
		if pt, ok := usage["prompt_tokens"].(float64); ok {
			inTokens = int(pt)
		}
		if ct, ok := usage["completion_tokens"].(float64); ok {
			outTokens = int(ct)
		}
		if tt, ok := usage["total_tokens"].(float64); ok {
			totTokens = int(tt)
		}
	}
	if totTokens == 0 {
		totTokens = inTokens + outTokens
	}

	resp := map[string]any{
		"id":         id,
		"object":     "response",
		"created_at": time.Now().Unix(),
		"status":     "completed",
		"model":      respModel,
		"output":     outputItems,
		"usage": map[string]any{
			"total_tokens":  totTokens,
			"input_tokens":  inTokens,
			"output_tokens": outTokens,
		},
	}
	if outputText != "" {
		resp["output_text"] = outputText
	}

	return json.Marshal(resp)
}

// ResponsesToOpenAIResponse converts an OpenAI Responses API JSON response
// back into an OpenAI ChatCompletion JSON response.
func ResponsesToOpenAIResponse(data []byte, model string) ([]byte, error) {
	var resp map[string]any
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	id, _ := resp["id"].(string)
	if id == "" {
		id = fmt.Sprintf("chatcmpl_%d", time.Now().UnixNano())
	}
	id = strings.Replace(id, "resp_", "chatcmpl_", 1)

	respModel, _ := resp["model"].(string)
	if respModel == "" {
		respModel = model
	}

	var contentText string
	var toolCalls []any

	if outputText, ok := resp["output_text"].(string); ok && outputText != "" {
		contentText = outputText
	}

	if outputItems, ok := resp["output"].([]any); ok {
		for _, item := range outputItems {
			if im, ok := item.(map[string]any); ok {
				iType, _ := im["type"].(string)
				if iType == "message" && contentText == "" {
					if cArr, ok := im["content"].([]any); ok {
						for _, cp := range cArr {
							if cpm, ok := cp.(map[string]any); ok {
								if t, ok := cpm["text"].(string); ok && t != "" {
									contentText += t
								}
							}
						}
					}
				} else if iType == "function_call" {
					tcID, _ := im["id"].(string)
					fnName, _ := im["name"].(string)
					args, _ := im["arguments"].(string)
					toolCalls = append(toolCalls, map[string]any{
						"id":   tcID,
						"type": "function",
						"function": map[string]any{
							"name":      fnName,
							"arguments": args,
						},
					})
				}
			}
		}
	}

	message := map[string]any{
		"role":    "assistant",
		"content": contentText,
	}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	inTokens := 0
	outTokens := 0
	totTokens := 0
	if usage, ok := resp["usage"].(map[string]any); ok {
		if pt, ok := usage["input_tokens"].(float64); ok {
			inTokens = int(pt)
		}
		if ct, ok := usage["output_tokens"].(float64); ok {
			outTokens = int(ct)
		}
		if tt, ok := usage["total_tokens"].(float64); ok {
			totTokens = int(tt)
		}
	}
	if totTokens == 0 {
		totTokens = inTokens + outTokens
	}

	chatResp := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   respModel,
		"choices": []any{
			map[string]any{
				"index":         0,
				"message":       message,
				"finish_reason": finishReason,
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     inTokens,
			"completion_tokens": outTokens,
			"total_tokens":      totTokens,
		},
	}

	return json.Marshal(chatResp)
}

// ResponsesSSEToOpenAI converts an OpenAI Responses API SSE data chunk into an OpenAI ChatCompletion SSE chunk.
func ResponsesSSEToOpenAI(data []byte, model string) ([]byte, bool, error) {
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("[DONE]")) {
		return []byte("[DONE]"), true, nil
	}

	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, false, nil
	}

	eType, _ := event["type"].(string)
	switch eType {
	case "response.output_text.delta":
		deltaText, _ := event["delta"].(string)
		chunk := map[string]any{
			"id":      "chatcmpl-stream",
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []any{
				map[string]any{
					"index": 0,
					"delta": map[string]any{
						"content": deltaText,
					},
					"finish_reason": nil,
				},
			},
		}
		b, _ := json.Marshal(chunk)
		return b, false, nil

	case "response.done":
		chunk := map[string]any{
			"id":      "chatcmpl-stream",
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []any{
				map[string]any{
					"index":         0,
					"delta":         map[string]any{},
					"finish_reason": "stop",
				},
			},
		}
		if respObj, ok := event["response"].(map[string]any); ok {
			if usage, ok := respObj["usage"].(map[string]any); ok {
				inTokens, _ := usage["input_tokens"].(float64)
				outTokens, _ := usage["output_tokens"].(float64)
				totTokens, _ := usage["total_tokens"].(float64)
				chunk["usage"] = map[string]any{
					"prompt_tokens":     int(inTokens),
					"completion_tokens": int(outTokens),
					"total_tokens":      int(totTokens),
				}
			}
		}
		b, _ := json.Marshal(chunk)
		return b, true, nil

	default:
		// Ignore structural control events (response.created, response.output_item.added, etc.)
		return nil, false, nil
	}
}

// OpenAIToResponsesSSETranslator manages streaming state to translate
// OpenAI ChatCompletion SSE lines into OpenAI Responses API SSE events.
type OpenAIToResponsesSSETranslator struct {
	Model        string
	respID       string
	msgID        string
	outputBuf    strings.Builder
	promptTokens int
	outputTokens int
	totalTokens  int
	outputIndex  int
	contentIndex int
	created      bool
	itemAdded    bool
	partAdded    bool
	done         bool
}

// NewOpenAIToResponsesSSETranslator creates a new streaming translator for Responses API.
func NewOpenAIToResponsesSSETranslator(model string) *OpenAIToResponsesSSETranslator {
	now := time.Now().UnixNano()
	return &OpenAIToResponsesSSETranslator{
		Model:  model,
		respID: fmt.Sprintf("resp_%d", now),
		msgID:  fmt.Sprintf("msg_%d", now),
	}
}

// TranslateChunk converts an OpenAI ChatCompletion chunk (or [DONE]) into Responses API SSE events.
func (t *OpenAIToResponsesSSETranslator) TranslateChunk(data []byte) ([]byte, bool, error) {
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		if t.done {
			return nil, true, nil
		}
		var out bytes.Buffer
		outText := t.outputBuf.String()

		if t.partAdded {
			out.WriteString(fmt.Sprintf("event: response.content_part.done\ndata: {\"type\":\"response.content_part.done\",\"output_index\":%d,\"content_index\":%d,\"part\":{\"type\":\"output_text\",\"text\":%s}}\n\n",
				t.outputIndex, t.contentIndex, quoteJSON(outText)))
		}

		if t.itemAdded {
			out.WriteString(fmt.Sprintf("event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"output_index\":%d,\"item\":{\"id\":\"%s\",\"type\":\"message\",\"status\":\"completed\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":%s}]}}\n\n",
				t.outputIndex, t.msgID, quoteJSON(outText)))
		}

		// Final response.done event
		tot := t.totalTokens
		if tot == 0 {
			tot = t.promptTokens + t.outputTokens
		}
		donePayload := map[string]any{
			"type": "response.done",
			"response": map[string]any{
				"id":         t.respID,
				"object":     "response",
				"created_at": time.Now().Unix(),
				"status":     "completed",
				"model":      t.Model,
				"output": []any{
					map[string]any{
						"id":     t.msgID,
						"type":   "message",
						"status": "completed",
						"role":   "assistant",
						"content": []any{
							map[string]any{
								"type": "output_text",
								"text": outText,
							},
						},
					},
				},
				"output_text": outText,
				"usage": map[string]any{
					"total_tokens":  tot,
					"input_tokens":  t.promptTokens,
					"output_tokens": t.outputTokens,
				},
			},
		}
		doneBytes, _ := json.Marshal(donePayload)
		out.WriteString(fmt.Sprintf("event: response.done\ndata: %s\n\n", string(doneBytes)))
		t.done = true
		return out.Bytes(), true, nil
	}

	var chunk map[string]any
	if err := json.Unmarshal(data, &chunk); err != nil {
		return nil, false, nil
	}

	var out bytes.Buffer

	// Check for custom id or model
	if id, ok := chunk["id"].(string); ok && id != "" && !t.created {
		t.respID = "resp_" + id
		t.msgID = "msg_" + id
	}
	if m, ok := chunk["model"].(string); ok && m != "" {
		t.Model = m
	}

	if usageMap, ok := chunk["usage"].(map[string]any); ok {
		if pt, ok := usageMap["prompt_tokens"].(float64); ok {
			t.promptTokens = int(pt)
		}
		if ct, ok := usageMap["completion_tokens"].(float64); ok {
			t.outputTokens = int(ct)
		}
		if tt, ok := usageMap["total_tokens"].(float64); ok {
			t.totalTokens = int(tt)
		}
	}

	// 1. First event: response.created
	if !t.created {
		t.created = true
		createdPayload := map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id":         t.respID,
				"object":     "response",
				"created_at": time.Now().Unix(),
				"status":     "in_progress",
				"model":      t.Model,
			},
		}
		cBytes, _ := json.Marshal(createdPayload)
		out.WriteString(fmt.Sprintf("event: response.created\ndata: %s\n\n", string(cBytes)))
	}

	// 2. Process delta content
	if choices, ok := chunk["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if delta, ok := choice["delta"].(map[string]any); ok {
				if content, ok := delta["content"].(string); ok && content != "" {
					// 3. Ensure item added
					if !t.itemAdded {
						t.itemAdded = true
						itemAddedPayload := map[string]any{
							"type":         "response.output_item.added",
							"output_index": t.outputIndex,
							"item": map[string]any{
								"id":      t.msgID,
								"type":    "message",
								"status":  "in_progress",
								"role":    "assistant",
								"content": []any{},
							},
						}
						iaBytes, _ := json.Marshal(itemAddedPayload)
						out.WriteString(fmt.Sprintf("event: response.output_item.added\ndata: %s\n\n", string(iaBytes)))
					}

					// 4. Ensure part added
					if !t.partAdded {
						t.partAdded = true
						partAddedPayload := map[string]any{
							"type":          "response.content_part.added",
							"output_index":  t.outputIndex,
							"content_index": t.contentIndex,
							"part": map[string]any{
								"type": "output_text",
								"text": "",
							},
						}
						paBytes, _ := json.Marshal(partAddedPayload)
						out.WriteString(fmt.Sprintf("event: response.content_part.added\ndata: %s\n\n", string(paBytes)))
					}

					// 5. Emit output_text.delta
					t.outputBuf.WriteString(content)
					t.outputTokens++
					deltaPayload := map[string]any{
						"type":          "response.output_text.delta",
						"item_id":       t.msgID,
						"output_index":  t.outputIndex,
						"content_index": t.contentIndex,
						"delta":         content,
					}
					dBytes, _ := json.Marshal(deltaPayload)
					out.WriteString(fmt.Sprintf("event: response.output_text.delta\ndata: %s\n\n", string(dBytes)))
				}
			}
		}
	}

	return out.Bytes(), false, nil
}

// quoteJSON serializes string into valid JSON escaped string literal.
func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
