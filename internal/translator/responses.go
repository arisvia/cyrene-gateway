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
		if choice, ok := tc.(map[string]any); ok && choice["type"] == "function" {
			if name, ok := choice["name"].(string); ok {
				result["tool_choice"] = map[string]any{
					"type": "function", "function": map[string]any{"name": name},
				}
			}
		}
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
					iType, _ := itemMap["type"].(string)
					// 1. Tool result item in Responses API (function_call_output)
					if iType == "function_call_output" || (itemMap["call_id"] != nil && itemMap["output"] != nil) {
						callID, _ := itemMap["call_id"].(string)
						outputVal := itemMap["output"]
						outputStr, isStr := outputVal.(string)
						if !isStr {
							b, _ := json.Marshal(outputVal)
							outputStr = string(b)
						}
						openAIMessages = append(openAIMessages, map[string]any{
							"role":         "tool",
							"tool_call_id": callID,
							"content":      outputStr,
						})
						continue
					}
					// 2. Tool call item in Responses API (function_call)
					if iType == "function_call" || (itemMap["name"] != nil && itemMap["call_id"] != nil && itemMap["arguments"] != nil) {
						callID, _ := itemMap["call_id"].(string)
						name, _ := itemMap["name"].(string)
						args, _ := itemMap["arguments"].(string)
						openAIMessages = append(openAIMessages, map[string]any{
							"role":    "assistant",
							"content": nil,
							"tool_calls": []any{
								map[string]any{
									"id":   callID,
									"type": "function",
									"function": map[string]any{
										"name":      name,
										"arguments": args,
									},
								},
							},
						})
						continue
					}

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
	if tools, ok := body["tools"].([]any); ok {
		flatTools := make([]any, 0, len(tools))
		for _, tool := range tools {
			if tm, ok := tool.(map[string]any); ok && tm["type"] == "function" {
				if fn, ok := tm["function"].(map[string]any); ok {
					flat := map[string]any{"type": "function"}
					for _, key := range []string{"name", "description", "parameters", "strict"} {
						if value, ok := fn[key]; ok {
							flat[key] = value
						}
					}
					flatTools = append(flatTools, flat)
					continue
				}
			}
			flatTools = append(flatTools, tool)
		}
		result["tools"] = flatTools
	}
	if tc, ok := body["tool_choice"]; ok {
		result["tool_choice"] = tc
		if choice, ok := tc.(map[string]any); ok && choice["type"] == "function" {
			if fn, ok := choice["function"].(map[string]any); ok {
				result["tool_choice"] = map[string]any{"type": "function", "name": fn["name"]}
			}
		}
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
				} else if role == "tool" {
					callID, _ := mm["tool_call_id"].(string)
					outputStr, isStr := content.(string)
					if !isStr {
						b, _ := json.Marshal(content)
						outputStr = string(b)
					}
					inputItems = append(inputItems, map[string]any{
						"type":    "function_call_output",
						"call_id": callID,
						"output":  outputStr,
					})
				} else if role == "assistant" && mm["tool_calls"] != nil {
					if tcList, ok := mm["tool_calls"].([]any); ok {
						for _, tcRaw := range tcList {
							if tc, ok := tcRaw.(map[string]any); ok {
								callID, _ := tc["id"].(string)
								if fn, ok := tc["function"].(map[string]any); ok {
									name, _ := fn["name"].(string)
									args, _ := fn["arguments"].(string)
									inputItems = append(inputItems, map[string]any{
										"type":      "function_call",
										"call_id":   callID,
										"name":      name,
										"arguments": args,
									})
								}
							}
						}
					}
					if s, ok := content.(string); ok && s != "" {
						inputItems = append(inputItems, map[string]any{
							"type":    "message",
							"role":    "assistant",
							"content": s,
						})
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
	outputItems := []any{}
	status := "completed"
	var incompleteDetails map[string]any

	if choices, ok := openAIResp["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			finishReason, _ := choice["finish_reason"].(string)
			status, incompleteDetails = responsesStatus(finishReason)
			if msg, ok := choice["message"].(map[string]any); ok {
				if text, ok := msg["content"].(string); ok && text != "" {
					outputText = text
					outputItems = append(outputItems, map[string]any{
						"id":     "msg_" + id,
						"type":   "message",
						"status": status,
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
									"id":        "fc_" + tcID,
									"type":      "function_call",
									"status":    status,
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
		"status":     status,
		"model":      respModel,
		"output":     outputItems,
		"usage": map[string]any{
			"total_tokens":  totTokens,
			"input_tokens":  inTokens,
			"output_tokens": outTokens,
		},
	}
	if sourceUsage, ok := openAIResp["usage"].(map[string]any); ok {
		targetUsage := resp["usage"].(map[string]any)
		if details, ok := sourceUsage["prompt_tokens_details"]; ok {
			targetUsage["input_tokens_details"] = details
		}
		if details, ok := sourceUsage["completion_tokens_details"]; ok {
			targetUsage["output_tokens_details"] = details
		}
	}
	if outputText != "" {
		resp["output_text"] = outputText
	}
	if incompleteDetails != nil {
		resp["incomplete_details"] = incompleteDetails
	}
	if upstreamError := openAIResp["error"]; upstreamError != nil {
		resp["status"] = "failed"
		resp["error"] = upstreamError
		delete(resp, "incomplete_details")
		for _, item := range outputItems {
			item.(map[string]any)["status"] = "incomplete"
		}
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
	if upstreamError := responsesError(resp); upstreamError != nil {
		return json.Marshal(map[string]any{"error": upstreamError})
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
					tcID, _ := im["call_id"].(string)
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

	finishReason := responsesFinishReason(resp, len(toolCalls) > 0)

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

	if sourceUsage, ok := resp["usage"].(map[string]any); ok {
		targetUsage := chatResp["usage"].(map[string]any)
		if details, ok := sourceUsage["input_tokens_details"]; ok {
			targetUsage["prompt_tokens_details"] = details
		}
		if details, ok := sourceUsage["output_tokens_details"]; ok {
			targetUsage["completion_tokens_details"] = details
		}
	}
	return json.Marshal(chatResp)
}

// ResponsesSSEToOpenAITranslator manages streaming state to translate
// OpenAI Responses API SSE events into OpenAI ChatCompletion SSE chunks.
type ResponsesSSEToOpenAITranslator struct {
	Model           string
	toolCallIndex   int
	hasToolCalls    bool
	done            bool
	toolIdxByOutput map[int]int // output_index → tool_calls[].index
}

// NewResponsesSSEToOpenAITranslator creates a new stateful streaming translator.
func NewResponsesSSEToOpenAITranslator(model string) *ResponsesSSEToOpenAITranslator {
	return &ResponsesSSEToOpenAITranslator{Model: model}
}

// TranslateChunk converts a single Responses API SSE event into ChatCompletion SSE chunk(s).
func (t *ResponsesSSEToOpenAITranslator) TranslateChunk(data []byte) ([]byte, bool, error) {
	if t.done {
		return nil, true, nil
	}
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("[DONE]")) {
		t.done = true
		return nil, true, nil
	}

	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, false, nil
	}

	eType, _ := event["type"].(string)
	if upstreamError := event["error"]; upstreamError != nil {
		t.done = true
		b, err := json.Marshal(map[string]any{"error": upstreamError})
		return b, true, err
	}
	if eType == "error" {
		delete(event, "type")
		delete(event, "sequence_number")
		t.done = true
		b, err := json.Marshal(map[string]any{"error": event})
		return b, true, err
	}
	switch eType {
	case "response.output_text.delta":
		deltaText, _ := event["delta"].(string)
		chunk := map[string]any{
			"id":      "chatcmpl-stream",
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   t.Model,
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

	case "response.output_item.added":
		// Function call item added — emit initial tool_call chunk with id and name
		if item, ok := event["item"].(map[string]any); ok {
			if iType, _ := item["type"].(string); iType == "function_call" {
				fnName, _ := item["name"].(string)
				callID, _ := item["call_id"].(string)
				idx := t.toolCallIndex
				if outputIdx, ok := event["output_index"].(float64); ok {
					if t.toolIdxByOutput == nil {
						t.toolIdxByOutput = make(map[int]int)
					}
					t.toolIdxByOutput[int(outputIdx)] = idx
				}
				t.toolCallIndex++
				t.hasToolCalls = true
				chunk := map[string]any{
					"id":      "chatcmpl-stream",
					"object":  "chat.completion.chunk",
					"created": time.Now().Unix(),
					"model":   t.Model,
					"choices": []any{
						map[string]any{
							"index": 0,
							"delta": map[string]any{
								"tool_calls": []any{
									map[string]any{
										"index": idx,
										"id":    callID,
										"type":  "function",
										"function": map[string]any{
											"name":      fnName,
											"arguments": "",
										},
									},
								},
							},
							"finish_reason": nil,
						},
					},
				}
				b, _ := json.Marshal(chunk)
				return b, false, nil
			}
		}
		return nil, false, nil

	case "response.function_call_arguments.delta":
		argsDelta, _ := event["delta"].(string)
		outputIdx, _ := event["output_index"].(float64)
		tcIdx, ok := t.toolIdxByOutput[int(outputIdx)]
		if !ok {
			// Fallback: if we never saw output_item.added for this index,
			// assign the next available tool call index
			tcIdx = t.toolCallIndex
			if t.toolIdxByOutput == nil {
				t.toolIdxByOutput = make(map[int]int)
			}
			t.toolIdxByOutput[int(outputIdx)] = tcIdx
			t.toolCallIndex++
			t.hasToolCalls = true
		}
		chunk := map[string]any{
			"id":      "chatcmpl-stream",
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   t.Model,
			"choices": []any{
				map[string]any{
					"index": 0,
					"delta": map[string]any{
						"tool_calls": []any{
							map[string]any{
								"index": tcIdx,
								"function": map[string]any{
									"arguments": argsDelta,
								},
							},
						},
					},
					"finish_reason": nil,
				},
			},
		}
		b, _ := json.Marshal(chunk)
		return b, false, nil

	case "response.completed", "response.failed", "response.incomplete":
		t.done = true
		respObj, _ := event["response"].(map[string]any)
		if respObj == nil {
			respObj = map[string]any{}
		}
		respObj["status"] = strings.TrimPrefix(eType, "response.")
		if upstreamError := responsesError(respObj); upstreamError != nil {
			b, err := json.Marshal(map[string]any{"error": upstreamError})
			return b, true, err
		}
		chunk := map[string]any{
			"id":      "chatcmpl-stream",
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   t.Model,
			"choices": []any{
				map[string]any{
					"index":         0,
					"delta":         map[string]any{},
					"finish_reason": responsesFinishReason(respObj, t.hasToolCalls),
				},
			},
		}
		if usage, ok := respObj["usage"].(map[string]any); ok {
			inTokens, _ := usage["input_tokens"].(float64)
			outTokens, _ := usage["output_tokens"].(float64)
			totTokens, _ := usage["total_tokens"].(float64)
			if totTokens == 0 {
				totTokens = inTokens + outTokens
			}
			convertedUsage := map[string]any{
				"prompt_tokens":     int(inTokens),
				"completion_tokens": int(outTokens),
				"total_tokens":      int(totTokens),
			}
			if details, ok := usage["input_tokens_details"]; ok {
				convertedUsage["prompt_tokens_details"] = details
			}
			if details, ok := usage["output_tokens_details"]; ok {
				convertedUsage["completion_tokens_details"] = details
			}
			chunk["usage"] = convertedUsage
		}
		b, err := json.Marshal(chunk)
		return b, true, err

	default:
		// Ignore structural control events (response.created, response.content_part.added, etc.)
		return nil, false, nil
	}
}

// OpenAIToResponsesSSETranslator manages streaming state to translate
// OpenAI ChatCompletion SSE lines into OpenAI Responses API SSE events.
type OpenAIToResponsesSSETranslator struct {
	Model         string
	respID        string
	msgID         string
	outputBuf     strings.Builder
	promptTokens  int
	outputTokens  int
	totalTokens   int
	inputDetails  map[string]any
	outputDetails map[string]any
	deltaChunks   int // fallback chunk counter when upstream omits usage
	// Output item index allocation: assigned once at first emission, never recomputed.
	nextOutputIndex int
	msgOutputIndex  int // -1 until the message item is emitted
	contentIndex    int
	created         bool
	itemAdded       bool
	partAdded       bool
	done            bool
	finishReason    string
	upstreamError   any
	// Tool call tracking
	toolCalls []responsesToolCallState
}

type responsesToolCallState struct {
	id          string
	name        string
	argsBuf     strings.Builder
	outputIndex int  // assigned once at first emission
	added       bool // whether output_item.added has been emitted
}

// NewOpenAIToResponsesSSETranslator creates a new streaming translator for Responses API.
func NewOpenAIToResponsesSSETranslator(model string) *OpenAIToResponsesSSETranslator {
	now := time.Now().UnixNano()
	return &OpenAIToResponsesSSETranslator{
		Model:          model,
		respID:         fmt.Sprintf("resp_%d", now),
		msgID:          fmt.Sprintf("msg_%d", now),
		msgOutputIndex: -1,
	}
}

// TranslateChunk converts an OpenAI ChatCompletion chunk (or [DONE]) into Responses API SSE events.
func (t *OpenAIToResponsesSSETranslator) TranslateChunk(data []byte) ([]byte, bool, error) {
	if t.done {
		return nil, true, nil
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		status, incompleteDetails := responsesStatus(t.finishReason)
		itemStatus := status
		if t.upstreamError != nil {
			status = "failed"
			itemStatus = "incomplete"
			incompleteDetails = nil
		}
		var out bytes.Buffer
		outText := t.outputBuf.String()

		if t.partAdded {
			out.WriteString(fmt.Sprintf("event: response.content_part.done\ndata: {\"type\":\"response.content_part.done\",\"output_index\":%d,\"content_index\":%d,\"part\":{\"type\":\"output_text\",\"text\":%s}}\n\n",
				t.msgOutputIndex, t.contentIndex, quoteJSON(outText)))
		}

		if t.itemAdded {
			out.WriteString(fmt.Sprintf("event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"output_index\":%d,\"item\":{\"id\":%s,\"type\":\"message\",\"status\":%s,\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":%s}]}}\n\n",
				t.msgOutputIndex, quoteJSON(t.msgID), quoteJSON(itemStatus), quoteJSON(outText)))
		}

		// Emit function_call_arguments.done and output_item.done for each tool call
		for i := range t.toolCalls {
			tcs := &t.toolCalls[i]
			if !tcs.added {
				continue
			}
			argsDonePayload := map[string]any{
				"type":         "response.function_call_arguments.done",
				"item_id":      "fc_" + tcs.id,
				"output_index": tcs.outputIndex,
				"call_id":      tcs.id,
				"arguments":    tcs.argsBuf.String(),
			}
			adBytes, _ := json.Marshal(argsDonePayload)
			out.WriteString(fmt.Sprintf("event: response.function_call_arguments.done\ndata: %s\n\n", string(adBytes)))

			itemDonePayload := map[string]any{
				"type":         "response.output_item.done",
				"output_index": tcs.outputIndex,
				"item": map[string]any{
					"id":        "fc_" + tcs.id,
					"type":      "function_call",
					"status":    itemStatus,
					"call_id":   tcs.id,
					"name":      tcs.name,
					"arguments": tcs.argsBuf.String(),
				},
			}
			idBytes, _ := json.Marshal(itemDonePayload)
			out.WriteString(fmt.Sprintf("event: response.output_item.done\ndata: %s\n\n", string(idBytes)))
		}

		// Build output items ordered by assigned output_index.
		outputItems := make([]any, t.nextOutputIndex)
		if t.itemAdded && t.msgOutputIndex >= 0 && t.msgOutputIndex < len(outputItems) {
			outputItems[t.msgOutputIndex] = map[string]any{
				"id":     t.msgID,
				"type":   "message",
				"status": itemStatus,
				"role":   "assistant",
				"content": []any{
					map[string]any{
						"type": "output_text",
						"text": outText,
					},
				},
			}
		}
		for i := range t.toolCalls {
			tcs := &t.toolCalls[i]
			if tcs.added && tcs.outputIndex >= 0 && tcs.outputIndex < len(outputItems) {
				outputItems[tcs.outputIndex] = map[string]any{
					"id":        "fc_" + tcs.id,
					"type":      "function_call",
					"status":    itemStatus,
					"call_id":   tcs.id,
					"name":      tcs.name,
					"arguments": tcs.argsBuf.String(),
				}
			}
		}
		var compactOutput []any
		for _, it := range outputItems {
			if it != nil {
				compactOutput = append(compactOutput, it)
			}
		}
		if compactOutput == nil {
			compactOutput = []any{}
		}

		// Use real usage when available, fall back to chunk counter estimate
		outTokens := t.outputTokens
		if outTokens == 0 {
			outTokens = t.deltaChunks
		}
		tot := t.totalTokens
		if tot == 0 {
			tot = t.promptTokens + outTokens
		}
		response := map[string]any{
			"id":          t.respID,
			"object":      "response",
			"created_at":  time.Now().Unix(),
			"status":      status,
			"model":       t.Model,
			"output":      compactOutput,
			"output_text": outText,
			"usage": map[string]any{
				"total_tokens":  tot,
				"input_tokens":  t.promptTokens,
				"output_tokens": outTokens,
			},
		}
		if t.inputDetails != nil {
			response["usage"].(map[string]any)["input_tokens_details"] = t.inputDetails
		}
		if t.outputDetails != nil {
			response["usage"].(map[string]any)["output_tokens_details"] = t.outputDetails
		}
		if incompleteDetails != nil {
			response["incomplete_details"] = incompleteDetails
		}
		if t.upstreamError != nil {
			response["error"] = t.upstreamError
		}
		eventType := "response." + status
		donePayload := map[string]any{"type": eventType, "response": response}
		doneBytes, _ := json.Marshal(donePayload)
		out.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(doneBytes)))
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
		if details, ok := usageMap["prompt_tokens_details"].(map[string]any); ok {
			t.inputDetails = details
		}
		if details, ok := usageMap["completion_tokens_details"].(map[string]any); ok {
			t.outputDetails = details
		}
	}

	// First event: response.created
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

	if upstreamError := chunk["error"]; upstreamError != nil {
		t.upstreamError = upstreamError
		terminal, done, err := t.TranslateChunk([]byte("[DONE]"))
		out.Write(terminal)
		return out.Bytes(), done, err
	}

	// Retain finish_reason until [DONE] so trailing usage chunks are included.
	if choices, ok := chunk["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if reason, ok := choice["finish_reason"].(string); ok && reason != "" {
				t.finishReason = reason
			}
			if delta, ok := choice["delta"].(map[string]any); ok {
				if content, ok := delta["content"].(string); ok && content != "" {
					// Ensure message item added
					if !t.itemAdded {
						t.itemAdded = true
						t.msgOutputIndex = t.nextOutputIndex
						t.nextOutputIndex++
						itemAddedPayload := map[string]any{
							"type":         "response.output_item.added",
							"output_index": t.msgOutputIndex,
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

					// Ensure content part added
					if !t.partAdded {
						t.partAdded = true
						partAddedPayload := map[string]any{
							"type":          "response.content_part.added",
							"output_index":  t.msgOutputIndex,
							"content_index": t.contentIndex,
							"part": map[string]any{
								"type": "output_text",
								"text": "",
							},
						}
						paBytes, _ := json.Marshal(partAddedPayload)
						out.WriteString(fmt.Sprintf("event: response.content_part.added\ndata: %s\n\n", string(paBytes)))
					}

					// Emit output_text.delta
					t.outputBuf.WriteString(content)
					t.deltaChunks++
					deltaPayload := map[string]any{
						"type":          "response.output_text.delta",
						"item_id":       t.msgID,
						"output_index":  t.msgOutputIndex,
						"content_index": t.contentIndex,
						"delta":         content,
					}
					dBytes, _ := json.Marshal(deltaPayload)
					out.WriteString(fmt.Sprintf("event: response.output_text.delta\ndata: %s\n\n", string(dBytes)))
				}

				// Handle tool call deltas
				if tcList, ok := delta["tool_calls"].([]any); ok {
					for _, tcRaw := range tcList {
						tc, ok := tcRaw.(map[string]any)
						if !ok {
							continue
						}
						tcIdxF, _ := tc["index"].(float64)
						tcIdx := int(tcIdxF)

						for len(t.toolCalls) <= tcIdx {
							t.toolCalls = append(t.toolCalls, responsesToolCallState{outputIndex: -1})
						}
						tcs := &t.toolCalls[tcIdx]

						if id, ok := tc["id"].(string); ok && id != "" {
							tcs.id = id
						}
						if fn, ok := tc["function"].(map[string]any); ok {
							if name, ok := fn["name"].(string); ok && name != "" {
								tcs.name = name
							}
						}
						if !tcs.added && (tcs.name != "" || tcs.id != "") {
							tcs.added = true
							tcs.outputIndex = t.nextOutputIndex
							t.nextOutputIndex++
							addedPayload := map[string]any{
								"type":         "response.output_item.added",
								"output_index": tcs.outputIndex,
								"item": map[string]any{
									"id":        "fc_" + tcs.id,
									"type":      "function_call",
									"status":    "in_progress",
									"call_id":   tcs.id,
									"name":      tcs.name,
									"arguments": "",
								},
							}
							aBytes, _ := json.Marshal(addedPayload)
							out.WriteString(fmt.Sprintf("event: response.output_item.added\ndata: %s\n\n", string(aBytes)))
						}
						if fn, ok := tc["function"].(map[string]any); ok {
							if args, ok := fn["arguments"].(string); ok && args != "" {
								tcs.argsBuf.WriteString(args)

								if !tcs.added {
									tcs.added = true
									tcs.outputIndex = t.nextOutputIndex
									t.nextOutputIndex++
									addedPayload := map[string]any{
										"type":         "response.output_item.added",
										"output_index": tcs.outputIndex,
										"item": map[string]any{
											"id":        "fc_" + tcs.id,
											"type":      "function_call",
											"status":    "in_progress",
											"call_id":   tcs.id,
											"name":      tcs.name,
											"arguments": "",
										},
									}
									aBytes, _ := json.Marshal(addedPayload)
									out.WriteString(fmt.Sprintf("event: response.output_item.added\ndata: %s\n\n", string(aBytes)))
								}

								argDeltaPayload := map[string]any{
									"type":         "response.function_call_arguments.delta",
									"item_id":      "fc_" + tcs.id,
									"output_index": tcs.outputIndex,
									"call_id":      tcs.id,
									"delta":        args,
								}
								adBytes, _ := json.Marshal(argDeltaPayload)
								out.WriteString(fmt.Sprintf("event: response.function_call_arguments.delta\ndata: %s\n\n", string(adBytes)))
							}
						}
					}
				}
			}
		}
	}

	return out.Bytes(), false, nil
}

func responsesStatus(finishReason string) (string, map[string]any) {
	switch finishReason {
	case "length":
		return "incomplete", map[string]any{"reason": "max_output_tokens"}
	case "content_filter":
		return "incomplete", map[string]any{"reason": "content_filter"}
	default:
		return "completed", nil
	}
}

func responsesFinishReason(resp map[string]any, hasToolCalls bool) string {
	if resp["status"] == "incomplete" {
		if details, ok := resp["incomplete_details"].(map[string]any); ok && details["reason"] == "content_filter" {
			return "content_filter"
		}
		return "length"
	}
	if output, ok := resp["output"].([]any); ok {
		for _, raw := range output {
			if item, ok := raw.(map[string]any); ok && item["type"] == "function_call" {
				hasToolCalls = true
			}
		}
	}
	if hasToolCalls {
		return "tool_calls"
	}
	return "stop"
}

func responsesError(resp map[string]any) any {
	if upstreamError := resp["error"]; upstreamError != nil {
		return upstreamError
	}
	if resp["status"] == "failed" {
		return map[string]any{"type": "upstream_error", "code": "server_error", "message": "Upstream response failed"}
	}
	return nil
}

// quoteJSON serializes string into valid JSON escaped string literal.
func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
