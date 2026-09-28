package rtk

import "strings"

const sep = "\n\n"

// InjectSystemPrompt appends a prompt into the system message of a request body.
// Supports OpenAI (messages[]), Claude (system field), and Gemini (systemInstruction) formats.
func InjectSystemPrompt(body map[string]any, format string, prompt string) {
	if body == nil || prompt == "" {
		return
	}

	switch format {
	case "anthropic", "claude":
		injectClaudeSystem(body, prompt)
	case "gemini":
		injectGeminiSystem(body, prompt)
	default:
		injectMessagesSystem(body, prompt)
	}
}

// InjectCaveman injects the caveman prompt for the given level.
func InjectCaveman(body map[string]any, format string, level string) {
	if p, ok := CavemanPrompts[level]; ok {
		InjectSystemPrompt(body, format, p)
	}
}

// InjectPonytail injects the ponytail prompt for the given level.
func InjectPonytail(body map[string]any, format string, level string) {
	if p, ok := PonytailPrompts[level]; ok {
		InjectSystemPrompt(body, format, p)
	}
}

func injectMessagesSystem(body map[string]any, prompt string) {
	// OpenAI Responses API: top-level string field
	if instructions, ok := body["instructions"].(string); ok {
		if strings.Contains(instructions, prompt) {
			return
		}
		if instructions != "" {
			body["instructions"] = instructions + sep + prompt
		} else {
			body["instructions"] = prompt
		}
		return
	}

	// The array source decides both the write-back key and the part type.
	key, arr, blockType := "messages", []any(nil), "text"
	if msgs, ok := body["messages"].([]any); ok {
		arr = msgs
	} else if input, ok := body["input"].([]any); ok {
		key, arr, blockType = "input", input, "input_text"
	} else {
		return
	}

	// Find existing system/developer message
	for i, m := range arr {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if role == "system" || role == "developer" {
			appendToMessage(msg, prompt, blockType)
			arr[i] = msg
			return
		}
	}

	// No system message found, prepend one
	newMsg := map[string]any{"role": "system", "content": prompt}
	body[key] = append([]any{newMsg}, arr...)
}

func appendToMessage(msg map[string]any, prompt, blockType string) {
	switch content := msg["content"].(type) {
	case string:
		if strings.Contains(content, prompt) {
			return
		}
		if content != "" {
			msg["content"] = content + sep + prompt
		} else {
			msg["content"] = prompt
		}
	case []any:
		// Blocks are appended whole, so an exact match is the right dedup rule.
		for _, b := range content {
			if bm, ok := b.(map[string]any); ok {
				if t, ok := bm["text"].(string); ok && t == prompt {
					return
				}
			}
		}
		msg["content"] = append(content, map[string]any{"type": blockType, "text": prompt})
	default:
		msg["content"] = prompt
	}
}

func injectClaudeSystem(body map[string]any, prompt string) {
	switch sys := body["system"].(type) {
	case string:
		if strings.Contains(sys, prompt) {
			return
		}
		if sys != "" {
			body["system"] = sys + sep + prompt
		} else {
			body["system"] = prompt
		}
	case []any:
		for _, b := range sys {
			if bm, ok := b.(map[string]any); ok {
				if t, ok := bm["text"].(string); ok && t == prompt {
					return
				}
			}
		}
		block := map[string]any{"type": "text", "text": prompt}
		body["system"] = append(sys, block)
	default:
		body["system"] = prompt
	}
}

func injectGeminiSystem(body map[string]any, prompt string) {
	// Check body.request wrapper
	target := body
	if req, ok := body["request"].(map[string]any); ok {
		target = req
	}

	// Determine key: system_instruction or systemInstruction
	key := "systemInstruction"
	if _, ok := target["system_instruction"]; ok {
		key = "system_instruction"
	}

	if sys, ok := target[key].(map[string]any); ok {
		if parts, ok := geminiParts(sys["parts"]); ok {
			for _, p := range parts {
				if pm, ok := p.(map[string]any); ok {
					if t, ok := pm["text"].(string); ok && t == prompt {
						return
					}
				}
			}
			sys["parts"] = append(parts, map[string]any{"text": prompt})
			return
		}
	}
	target[key] = map[string]any{"parts": []any{map[string]any{"text": prompt}}}
}

// geminiParts accepts the typed slices an envelope built in memory carries, not
// only the []any a decoded JSON body carries, so the existing parts survive.
func geminiParts(v any) ([]any, bool) {
	switch parts := v.(type) {
	case []any:
		return parts, true
	case []map[string]any:
		normalized := make([]any, len(parts))
		for i, part := range parts {
			normalized[i] = part
		}
		return normalized, true
	}
	return nil, false
}
