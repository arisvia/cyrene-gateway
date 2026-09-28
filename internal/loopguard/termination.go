package loopguard

import "strings"

// TerminationPrompt is the minimal termination contract injected into system messages.
// Based on VansRouter/Moonshot research: reward stopping + anti-repetition.
const TerminationPrompt = `When you have gathered sufficient information to answer the request, STOP calling tools and provide your final answer. Do not call a tool with the same arguments more than once. If a previous attempt returned the same result, change strategy or summarize with available data. Plan briefly (1-3 steps max), then ACT immediately. Do NOT restate your plan — if you have decided what to do, do it now. If you catch yourself repeating the same intention, STOP and give your answer with current knowledge.`

const sep = "\n\n"

// InjectTerminationPrompt injects the termination contract into the OpenAI-shaped
// request body's system message. Only this shape is proxied through the executor.
func InjectTerminationPrompt(body map[string]any) {
	if body == nil {
		return
	}
	injectMessagesSystem(body, TerminationPrompt)
}

// InjectLoopHint injects a loop detection hint as an additional system message.
func InjectLoopHint(body map[string]any, hint string) {
	if body == nil || hint == "" {
		return
	}
	injectMessagesSystem(body, hint)
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

	// The array source decides both the write-back key and the part type.
	key, arr, blockType := "messages", []any(nil), "text"
	if messages, ok := body["messages"].([]any); ok {
		arr = messages
	} else if input, ok := body["input"].([]any); ok {
		key, arr, blockType = "input", input, "input_text"
	} else {
		return
	}

	// Find existing system/developer message
	for i, msgRaw := range arr {
		msg, ok := msgRaw.(map[string]any)
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
	content := msg["content"]
	switch c := content.(type) {
	case string:
		if strings.Contains(c, prompt) {
			return
		}
		if c != "" {
			msg["content"] = c + sep + prompt
		} else {
			msg["content"] = prompt
		}
	case []any:
		// Blocks are appended whole, so an exact match is the right dedup rule.
		for _, part := range c {
			if p, ok := part.(map[string]any); ok {
				if p["text"] == prompt {
					return
				}
			}
		}
		msg["content"] = append(c, map[string]any{"type": blockType, "text": prompt})
	default:
		msg["content"] = prompt
	}
}
