package loopguard

import (
	"encoding/json"
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
	InjectTerminationPrompt(body, "openai")

	msgs := body["messages"].([]any)
	sys := msgs[0].(map[string]any)
	content := sys["content"].(string)
	if !contains(content, TerminationPrompt) {
		t.Error("expected termination prompt in system message")
	}

	// Idempotency: inject again should not duplicate
	InjectTerminationPrompt(body, "openai")
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

func TestInjectTerminationPrompt_Anthropic(t *testing.T) {
	body := map[string]any{
		"system": "You are Claude.",
	}
	InjectTerminationPrompt(body, "anthropic")
	sys := body["system"].(string)
	if !contains(sys, TerminationPrompt) {
		t.Error("expected termination prompt in anthropic system field")
	}
}

func TestInjectTerminationPrompt_Gemini(t *testing.T) {
	body := map[string]any{}
	InjectTerminationPrompt(body, "gemini")
	sys, ok := body["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatal("expected systemInstruction to be created")
	}
	parts := sys["parts"].([]any)
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0].(map[string]any)["text"] != TerminationPrompt {
		t.Error("expected termination prompt in gemini systemInstruction")
	}
}

func TestInjectLoopHint_NoSystemMessage(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "Hi"},
		},
	}
	InjectLoopHint(body, "openai", "STOP looping")
	msgs := body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after injection, got %d", len(msgs))
	}
	sys := msgs[0].(map[string]any)
	if sys["role"] != "system" || sys["content"] != "STOP looping" {
		t.Error("expected prepended system message with loop hint")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
