package loopguard

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

func makeMsg(role, content string) Message {
	return Message{
		Role:    role,
		Content: json.RawMessage(`"` + content + `"`),
	}
}

func makeToolCallMsg(name, args string) Message {
	tc := ToolCall{Function: ToolCallFunction{Name: name, Arguments: args}}
	tcs, _ := json.Marshal([]ToolCall{tc})
	var raw []ToolCall
	json.Unmarshal(tcs, &raw)
	return Message{
		Role:      "assistant",
		Content:   json.RawMessage(`""`),
		ToolCalls: raw,
	}
}

func TestDetectLoop_NoLoop(t *testing.T) {
	msgs := []Message{
		makeMsg("user", "Hello"),
		makeMsg("assistant", "Hi there"),
		makeMsg("user", "How are you?"),
		makeMsg("assistant", "I'm fine"),
	}
	result := DetectLoop(msgs)
	if result.Detected {
		t.Errorf("expected no loop, got detected with hint: %s", result.Hint)
	}
}

func TestDetectLoop_SingleToolRepeat(t *testing.T) {
	msgs := []Message{
		makeMsg("user", "Read the file"),
		makeToolCallMsg("read_file", `{"path":"/tmp/a.txt"}`),
		makeToolCallMsg("read_file", `{"path":"/tmp/a.txt"}`),
		makeToolCallMsg("read_file", `{"path":"/tmp/a.txt"}`),
	}
	result := DetectLoop(msgs)
	if !result.Detected {
		t.Error("expected loop detection for repeated tool calls")
	}
	if result.Hint == "" {
		t.Error("expected non-empty hint")
	}
}

func TestDetectLoop_SequenceRepeat(t *testing.T) {
	msgs := []Message{
		makeMsg("user", "Do the thing"),
		makeToolCallMsg("read_file", `{"path":"/a"}`),
		makeToolCallMsg("write_file", `{"path":"/b"}`),
		makeToolCallMsg("read_file", `{"path":"/a"}`),
		makeToolCallMsg("write_file", `{"path":"/b"}`),
	}
	result := DetectLoop(msgs)
	if !result.Detected {
		t.Error("expected loop detection for repeated sequence")
	}
}

func TestDetectLoop_TextRepeat(t *testing.T) {
	repeatedText := "I need to read the key files to understand the structure"
	msgs := []Message{
		makeMsg("user", "Help me"),
		makeMsg("assistant", repeatedText),
		makeMsg("user", "Continue"),
		makeMsg("assistant", repeatedText),
		makeMsg("user", "Go on"),
		makeMsg("assistant", repeatedText),
	}
	result := DetectLoop(msgs)
	if !result.Detected {
		t.Error("expected loop detection for repeated text")
	}
}

func TestDetectLoop_ArgKeyOrderInsensitive(t *testing.T) {
	// Same args, different key order should still be detected
	msgs := []Message{
		makeToolCallMsg("search", `{"query":"test","limit":10}`),
		makeToolCallMsg("search", `{"limit":10,"query":"test"}`),
		makeToolCallMsg("search", `{"query":"test","limit":10}`),
	}
	result := DetectLoop(msgs)
	if !result.Detected {
		t.Error("expected loop detection regardless of JSON key order")
	}
}

func TestInjectTerminationPrompt_OpenAI(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": "You are helpful."},
			map[string]any{"role": "user", "content": "Hi"},
		},
	}
	InjectTerminationPrompt(body)

	msgs := body["messages"].([]any)
	sys := msgs[0].(map[string]any)
	content := sys["content"].(string)
	if !strings.Contains(content, TerminationPrompt) {
		t.Error("expected termination prompt in system message")
	}

	// Idempotency: inject again should not duplicate
	InjectTerminationPrompt(body)
	sys = body["messages"].([]any)[0].(map[string]any)
	content = sys["content"].(string)
	count := 0
	for i := 0; i < len(content)-len(TerminationPrompt)+1; i++ {
		if content[i:i+len(TerminationPrompt)] == TerminationPrompt {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 injection, got %d", count)
	}
}

func TestInjectLoopHint_NoSystemMessage(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "Hi"},
		},
	}
	InjectLoopHint(body, "STOP looping")
	msgs := body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after injection, got %d", len(msgs))
	}
	sys := msgs[0].(map[string]any)
	if sys["role"] != "system" || sys["content"] != "STOP looping" {
		t.Error("expected prepended system message with loop hint")
	}
}

// legacySequenceRepeat is the pre-optimization oracle. Keep its inputs small.
func legacySequenceRepeat(seq []string) bool {
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

func TestSequenceRepeatSemantics(t *testing.T) {
	for _, tc := range []struct {
		name string
		seq  []string
		want bool
	}{
		{"empty", nil, false},
		{"too short", []string{"a", "b", "a"}, false},
		{"overlap only", []string{"a", "b", "a", "b", "a"}, true},
		{"overlapping identical calls", []string{"a", "a", "a", "b"}, false},
		{"adjacent", []string{"a", "b", "a", "b"}, true},
		{"separated", []string{"a", "b", "x", "a", "b"}, true},
		{"not at tail", []string{"a", "b", "a", "b", "c", "d", "e", "f", "g", "h"}, true},
		{"unique", []string{"a", "b", "c", "d", "e", "f"}, false},
		{"legacy delimiter equality", []string{"a|b", "c", "a", "b|c"}, true},
		{"empty elements", []string{"", "", "", ""}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectSequenceRepeat(tc.seq); got != tc.want {
				t.Fatalf("got %v want %v for %q", got, tc.want, tc.seq)
			}
		})
	}
}

func TestSequenceRepeatDifferential(t *testing.T) {
	rng := rand.New(rand.NewPCG(17, 29))
	alphabet := []string{"a", "b", "c", "", "a|b", "b|c", "|", "工具"}
	for trial := range 1000 {
		seq := make([]string, rng.IntN(17))
		for i := range seq {
			seq[i] = alphabet[rng.IntN(len(alphabet))]
		}
		if got, want := detectSequenceRepeat(seq), legacySequenceRepeat(seq); got != want {
			t.Fatalf("trial %d: seq=%q got=%v want=%v", trial, seq, got, want)
		}
	}
}

func TestSingleRepeatPrecedesSequence(t *testing.T) {
	messages := []Message{
		makeToolCallMsg("a", `{}`), makeToolCallMsg("b", `{}`),
		makeToolCallMsg("a", `{}`), makeToolCallMsg("b", `{}`),
		makeToolCallMsg("a", `{}`),
	}
	result := DetectLoop(messages)
	if !result.Detected || !strings.Contains(result.Hint, "same tool with identical arguments") {
		t.Fatalf("single-repeat hint must take precedence: %+v", result)
	}
}

func TestSequenceRepeatAllocationBudget(t *testing.T) {
	seq := make([]string, 24)
	for i := range seq {
		seq[i] = fmt.Sprintf("tool::%d", i)
	}
	allocs := testing.AllocsPerRun(2, func() { detectSequenceRepeat(seq) })
	if allocs > 64 {
		t.Fatalf("%.0f allocations for 24 calls: windows must not be repeatedly joined", allocs)
	}
}

func BenchmarkSequenceRepeatUnique(b *testing.B) {
	for _, n := range []int{100, 200, 400} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			seq := make([]string, n)
			for i := range seq {
				seq[i] = fmt.Sprintf("tool::%d", i)
			}
			b.ReportAllocs()
			for b.Loop() {
				if detectSequenceRepeat(seq) {
					b.Fatal("unique calls reported as repeated")
				}
			}
		})
	}
}
