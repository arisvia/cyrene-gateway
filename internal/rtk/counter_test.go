package rtk

import (
	"testing"
)

func TestEstimateTextTokens(t *testing.T) {
	if got := EstimateTextTokens(""); got != 0 {
		t.Errorf("empty string should be 0, got %d", got)
	}

	asciiText := "Hello, world! This is a test of the token estimator."
	asciiTokens := EstimateTextTokens(asciiText)
	if asciiTokens < 10 || asciiTokens > 18 {
		t.Errorf("expected ~13 tokens for ASCII sentence, got %d", asciiTokens)
	}

	cjkText := "你好世界，这是一个基于 Cyrene Gateway 的多语言分词估算器测试。"
	cjkTokens := EstimateTextTokens(cjkText)
	if cjkTokens < 15 || cjkTokens > 35 {
		t.Errorf("expected ~25 tokens for CJK sentence, got %d", cjkTokens)
	}
}

func TestEstimateAnthropicTokens(t *testing.T) {
	if got := EstimateAnthropicTokens(nil); got != 0 {
		t.Errorf("nil body should be 0, got %d", got)
	}

	body := map[string]any{
		"model":  "claude-3-7-sonnet-20250219",
		"system": "You are a helpful coding assistant.",
		"messages": []any{
			map[string]any{"role": "user", "content": "Write a quick hello world in Go."},
			map[string]any{"role": "assistant", "content": "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello, world!\")\n}"},
			map[string]any{"role": "user", "content": "Thanks!"},
		},
		"tools": []any{
			map[string]any{
				"name":        "bash",
				"description": "Run shell command",
				"input_schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{"type": "string"},
					},
					"required": []any{"command"},
				},
			},
		},
	}

	tokens := EstimateAnthropicTokens(body)
	// Body has system prompt (~12), 3 messages (~50), 1 tool (~35), plus framing (~20)
	// Total should be around 100-140 tokens.
	if tokens < 80 || tokens > 200 {
		t.Errorf("expected tokens in range [80, 200], got %d", tokens)
	}
}
