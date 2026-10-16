package rtk

import (
	"encoding/json"
	"unicode"
)

// EstimateTextTokens estimates the token count of a plain text string using
// multi-lingual BPE approximation (ASCII words/code vs CJK/multibyte runes).
func EstimateTextTokens(s string) int {
	if s == "" {
		return 0
	}

	asciiChars := 0
	cjkChars := 0
	otherChars := 0

	for _, r := range s {
		switch {
		case r <= 127:
			asciiChars++
		case unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r):
			cjkChars++
		default:
			otherChars++
		}
	}

	tokens := 0
	if asciiChars > 0 {
		tokens += (asciiChars + 3) / 4
	}
	if cjkChars > 0 {
		tokens += (cjkChars*7 + 9) / 10 // ~0.7 tokens per CJK char
	}
	if otherChars > 0 {
		tokens += (otherChars + 1) / 2
	}

	if tokens < 1 && len(s) > 0 {
		return 1
	}
	return tokens
}

// EstimateAnthropicTokens estimates the input token count for an Anthropic Messages API payload.
// It accounts for system prompt, message structure framing, text blocks, tool definitions, and tool results.
func EstimateAnthropicTokens(body map[string]any) int {
	if body == nil {
		return 0
	}

	total := 3 // base conversation framing tokens

	// 1. System prompt
	if sys, ok := body["system"].(string); ok && sys != "" {
		total += EstimateTextTokens(sys) + 4
	} else if sysArr, ok := body["system"].([]any); ok {
		for _, block := range sysArr {
			if bm, ok := block.(map[string]any); ok {
				if t, ok := bm["text"].(string); ok {
					total += EstimateTextTokens(t) + 4
				}
			}
		}
	}

	// 2. Messages
	if msgs, ok := body["messages"].([]any); ok {
		for _, m := range msgs {
			msg, ok := m.(map[string]any)
			if !ok {
				continue
			}
			total += 4 // role framing overhead per message

			if contentStr, ok := msg["content"].(string); ok {
				total += EstimateTextTokens(contentStr)
			} else if contentArr, ok := msg["content"].([]any); ok {
				for _, block := range contentArr {
					b, ok := block.(map[string]any)
					if !ok {
						continue
					}
					total += 3 // block framing

					bType, _ := b["type"].(string)
					switch bType {
					case "text":
						if t, ok := b["text"].(string); ok {
							total += EstimateTextTokens(t)
						}
					case "tool_use":
						total += 8 // tool_use envelope
						if name, ok := b["name"].(string); ok {
							total += EstimateTextTokens(name)
						}
						if input := b["input"]; input != nil {
							if inputBytes, err := json.Marshal(input); err == nil {
								total += EstimateTextTokens(string(inputBytes))
							}
						}
					case "tool_result":
						total += 6 // tool_result envelope
						if cStr, ok := b["content"].(string); ok {
							total += EstimateTextTokens(cStr)
						} else if cArr, ok := b["content"].([]any); ok {
							for _, inner := range cArr {
								if im, ok := inner.(map[string]any); ok {
									if t, ok := im["text"].(string); ok {
										total += EstimateTextTokens(t)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// 3. Tools definitions
	if tools, ok := body["tools"].([]any); ok {
		total += 8 // tools section preamble
		for _, tool := range tools {
			tm, ok := tool.(map[string]any)
			if !ok {
				continue
			}
			total += 12 // per-tool overhead
			if name, ok := tm["name"].(string); ok {
				total += EstimateTextTokens(name)
			}
			if desc, ok := tm["description"].(string); ok {
				total += EstimateTextTokens(desc)
			}
			if schema := tm["input_schema"]; schema != nil {
				if schemaBytes, err := json.Marshal(schema); err == nil {
					total += EstimateTextTokens(string(schemaBytes))
				}
			}
		}
	}

	return total
}
