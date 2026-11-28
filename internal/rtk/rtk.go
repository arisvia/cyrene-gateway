// Package rtk implements token-saving optimizations: RTK compression of tool
// results, caveman terse-style prompts, and ponytail minimal-code prompts.
package rtk

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	rawCap          = 10 * 1024 * 1024 // 10 MiB
	minCompressSize = 500              // bytes; skip tiny blobs
)

const (
	// RTKLossless performs lossless pre-cleaning only (ANSI strip, blank line collapse, JSON compaction) without truncation.
	RTKLossless = "lossless"
	// RTKBalanced raises truncation thresholds so long coding sessions retain more active and historical context.
	RTKBalanced = "balanced"
	// RTKAggressive applies compact turn-aware truncation thresholds (default behavior).
	RTKAggressive = "aggressive"
)

// IsValidRTKLevel reports whether level is empty (default) or a recognized RTK mode.
func IsValidRTKLevel(level string) bool {
	switch level {
	case "", RTKLossless, RTKBalanced, RTKAggressive:
		return true
	default:
		return false
	}
}

// CompressMessages compresses tool_result content in-place within an OpenAI,
// Anthropic, or Gemini shaped request body. Level controls truncation behavior:
// RTKLossless cleans without truncating; RTKBalanced uses relaxed limits;
// RTKAggressive (default) keeps the existing turn-aware slimming limits.
// Failing outputs in the latest active turn are never touched. Returns bytes saved or 0.
func CompressMessages(body map[string]any, enabled bool, level ...string) int {
	if !enabled || body == nil {
		return 0
	}

	mode := RTKAggressive
	if len(level) > 0 && level[0] != "" {
		mode = level[0]
	}

	saved := 0

	// OpenAI / Anthropic format: body["messages"]
	if msgs, ok := body["messages"].([]any); ok {
		lastAssistantIdx := -1
		for i, m := range msgs {
			if msg, ok := m.(map[string]any); ok {
				if role, _ := msg["role"].(string); role == "assistant" {
					lastAssistantIdx = i
				}
			}
		}

		for i, m := range msgs {
			msg, ok := m.(map[string]any)
			if !ok {
				continue
			}
			isHistorical := (lastAssistantIdx != -1 && i < lastAssistantIdx)

			// OpenAI tool message: {role:"tool", content:"string"} or content parts
			role, _ := msg["role"].(string)
			if role == "tool" {
				// Preserve active errors in latest turn (e.g. error/exit code output)
				if !isHistorical && isToolOutputError(msg["content"]) {
					continue
				}
				if content, ok := msg["content"].(string); ok {
					compressed := compressText(content, isHistorical, mode)
					if len(compressed) < len(content) {
						saved += len(content) - len(compressed)
						msg["content"] = compressed
					}
				} else if parts, ok := msg["content"].([]any); ok {
					for _, part := range parts {
						if p, ok := part.(map[string]any); ok {
							if text, ok := p["text"].(string); ok {
								compressed := compressText(text, isHistorical, mode)
								if len(compressed) < len(text) {
									saved += len(text) - len(compressed)
									p["text"] = compressed
								}
							}
						}
					}
				}
				continue
			}

			// Content blocks array with tool_result entries (Anthropic format)
			contentArr, ok := msg["content"].([]any)
			if !ok {
				continue
			}
			for _, block := range contentArr {
				b, ok := block.(map[string]any)
				if !ok {
					continue
				}
				blockType, _ := b["type"].(string)
				if blockType != "tool_result" {
					continue
				}
				// Preserve active errors in the latest turn so agent can debug;
				// historical errors can be safely compressed.
				if isError, _ := b["is_error"].(bool); isError && !isHistorical {
					continue
				}
				if text, ok := b["content"].(string); ok {
					compressed := compressText(text, isHistorical, mode)
					if len(compressed) < len(text) {
						saved += len(text) - len(compressed)
						b["content"] = compressed
					}
				} else if innerArr, ok := b["content"].([]any); ok {
					for _, inner := range innerArr {
						if ib, ok := inner.(map[string]any); ok {
							if text, ok := ib["text"].(string); ok {
								compressed := compressText(text, isHistorical, mode)
								if len(compressed) < len(text) {
									saved += len(text) - len(compressed)
									ib["text"] = compressed
								}
							}
						}
					}
				}
			}
		}
	}

	// Gemini format: body["contents"] -> parts -> functionResponse -> response -> content
	// Accept the typed slices the antigravity envelope carries in memory, not
	// just JSON-decoded []any (geminiParts parity).
	if contents, ok := geminiParts(body["contents"]); ok {
		lastModelIdx := -1
		for i, c := range contents {
			if contentMap, ok := c.(map[string]any); ok {
				if role, _ := contentMap["role"].(string); role == "model" {
					lastModelIdx = i
				}
			}
		}

		for i, c := range contents {
			contentMap, ok := c.(map[string]any)
			if !ok {
				continue
			}
			isHistorical := (lastModelIdx != -1 && i < lastModelIdx)

			parts, ok := geminiParts(contentMap["parts"])
			if !ok {
				continue
			}
			for _, p := range parts {
				partMap, ok := p.(map[string]any)
				if !ok {
					continue
				}
				fnResp, ok := partMap["functionResponse"].(map[string]any)
				if !ok {
					continue
				}
				resp, ok := fnResp["response"].(map[string]any)
				if !ok {
					continue
				}
				// Preserve active errors in latest turn for Gemini function response
				if !isHistorical && (isToolOutputError(resp["error"]) || isToolOutputError(resp["content"]) || isToolOutputError(resp["result"])) {
					continue
				}
				if content, ok := resp["content"].(string); ok {
					compressed := compressText(content, isHistorical, mode)
					if len(compressed) < len(content) {
						saved += len(content) - len(compressed)
						resp["content"] = compressed
					}
				} else if result, ok := resp["result"].(string); ok {
					compressed := compressText(result, isHistorical, mode)
					if len(compressed) < len(result) {
						saved += len(result) - len(compressed)
						resp["result"] = compressed
					}
				}
			}
		}
	}

	// OpenAI Responses format: body["input"] -> items with type "function_call_output"
	if input, ok := body["input"].([]any); ok {
		lastAssistantIdx := -1
		for i, raw := range input {
			if item, ok := raw.(map[string]any); ok {
				role, _ := item["role"].(string)
				itemType, _ := item["type"].(string)
				if role == "assistant" || itemType == "function_call" || itemType == "reasoning" {
					lastAssistantIdx = i
				}
			}
		}

		for i, raw := range input {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if itemType, _ := item["type"].(string); itemType != "function_call_output" {
				continue
			}
			isHistorical := (lastAssistantIdx != -1 && i < lastAssistantIdx)
			if !isHistorical && isToolOutputError(item["output"]) {
				continue
			}
			if output, ok := item["output"].(string); ok {
				compressed := compressText(output, isHistorical, mode)
				if len(compressed) < len(output) {
					saved += len(output) - len(compressed)
					item["output"] = compressed
				}
			} else if parts, ok := item["output"].([]any); ok {
				for _, part := range parts {
					if p, ok := part.(map[string]any); ok {
						if text, ok := p["text"].(string); ok {
							compressed := compressText(text, isHistorical, mode)
							if len(compressed) < len(text) {
								saved += len(text) - len(compressed)
								p["text"] = compressed
							}
						}
					}
				}
			}
		}
	}

	return saved
}

