package loopguard

import (
	"encoding/json"
	"sort"
	"strings"
)

// Thresholds for loop detection (mirrors VansRouter loopGuard.js).
const (
	SingleRepeatThreshold   = 3 // same tool+args appearing >= this many times
	SequenceRepeatThreshold = 2 // same sequence of N tool calls appearing >= this many times
	MinSequenceLength       = 2 // minimum sequence length to detect

	TextMessageRepeatThreshold  = 3  // same normalized assistant message >= this many times
	TextSentenceRepeatThreshold = 3  // same sentence across all assistant msgs >= this many times
	MinTextLength               = 12 // ignore tiny fragments
)

// Result holds the loop detection outcome.
type Result struct {
	Detected bool
	Hint     string
}

// Message represents a chat message for loop analysis.
type Message struct {
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	ToolCalls []ToolCall      `json:"tool_calls,omitempty"`
}

// ToolCall represents a tool call in an assistant message.
type ToolCall struct {
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction holds the function name and arguments.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// DetectLoop analyzes the conversation messages for repeating patterns.
// It checks both tool-call loops and text-only reasoning loops.
func DetectLoop(messages []Message) Result {
	if len(messages) == 0 {
		return Result{}
	}

	// Tool-call loop detection
	seq := extractToolCallSequence(messages)
	if len(seq) >= SingleRepeatThreshold {
		if h := detectSingleRepeat(seq); h != "" {
			return Result{
				Detected: true,
				Hint:     "You have called the same tool with identical arguments multiple times with no new progress. STOP repeating. Summarize findings from existing results or change your strategy.",
			}
		}
		if detectSequenceRepeat(seq) {
			return Result{
				Detected: true,
				Hint:     "You have repeated the same sequence of tool calls multiple times. This is a loop. STOP this pattern immediately. Summarize what you have already found or take a completely different approach.",
			}
		}
	}

	// Text-only loop detection
	if r := detectTextRepeat(messages); r.Detected {
		return r
	}

	return Result{}
}

// normalizeArgs sorts JSON object keys for stable comparison.
func normalizeArgs(argsStr string) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(argsStr), &obj); err != nil {
		return argsStr
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		kb, _ := json.Marshal(k)
		parts = append(parts, string(kb)+":"+string(obj[k]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func toolCallHash(tc ToolCall) string {
	name := tc.Function.Name
	args := normalizeArgs(tc.Function.Arguments)
	return name + "::" + args
}

// extractToolCallSequence extracts all tool_call hashes from conversation history in order.
func extractToolCallSequence(messages []Message) []string {
	var seq []string
	for _, msg := range messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				seq = append(seq, toolCallHash(tc))
			}
		}
	}
	return seq
}

// detectSingleRepeat checks if any tool call hash appears >= SingleRepeatThreshold times.
func detectSingleRepeat(seq []string) string {
	counts := make(map[string]int)
	for _, h := range seq {
		counts[h]++
		if counts[h] >= SingleRepeatThreshold {
			return h
		}
	}
	return ""
}

// detectSequenceRepeat checks if a sequence of N tool calls repeats >= SequenceRepeatThreshold times.
func detectSequenceRepeat(seq []string) bool {
	n := len(seq)
	for length := n / 2; length >= MinSequenceLength; length-- {
		for start := 0; start <= n-length*2; start++ {
			pattern := strings.Join(seq[start:start+length], "|")
			count := 0
			pos := 0
			for pos <= n-length {
				window := strings.Join(seq[pos:pos+length], "|")
				if window == pattern {
					count++
					pos += length
				} else {
					pos++
				}
			}
			if count >= SequenceRepeatThreshold {
				return true
			}
		}
	}
	return false
}

// messageText extracts plain-text content from a message.
func messageText(msg Message) string {
	if len(msg.Content) == 0 {
		return ""
	}
	// Try string first
	var s string
	if err := json.Unmarshal(msg.Content, &s); err == nil {
		return s
	}
	// Try content array
	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(msg.Content, &parts); err == nil {
		var texts []string
		for _, p := range parts {
			if p.Text != "" {
				texts = append(texts, p.Text)
			}
		}
		return strings.Join(texts, " ")
	}
	return ""
}

// normalizeText normalizes text for stable comparison.
func normalizeText(text string) string {
	lower := strings.ToLower(text)
	// Collapse whitespace
	fields := strings.Fields(lower)
	joined := strings.Join(fields, " ")
	// Strip trailing punctuation
	joined = strings.TrimRight(joined, ".!?…")
	return strings.TrimSpace(joined)
}

// splitSentences splits text into sentence-ish chunks.
func splitSentences(text string) []string {
	// Split on newlines and sentence delimiters
	replacer := strings.NewReplacer("\n", "\x00", ".", "\x00", "!", "\x00", "?", "\x00", "…", "\x00")
	split := strings.Split(replacer.Replace(text), "\x00")
	var result []string
	for _, s := range split {
		norm := normalizeText(s)
		if len(norm) >= MinTextLength {
			result = append(result, norm)
		}
	}
	return result
}

// extractAssistantTexts extracts all assistant message texts.
func extractAssistantTexts(messages []Message) []string {
	var texts []string
	for _, msg := range messages {
		if msg.Role == "assistant" {
			t := messageText(msg)
			if len(t) >= MinTextLength {
				texts = append(texts, t)
			}
		}
	}
	return texts
}

// detectTextRepeat detects text-only reasoning loops.
func detectTextRepeat(messages []Message) Result {
	texts := extractAssistantTexts(messages)
	if len(texts) < TextMessageRepeatThreshold {
		return Result{}
	}

	// 1. Exact message repeat (normalized)
	msgCounts := make(map[string]int)
	for _, t := range texts {
		norm := normalizeText(t)
		if len(norm) < MinTextLength {
			continue
		}
		msgCounts[norm]++
		if msgCounts[norm] >= TextMessageRepeatThreshold {
			return Result{
				Detected: true,
				Hint:     "You have repeated the same response multiple times without making progress. This is a text loop — you are NOT moving forward. STOP repeating yourself. Either call a tool to act, or give your final answer now with the information you already have.",
			}
		}
	}

	// 2. Sentence-level repeat across messages
	sentenceCounts := make(map[string]int)
	for _, t := range texts {
		for _, s := range splitSentences(t) {
			sentenceCounts[s]++
			if sentenceCounts[s] >= TextSentenceRepeatThreshold {
				return Result{
					Detected: true,
					Hint:     "You keep repeating the same planning statement without acting on it. STOP planning in circles. Either execute a tool call NOW, or provide your final answer with current knowledge. Do not restate your plan again.",
				}
			}
		}
	}

	return Result{}
}
