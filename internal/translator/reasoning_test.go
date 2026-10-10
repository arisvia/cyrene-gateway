package translator

import (
	"testing"
)

func TestReasoningEffortTranslation(t *testing.T) {
	// 1. OpenAI -> Claude thinking conversion (drops temperature, expands max_tokens)
	bodyClaude := map[string]any{
		"model":            "gpt-4",
		"reasoning_effort": "high",
		"temperature":      0.7,
		"max_tokens":       float64(2048),
		"messages": []any{
			map[string]any{"role": "user", "content": "solve this math puzzle"},
		},
	}
	resClaude, err := openAIToClaude("claude-3-7-sonnet", bodyClaude, false)
	if err != nil {
		t.Fatalf("unexpected claude translation error: %v", err)
	}
	thinking, ok := resClaude["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("expected thinking map in claude request, got %v", resClaude["thinking"])
	}
	if thinking["type"] != "enabled" || thinking["budget_tokens"] != 16384 {
		t.Errorf("expected budget_tokens=16384 for high effort, got %+v", thinking)
	}
	if _, hasTemp := resClaude["temperature"]; hasTemp {
		t.Errorf("expected temperature to be dropped when thinking is enabled, got %v", resClaude["temperature"])
	}
	if maxTokens := resClaude["max_tokens"].(int); maxTokens <= 16384 {
		t.Errorf("expected max_tokens to be lifted above budget 16384, got %d", maxTokens)
	}

	// 1b. Adaptive thinking test
	bodyAdaptive := map[string]any{
		"model":            "gpt-4",
		"reasoning_effort": "adaptive",
		"temperature":      0.5,
		"messages": []any{
			map[string]any{"role": "user", "content": "think adaptively"},
		},
	}
	resAdaptive, err := openAIToClaude("claude-3-7-sonnet", bodyAdaptive, false)
	if err != nil {
		t.Fatalf("unexpected adaptive translation error: %v", err)
	}
	thAdaptive, ok := resAdaptive["thinking"].(map[string]any)
	if !ok || thAdaptive["type"] != "adaptive" {
		t.Errorf("expected thinking.type=adaptive, got %+v", thAdaptive)
	}
	if _, hasTemp := resAdaptive["temperature"]; hasTemp {
		t.Errorf("expected temperature to be dropped for adaptive thinking, got %v", resAdaptive["temperature"])
	}
	// 1c. Claude 5.x must use adaptive thinking and never forged budget tokens.
	bodyClaude5 := map[string]any{
		"reasoning_effort": "high",
		"temperature":      0.6,
		"messages": []any{
			map[string]any{"role": "user", "content": "use adaptive thinking"},
		},
	}
	resClaude5, err := openAIToClaude("anthropic/claude-haiku-5-5", bodyClaude5, false)
	if err != nil {
		t.Fatalf("unexpected Claude 5.x translation error: %v", err)
	}
	thClaude5, ok := resClaude5["thinking"].(map[string]any)
	if !ok || thClaude5["type"] != "adaptive" {
		t.Fatalf("expected Claude 5.x adaptive thinking, got %+v", thClaude5)
	}
	if _, exists := thClaude5["budget_tokens"]; exists {
		t.Fatalf("Claude 5.x must not receive budget_tokens: %+v", thClaude5)
	}
	if resClaude5["effort"] != "high" {
		t.Fatalf("expected Claude 5.x effort=high, got %v", resClaude5["effort"])
	}
	ocClaude5, ok := resClaude5["output_config"].(map[string]any)
	if !ok || ocClaude5["effort"] != "high" {
		t.Fatalf("expected output_config.effort=high, got %+v", resClaude5["output_config"])
	}

	// Round-trip Claude -> OpenAI preserves high effort (no degradation to medium)
	openAIRoundTrip, err := ClaudeToOpenAIRequest(resClaude5)
	if err != nil {
		t.Fatalf("ClaudeToOpenAIRequest round-trip failed: %v", err)
	}
	if openAIRoundTrip["reasoning_effort"] != "high" {
		t.Fatalf("expected round-trip reasoning_effort=high, got %v", openAIRoundTrip["reasoning_effort"])
	}
	if _, exists := resClaude5["temperature"]; exists {
		t.Fatal("temperature must be omitted when adaptive thinking is enabled")
	}

	bodyClaude5Max := map[string]any{
		"reasoning_effort": "max",
		"messages":         []any{map[string]any{"role": "user", "content": "think deeply"}},
	}
	resClaude5Max, err := openAIToClaude("claude-sonnet-5-5", bodyClaude5Max, false)
	if err != nil {
		t.Fatalf("unexpected Claude 5.x max translation error: %v", err)
	}
	if resClaude5Max["effort"] != "xhigh" {
		t.Fatalf("expected max compatibility alias to map to xhigh, got %v", resClaude5Max["effort"])
	}

	// 1d. Claude 3.5 models must retain manual budget and NOT trigger adaptive thinking.
	bodyClaude35 := map[string]any{
		"reasoning_effort": "high",
		"messages":         []any{map[string]any{"role": "user", "content": "manual budget"}},
	}
	resClaude35, err := openAIToClaude("anthropic/claude-3-5-sonnet", bodyClaude35, false)
	if err != nil {
		t.Fatalf("unexpected Claude 3.5 translation error: %v", err)
	}
	thClaude35, ok := resClaude35["thinking"].(map[string]any)
	if !ok || thClaude35["type"] != "enabled" {
		t.Fatalf("expected Claude 3.5 thinking.type=enabled, got %+v", thClaude35)
	}
	if thClaude35["budget_tokens"] != 16384 {
		t.Fatalf("expected Claude 3.5 budget_tokens=16384, got %v", thClaude35["budget_tokens"])
	}

	// 1e. claude-opus-5 (suffix -5 without minor) must be detected as Claude 5.x adaptive.
	resClaudeOpus5, err := openAIToClaude("claude-opus-5", bodyClaude5, false)
	if err != nil {
		t.Fatalf("unexpected claude-opus-5 translation error: %v", err)
	}
	thOpus5, ok := resClaudeOpus5["thinking"].(map[string]any)
	if !ok || thOpus5["type"] != "adaptive" {
		t.Fatalf("expected claude-opus-5 thinking.type=adaptive, got %+v", thOpus5)
	}
	if _, exists := thOpus5["budget_tokens"]; exists {
		t.Fatalf("claude-opus-5 must not receive budget_tokens")
	}

	// 1f. Explicit none/off must disable reasoning and keep temperature even on Claude 5.x.
	bodyClaude5Off := map[string]any{
		"reasoning_effort": "none",
		"temperature":      0.7,
		"messages":         []any{map[string]any{"role": "user", "content": "no reasoning"}},
	}
	resClaude5Off, err := openAIToClaude("claude-sonnet-5-5", bodyClaude5Off, false)
	if err != nil {
		t.Fatalf("unexpected Claude 5.x off translation error: %v", err)
	}
	if _, hasThinking := resClaude5Off["thinking"]; hasThinking {
		t.Fatalf("expected no thinking block when reasoning_effort=none, got %+v", resClaude5Off["thinking"])
	}
	if _, hasEffort := resClaude5Off["effort"]; hasEffort {
		t.Fatalf("expected no effort field when reasoning_effort=none, got %v", resClaude5Off["effort"])
	}
	if temp, ok := resClaude5Off["temperature"].(float64); !ok || temp != 0.7 {
		t.Fatalf("expected temperature 0.7 to be preserved, got %v", resClaude5Off["temperature"])
	}

	// 2. OpenAI -> Gemini thinkingConfig conversion
	bodyGemini := map[string]any{
		"model":            "gpt-4",
		"reasoning_effort": "low",
		"messages": []any{
			map[string]any{"role": "user", "content": "quick answer"},
		},
	}
	resGemini, err := openAIToGemini("gemini-2.5-flash", bodyGemini, false)
	if err != nil {
		t.Fatalf("unexpected gemini translation error: %v", err)
	}
	genCfg, ok := resGemini["generationConfig"].(map[string]any)
	if !ok {
		t.Fatalf("expected generationConfig in gemini request, got %v", resGemini["generationConfig"])
	}
	thinkingCfg, ok := genCfg["thinkingConfig"].(map[string]any)
	if !ok {
		t.Fatalf("expected thinkingConfig in gemini generationConfig, got %v", genCfg["thinkingConfig"])
	}
	if thinkingCfg["thinkingBudget"] != 2048 {
		t.Errorf("expected thinkingBudget=2048 for low effort, got %+v", thinkingCfg)
	}
}
