package translator

import (
	"testing"
)

func TestReasoningEffortTranslation(t *testing.T) {
	// 1. OpenAI -> Claude thinking conversion
	bodyClaude := map[string]any{
		"model":            "gpt-4",
		"reasoning_effort": "high",
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
