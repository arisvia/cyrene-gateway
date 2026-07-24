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
		if instructions != "" {
			body["instructions"] = instructions + sep + prompt
		} else {
			body["instructions"] = prompt
		}
		return
	}

	// messages or input array
	var arr []any
	if msgs, ok := body["messages"].([]any); ok {
		arr = msgs
	} else if input, ok := body["input"].([]any); ok {
		arr = input
	}
	if arr == nil {
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
			appendToMessage(msg, prompt)
			_ = i
			return
		}
	}

	// No system message found, prepend one
	newMsg := map[string]any{"role": "system", "content": prompt}
	if msgs, ok := body["messages"].([]any); ok {
		body["messages"] = append([]any{newMsg}, msgs...)
	} else if input, ok := body["input"].([]any); ok {
		body["input"] = append([]any{newMsg}, input...)
	}
}

func appendToMessage(msg map[string]any, prompt string) {
	switch content := msg["content"].(type) {
	case string:
		msg["content"] = content + sep + prompt
	case []any:
		msg["content"] = append(content, map[string]any{"type": "input_text", "text": prompt})
	default:
		msg["content"] = prompt
	}
}

func injectClaudeSystem(body map[string]any, prompt string) {
	switch sys := body["system"].(type) {
	case string:
		if sys != "" {
			body["system"] = sys + sep + prompt
		} else {
			body["system"] = prompt
		}
	case []any:
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
		if parts, ok := sys["parts"].([]any); ok {
			sys["parts"] = append(parts, map[string]any{"text": prompt})
			return
		}
	}
	target[key] = map[string]any{"parts": []any{map[string]any{"text": prompt}}}
}

// FormatFromAPIType maps provider API type to format string for injection.
func FormatFromAPIType(apiType string) string {
	switch strings.ToLower(apiType) {
	case "anthropic":
		return "anthropic"
	case "gemini":
		return "gemini"
	default:
		return "openai"
	}
}
