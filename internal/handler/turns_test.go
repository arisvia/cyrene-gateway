package handler

import "testing"

func TestCountTurnsCountsOnlyUserMessages(t *testing.T) {
	msgs := []Message{
		{Role: "system"},
		{Role: "user"},
		{Role: "assistant"},
		{Role: "tool"},
		{Role: "USER"},
		{Role: "assistant"},
		{Role: "user"},
	}
	if got := countTurns(msgs); got != 3 {
		t.Fatalf("countTurns = %d, want 3 (system/assistant/tool are not turns)", got)
	}
	if got := countTurns(nil); got != 0 {
		t.Fatalf("countTurns(nil) = %d, want 0", got)
	}
}

func TestCountTurnsRawMatchesParsedCount(t *testing.T) {
	body := map[string]any{"messages": []any{
		map[string]any{"role": "user", "content": "hi"},
		map[string]any{"role": "assistant", "content": "hello"},
		map[string]any{"role": "user", "content": "again"},
		map[string]any{"role": 42}, // malformed rows are skipped, not fatal
	}}
	if got := countTurnsRaw(body); got != 2 {
		t.Fatalf("countTurnsRaw = %d, want 2", got)
	}
	if got := countTurnsRaw(map[string]any{}); got != 0 {
		t.Fatalf("countTurnsRaw(empty) = %d, want 0", got)
	}
}

func TestRawUserAgentCapsLength(t *testing.T) {
	long := ""
	for i := 0; i < 400; i++ {
		long += "a"
	}
	r := newRequestWithUA(long)
	got := rawUserAgent(r)
	if len(got) > 203 {
		t.Fatalf("rawUserAgent not capped: %d bytes", len(got))
	}
	if got[:3] != "aaa" {
		t.Fatalf("unexpected prefix %q", got[:3])
	}
}
