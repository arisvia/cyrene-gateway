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