var (
	ansiRegex    = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)
	multiNLRegex = regexp.MustCompile(`\n{3,}`)
)

// isToolOutputError detects error markers in tool outputs to prevent collapsing active failures.
func isToolOutputError(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	switch val := v.(type) {
	case string:
		return hasErrorKeywords(val)
	case map[string]any:
		if errVal, ok := val["error"]; ok && errVal != nil {
			return true
		}
	case []any:
		for _, item := range val {
			if m, ok := item.(map[string]any); ok {
				if t, ok := m["text"].(string); ok && hasErrorKeywords(t) {
					return true
				}
			}
		}
	}
	return false
}

func hasErrorKeywords(str string) bool {
	if len(str) == 0 {
		return false
	}
	lower := strings.ToLower(str)
	if strings.HasPrefix(lower, "error:") {
		return true
	}
	// Bias toward preserving: a false positive only skips compression of one
	// active-turn output, while a miss drops the middle of a live failure that
	// the agent still needs (panic traces, nonzero exits, toolchain fatals).
	for _, kw := range []string{
		"traceback (most recent call last)",
		"exception:", "command failed", "command not found",
		"panic:", "fatal:", "exit status ", "exit code ",
		"permission denied", "segmentation fault", "core dumped",
		"failed:", "failed ", "test failed", "with code ",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// cleanAndCompactText performs lossless pre-compression on tool result text:
// 1. Strips terminal ANSI color and cursor escape sequences.
// 2. Collapses redundant consecutive blank lines (\n{3,} -> \n\n).
// 3. Compacts formatted JSON objects or arrays into dense minified JSON.
func cleanAndCompactText(text string) string {
	// 1. Strip ANSI escape sequences if present
	if strings.Contains(text, "\x1b[") {
		text = ansiRegex.ReplaceAllString(text, "")
	}

	// 2. Collapse 3+ newlines to 2 newlines (\n\n)
	if strings.Contains(text, "\n\n\n") {
		text = multiNLRegex.ReplaceAllString(text, "\n\n")
	}

	// 3. Lossless JSON compaction
	trimmed := strings.TrimSpace(text)
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		var buf bytes.Buffer
		if err := json.Compact(&buf, []byte(trimmed)); err == nil {
			compacted := buf.String()
			if len(compacted) < len(text) {
				text = compacted
			}
		}
	}

	return text
}

// compressText applies lossless cleaning/compacting followed by level-aware truncation.
func compressText(text string, isHistorical bool, level string) string {
	n := len(text)
	if n == 0 || n > rawCap {
		return text
	}

	if level == RTKLossless {
		return cleanAndCompactText(text)
	}

	if n < minCompressSize {
		return text
	}

	// Phase 1: Lossless pre-compression (ANSI strip, newline collapse, JSON compact)
	cleaned := cleanAndCompactText(text)
	if len(cleaned) < n {
		text = cleaned
		n = len(text)
	}

	// Thresholds depend on whether this tool output belongs to an active latest turn
	// or an expired historical turn, scaled by the configured RTK level.
	safeLineThreshold := 250
	maxCharThreshold := 16 * 1024 // 16 KiB (~4k tokens)
	headLines := 120
	tailLines := 60
	headChars := 8 * 1024
	tailChars := 4 * 1024
	omittedMsg := "lines omitted"
	omittedCharMsg := "characters omitted"

	if level == RTKBalanced {
		safeLineThreshold = 500
		maxCharThreshold = 32 * 1024
		headLines = 240
		tailLines = 120
		headChars = 16 * 1024
		tailChars = 8 * 1024
	}

	if isHistorical {
		omittedMsg = "lines of historical tool output omitted by Cyrene Gateway"
		omittedCharMsg = "characters of historical tool output omitted by Cyrene Gateway"
		if level == RTKBalanced {
			safeLineThreshold = 200
			maxCharThreshold = 12 * 1024
			headLines = 100
			tailLines = 60
			headChars = 6 * 1024
			tailChars = 3 * 1024
		} else {
			safeLineThreshold = 80
			maxCharThreshold = 4 * 1024 // 4 KiB (~1k tokens)
			headLines = 30
			tailLines = 20
			headChars = maxCharThreshold / 2
			tailChars = maxCharThreshold / 4
		}
	}

	lines := strings.Split(text, "\n")
	if len(lines) <= safeLineThreshold && n <= maxCharThreshold {
		return text
	}

	// Phase 2: Smart line-based truncation if exceeding line threshold
	if len(lines) > safeLineThreshold {
		if len(lines) > headLines+tailLines {
			head := lines[:headLines]
			tail := lines[len(lines)-tailLines:]
			omitted := len(lines) - headLines - tailLines

			var sb strings.Builder
			sb.WriteString(strings.Join(head, "\n"))
			sb.WriteString("\n\n... [")
			sb.WriteString(strconv.Itoa(omitted))
			sb.WriteString(" ")
			sb.WriteString(omittedMsg)
			sb.WriteString("] ...\n\n")
			sb.WriteString(strings.Join(tail, "\n"))

			result := sb.String()
			if len(result) < n {
				return result
			}
		}
	}

	// Phase 3: Fallback character truncation for massive outputs with few/no newlines
	if n > maxCharThreshold {
		head := safeSliceHead(text, headChars)
		tail := safeSliceTail(text, tailChars)
		omittedChars := n - len(head) - len(tail)
		if omittedChars > 0 {
			var sb strings.Builder
			sb.WriteString(head)
			sb.WriteString("\n\n... [")
			sb.WriteString(strconv.Itoa(omittedChars))
			sb.WriteString(" ")
			sb.WriteString(omittedCharMsg)
			sb.WriteString("] ...\n\n")
			sb.WriteString(tail)

			result := sb.String()
			if len(result) < n {
				return result
			}
		}
	}

	return text
}

func safeSliceHead(text string, limit int) string {
	if limit >= len(text) {
		return text
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit]
}

func safeSliceTail(text string, limit int) string {
	if limit >= len(text) {
		return text
	}
	start := len(text) - limit
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}
	return text[start:]
}
