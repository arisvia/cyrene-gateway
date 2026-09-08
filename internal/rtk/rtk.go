// Package rtk implements token-saving optimizations: RTK compression of tool
// results, caveman terse-style prompts, and ponytail minimal-code prompts.
package rtk

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

const (
	rawCap          = 10 * 1024 * 1024 // 10 MiB
	minCompressSize = 500              // bytes; skip tiny blobs
)

// CompressMessages compresses tool_result content in-place within an OpenAI,
// Anthropic, or Gemini shaped request body. Returns bytes saved or 0.
func CompressMessages(body map[string]any, enabled bool) int {
	if !enabled || body == nil {
		return 0
	}

	saved := 0

	// OpenAI / Anthropic format: body["messages"]
	if msgs, ok := body["messages"].([]any); ok {
		for _, m := range msgs {
			msg, ok := m.(map[string]any)
			if !ok {
				continue
			}

			// OpenAI tool message: {role:"tool", content:"string"} or content parts
			role, _ := msg["role"].(string)
			if role == "tool" {
				if content, ok := msg["content"].(string); ok {
					compressed := compressText(content)
					if len(compressed) < len(content) {
						saved += len(content) - len(compressed)
						msg["content"] = compressed
					}
				} else if parts, ok := msg["content"].([]any); ok {
					for _, part := range parts {
						if p, ok := part.(map[string]any); ok {
							if text, ok := p["text"].(string); ok {
								compressed := compressText(text)
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
				if isError, _ := b["is_error"].(bool); isError {
					continue
				}
				if text, ok := b["content"].(string); ok {
					compressed := compressText(text)
					if len(compressed) < len(text) {
						saved += len(text) - len(compressed)
						b["content"] = compressed
					}
				} else if innerArr, ok := b["content"].([]any); ok {
					for _, inner := range innerArr {
						if ib, ok := inner.(map[string]any); ok {
							if text, ok := ib["text"].(string); ok {
								compressed := compressText(text)
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
	if contents, ok := body["contents"].([]any); ok {
		for _, c := range contents {
			contentMap, ok := c.(map[string]any)
			if !ok {
				continue
			}
			parts, ok := contentMap["parts"].([]any)
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
				if content, ok := resp["content"].(string); ok {
					compressed := compressText(content)
					if len(compressed) < len(content) {
						saved += len(content) - len(compressed)
						resp["content"] = compressed
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

// compressText applies lossless cleaning/compacting followed by smart truncation.
func compressText(text string) string {
	n := len(text)
	if n < minCompressSize || n > rawCap {
		return text
	}

	// Phase 1: Lossless pre-compression
	cleaned := cleanAndCompactText(text)
	if len(cleaned) < n {
		text = cleaned
		n = len(text)
	}

	// Check if lossless compression brought the text within comfortable bounds
	const safeLineThreshold = 250
	const maxCharThreshold = 16 * 1024 // 16 KiB (~4k tokens)

	lines := strings.Split(text, "\n")
	if len(lines) <= safeLineThreshold && n <= maxCharThreshold {
		return text
	}

	// Phase 2: Smart line-based truncation if exceeding line threshold
	if len(lines) > safeLineThreshold {
		const headLines = 120
		const tailLines = 60

		if len(lines) > headLines+tailLines {
			head := lines[:headLines]
			tail := lines[len(lines)-tailLines:]
			omitted := len(lines) - headLines - tailLines

			var sb strings.Builder
			sb.WriteString(strings.Join(head, "\n"))
			sb.WriteString("\n\n... [")
			sb.WriteString(strconv.Itoa(omitted))
			sb.WriteString(" lines omitted] ...\n\n")
			sb.WriteString(strings.Join(tail, "\n"))

			result := sb.String()
			if len(result) < n {
				return result
			}
		}
	}

	// Phase 3: Fallback character truncation for massive outputs with few/no newlines
	if n > maxCharThreshold {
		const headChars = 8 * 1024
		const tailChars = 4 * 1024
		omittedChars := n - headChars - tailChars

		var sb strings.Builder
		sb.WriteString(text[:headChars])
		sb.WriteString("\n\n... [")
		sb.WriteString(strconv.Itoa(omittedChars))
		sb.WriteString(" characters omitted] ...\n\n")
		sb.WriteString(text[n-tailChars:])

		result := sb.String()
		if len(result) < n {
			return result
		}
	}

	return text
}
