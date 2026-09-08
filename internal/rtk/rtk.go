// Package rtk implements token-saving optimizations: RTK compression of tool
// results, caveman terse-style prompts, and ponytail minimal-code prompts.
package rtk

import (
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

// compressText applies smart truncation to large tool outputs.
func compressText(text string) string {
	n := len(text)
	if n < minCompressSize || n > rawCap {
		return text
	}

	lines := strings.Split(text, "\n")
	if len(lines) > 250 {
		// Smart truncate: keep head + tail
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

	// Fallback for massive outputs with few/no newlines (e.g. minified JSON/dumps)
	const maxCharThreshold = 16 * 1024 // 16 KiB (~4k tokens)
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
