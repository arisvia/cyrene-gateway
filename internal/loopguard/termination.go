package loopguard

import "strings"

// TerminationPrompt is the minimal termination contract injected into system messages.
// Based on VansRouter/Moonshot research: reward stopping + anti-repetition.
const TerminationPrompt = `When you have gathered sufficient information to answer the request, STOP calling tools and provide your final answer. Do not call a tool with the same arguments more than once. If a previous attempt returned the same result, change strategy or summarize with available data. Plan briefly (1-3 steps max), then ACT immediately. Do NOT restate your plan — if you have decided what to do, do it now. If you catch yourself repeating the same intention, STOP and give your answer with current knowledge.`

const sep = "\n\n"

// InjectTerminationPrompt injects the termination contract into the request body's system message.
// Supports OpenAI (messages array), Anthropic (system field), and Gemini (systemInstruction) formats.
func InjectTerminationPrompt(body map[string]any, format string) {
	if body == nil {
		return
	}

	switch format {
	case "anthropic":
		injectClaudeSystem(body, TerminationPrompt)
	case "gemini":
		injectGeminiSystem(body, TerminationPrompt)
	default:
		injectMessagesSystem(body, TerminationPrompt)
	}
}

// InjectLoopHint injects a loop detection hint into the request body as an additional system message.
func InjectLoopHint(body map[string]any, format string, hint string) {
	if body == nil || hint == "" {
		return
	}

	switch format {
	case "anthropic":
		injectClaudeSystem(body, hint)
	case "gemini":
		injectGeminiSystem(body, hint)
	default:
		injectMessagesSystem(body, hint)
	}
}

func injectMessagesSystem(body map[string]any, prompt string) {
	// Check for "instructions" field (Responses API)
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

	// Find messages array
	messages, ok := body["messages"].([]any)
	if !ok {
		// Try "input" field
		if input, ok := body["input"].([]any); ok {
			messages = input
		} else {
			return
		}
	}

	// Find existing system/developer message
	for i, msgRaw := range messages {
		msg, ok := msgRaw.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if role == "system" || role == "developer" {
			appendToMessage(msg, prompt)
			messages[i] = msg
			return
		}
	}

	// No system message found, prepend one
	newMsg := map[string]any{"role": "system", "content": prompt}
	body["messages"] = append([]any{newMsg}, messages...)
}

func appendToMessage(msg map[string]any, prompt string) {
	content := msg["content"]
	switch c := content.(type) {
	case string:
		if strings.Contains(c, prompt) {
			return
		}
		msg["content"] = c + sep + prompt
	case []any:
		// Check if already injected
		for _, part := range c {
			if p, ok := part.(map[string]any); ok {
				if p["text"] == prompt {
					return
				}
			}
		}
		msg["content"] = append(c, map[string]any{"type": "text", "text": prompt})
	default:
		msg["content"] = prompt
	}
}

func injectClaudeSystem(body map[string]any, prompt string) {
	system := body["system"]
	switch s := system.(type) {
	case string:
		if strings.Contains(s, prompt) {
			return
		}
		if s != "" {
			body["system"] = s + sep + prompt
		} else {
			body["system"] = prompt
		}
	case []any:
		for _, part := range s {
			if p, ok := part.(map[string]any); ok {
				if p["text"] == prompt {
					return
				}
			}
		}
		body["system"] = append(s, map[string]any{"type": "text", "text": prompt})
	default:
		body["system"] = prompt
	}
}

func injectGeminiSystem(body map[string]any, prompt string) {
	// Check for systemInstruction
	sys, ok := body["systemInstruction"].(map[string]any)
	if !ok {
		body["systemInstruction"] = map[string]any{
			"parts": []any{map[string]any{"text": prompt}},
		}
		return
	}

	parts, ok := sys["parts"].([]any)
	if !ok {
		sys["parts"] = []any{map[string]any{"text": prompt}}
		return
	}

	// Check idempotency
	for _, p := range parts {
		if pm, ok := p.(map[string]any); ok {
			if pm["text"] == prompt {
				return
			}
		}
	}
	sys["parts"] = append(parts, map[string]any{"text": prompt})
}
