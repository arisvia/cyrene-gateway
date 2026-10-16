package translator

import "strings"

const (
	thinkStart = "<think>"
	thinkEnd   = "</think>"
)

// ThinkDeltaType identifies whether a streamed fragment is thinking or standard text.
type ThinkDeltaType int

const (
	DeltaText ThinkDeltaType = iota
	DeltaThinking
)

// StreamDelta represents a classified stream fragment (either thinking or text).
type StreamDelta struct {
	Type  ThinkDeltaType
	Delta string
}

// ThinkTagState tracks stateful extraction of <think>...</think> tags across streaming
// chunks. Because <think> and </think> tags may arrive split across multiple SSE events
// (e.g. "<thi" then "nk>"), the parser buffers potential tag prefixes.
type ThinkTagState struct {
	thinkingMode bool
	buf          string
	flushed      bool
}

// ProcessFeed processes an incoming text delta and classifies it into thinking and/or text deltas.
func (ts *ThinkTagState) ProcessFeed(delta string) []StreamDelta {
	if delta == "" {
		return nil
	}

	// Fast path: not inside a thinking block, nothing buffered, and no '<' that could open a tag.
	if !ts.thinkingMode && ts.buf == "" {
		if !strings.ContainsRune(delta, '<') {
			return []StreamDelta{{Type: DeltaText, Delta: delta}}
		}
	}

	ts.buf += delta
	var deltas []StreamDelta

	for len(ts.buf) > 0 {
		if !ts.thinkingMode {
			// Check if start tag is present
			if idx := strings.Index(ts.buf, thinkStart); idx >= 0 {
				if idx > 0 {
					deltas = append(deltas, StreamDelta{
						Type:  DeltaText,
						Delta: ts.buf[:idx],
					})
				}
				ts.buf = ts.buf[idx+len(thinkStart):]
				ts.thinkingMode = true
				continue
			}

			// No start tag found. Check if suffix of buf could be a partial start tag.
			partial := longestTagPrefix(ts.buf, thinkStart)
			if partial == len(ts.buf) {
				// Entire buffer is a prefix of <think> — hold for more data.
				return deltas
			}
			safe := len(ts.buf) - partial
			if safe > 0 {
				deltas = append(deltas, StreamDelta{
					Type:  DeltaText,
					Delta: ts.buf[:safe],
				})
			}
			ts.buf = ts.buf[safe:]
			return deltas
		}

		// In thinking mode — look for end tag </think>
		if idx := strings.Index(ts.buf, thinkEnd); idx >= 0 {
			thinkingText := ts.buf[:idx]
			if thinkingText != "" {
				deltas = append(deltas, StreamDelta{
					Type:  DeltaThinking,
					Delta: thinkingText,
				})
			}
			ts.buf = ts.buf[idx+len(thinkEnd):]
			ts.thinkingMode = false
			continue
		}

		// Check for partial end tag suffix
		partial := longestTagPrefix(ts.buf, thinkEnd)
		if partial == len(ts.buf) {
			return deltas // entirely potential suffix
		}
		safe := len(ts.buf) - partial
		if safe > 0 {
			deltas = append(deltas, StreamDelta{
				Type:  DeltaThinking,
				Delta: ts.buf[:safe],
			})
		}
		ts.buf = ts.buf[safe:]
		return deltas
	}

	return deltas
}

// Flush emits any remaining buffered content at stream end.
func (ts *ThinkTagState) Flush() []StreamDelta {
	if ts.flushed || len(ts.buf) == 0 {
		return nil
	}
	ts.flushed = true
	var deltas []StreamDelta
	if ts.thinkingMode {
		deltas = append(deltas, StreamDelta{
			Type:  DeltaThinking,
			Delta: ts.buf,
		})
	} else {
		deltas = append(deltas, StreamDelta{
			Type:  DeltaText,
			Delta: ts.buf,
		})
	}
	ts.buf = ""
	return deltas
}

// StripThinkTags extracts content enclosed in <think> tags from complete text.
// Returns (thinking, cleanText). For non-streaming use.
func StripThinkTags(content string) (string, string) {
	if !strings.Contains(content, thinkStart) {
		return "", content
	}

	var textBuilder strings.Builder
	var thinkBuilder strings.Builder
	pos := 0

	for pos < len(content) {
		startIdx := strings.Index(content[pos:], thinkStart)
		if startIdx < 0 {
			textBuilder.WriteString(content[pos:])
			break
		}
		textBuilder.WriteString(content[pos : pos+startIdx])
		pos += startIdx + len(thinkStart)

		endIdx := strings.Index(content[pos:], thinkEnd)
		if endIdx < 0 {
			// Unclosed <think> tag — treat remainder as thinking
			thinkBuilder.WriteString(content[pos:])
			break
		}
		thinkBuilder.WriteString(content[pos : pos+endIdx])
		pos += endIdx + len(thinkEnd)
	}

	return strings.TrimSpace(thinkBuilder.String()), strings.TrimSpace(textBuilder.String())
}

// longestTagPrefix returns the length of the longest suffix of s that matches a prefix of tag.
func longestTagPrefix(s, tag string) int {
	max := 0
	limit := len(s)
	if len(tag) < limit {
		limit = len(tag)
	}
	for l := limit; l > 0; l-- {
		if s[len(s)-l:] == tag[:l] {
			max = l
			break
		}
	}
	return max
}
