// Package rtk implements token-saving optimizations: RTK compression of tool
// results, caveman terse-style prompts, and ponytail minimal-code prompts.
package rtk

import (
	"strings"
)

const (
	rawCap          = 10 * 1024 * 1024 // 10 MiB
	minCompressSize = 500              // bytes; skip tiny blobs
)

// CompressMessages compresses tool_result content in-place within an OpenAI-shaped
// request body (map with "messages" key). Returns bytes saved or 0.
func CompressMessages(body map[string]any, enabled bool) int {
	if !enabled || body == nil {
		return 0
	}

	msgs, ok := body["messages"].([]any)
	if !ok {
		return 0
	}

	saved := 0
	for _, m := range msgs {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}

		// OpenAI tool message: {role:"tool", content:"string"}
		role, _ := msg["role"].(string)
		if role == "tool" {
			if content, ok := msg["content"].(string); ok {
				compressed := compressText(content)
				if len(compressed) < len(content) {
					saved += len(content) - len(compressed)
					msg["content"] = compressed
				}
			}
			continue
		}

		// Content blocks array with tool_result entries
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
	if len(lines) <= 250 {
		return text
	}

	// Smart truncate: keep head + tail
	const headLines = 120
	const tailLines = 60

	if len(lines) <= headLines+tailLines {
		return text
	}

	head := lines[:headLines]
	tail := lines[len(lines)-tailLines:]
	omitted := len(lines) - headLines - tailLines

	var sb strings.Builder
	sb.WriteString(strings.Join(head, "\n"))
	sb.WriteString("\n\n... [")
	sb.WriteString(itoa(omitted))
	sb.WriteString(" lines omitted] ...\n\n")
	sb.WriteString(strings.Join(tail, "\n"))

	result := sb.String()
	if len(result) >= n {
		return text
	}
	return result
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
