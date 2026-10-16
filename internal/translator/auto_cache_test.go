package translator

import (
	"strings"
	"testing"
)

func TestAutoInjectClaudeCacheControl_Disabled(t *testing.T) {
	body := map[string]any{
		"system": strings.Repeat("system instructions ", 100),
		"tools": []any{
			map[string]any{"name": "bash", "description": "run command"},
		},
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
	}
	AutoInjectClaudeCacheControl(body, false)

	if CountClaudeCacheControl(body) != 0 {
		t.Fatalf("expected 0 cache_control markers when disabled, got %d", CountClaudeCacheControl(body))
	}
}

func TestAutoInjectClaudeCacheControl_Enabled(t *testing.T) {
	// Long system prompt (>1000 chars)
	longSystem := strings.Repeat("system prompt instruction line. ", 50)
	body := map[string]any{
		"system": longSystem,
		"tools": []any{
			map[string]any{"name": "tool1", "description": "first tool"},
			map[string]any{"name": "tool2", "description": "last tool"},
		},
		"messages": []any{
			map[string]any{"role": "user", "content": "turn 1 user"},
			map[string]any{"role": "assistant", "content": "turn 1 assistant"},
			map[string]any{"role": "user", "content": "turn 2 user (prefix)"},
			map[string]any{"role": "assistant", "content": "turn 2 assistant"},
			map[string]any{"role": "user", "content": "turn 3 user (latest)"},
		},
	}

	AutoInjectClaudeCacheControl(body, true)

	// 1. Verify system prompt converted to block with cache_control
	sysBlocks, ok := body["system"].([]any)
	if !ok || len(sysBlocks) != 1 {
		t.Fatalf("expected system to be converted to []any, got %T", body["system"])
	}
	sys0 := sysBlocks[0].(map[string]any)
	if sys0["cache_control"] == nil {
		t.Errorf("expected system block to have cache_control")
	}

	// 2. Verify last tool has cache_control (tool1 does not, tool2 does)
	tools := body["tools"].([]any)
	tool1 := tools[0].(map[string]any)
	tool2 := tools[1].(map[string]any)
	if tool1["cache_control"] != nil {
		t.Errorf("expected tool1 not to have cache_control")
	}
	if tool2["cache_control"] == nil {
		t.Errorf("expected last tool (tool2) to have cache_control")
	}

	// 3. Verify second-to-last user turn (index 2: "turn 2 user (prefix)") has cache_control
	msgs := body["messages"].([]any)
	msg2 := msgs[2].(map[string]any)
	msg2Content, ok := msg2["content"].([]any)
	if !ok || len(msg2Content) != 1 {
		t.Fatalf("expected second-to-last user message content to be []any, got %T", msg2["content"])
	}
	if msg2Content[0].(map[string]any)["cache_control"] == nil {
		t.Errorf("expected second-to-last user turn to have cache_control")
	}

	// 4. Verify latest user turn (index 4) does NOT have cache_control
	msg4 := msgs[4].(map[string]any)
	if _, ok := msg4["cache_control"]; ok {
		t.Errorf("expected latest user turn not to have cache_control")
	}

	// 5. Total count must be <= 4
	total := CountClaudeCacheControl(body)
	if total > 4 {
		t.Fatalf("expected <= 4 markers, got %d", total)
	}
}

func TestAutoInjectClaudeCacheControl_CapsAtFour(t *testing.T) {
	ephemeral := map[string]any{"type": "ephemeral"}
	body := map[string]any{
		"tools": []any{
			map[string]any{"name": "t1", "cache_control": ephemeral},
			map[string]any{"name": "t2", "cache_control": ephemeral},
			map[string]any{"name": "t3", "cache_control": ephemeral},
			map[string]any{"name": "t4", "cache_control": ephemeral},
		},
		"system": strings.Repeat("long system prompt ", 100),
	}

	AutoInjectClaudeCacheControl(body, true)
	if count := CountClaudeCacheControl(body); count != 4 {
		t.Fatalf("expected count capped at 4, got %d", count)
	}
}
