package provider

import (
	"testing"
)

func TestParseModel(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantProv  string
		wantModel string
	}{
		{"provider/model", "openai/gpt-4", "openai", "gpt-4"},
		{"alias/model", "claude/claude-3-opus", "anthropic", "claude-3-opus"},
		{"alias or/model", "or/llama-3", "openrouter", "llama-3"},
		{"bare model", "gpt-4", "", "gpt-4"},
		{"empty", "", "", ""},
		{"nested slash", "openai/gpt-4/turbo", "openai", "gpt-4/turbo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseModel(tt.input)
			if got.Provider != tt.wantProv {
				t.Errorf("ParseModel(%q).Provider = %q, want %q", tt.input, got.Provider, tt.wantProv)
			}
			if got.Model != tt.wantModel {
				t.Errorf("ParseModel(%q).Model = %q, want %q", tt.input, got.Model, tt.wantModel)
			}
		})
	}
}

func TestInferProviderFromModel(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"claude-3-opus", "anthropic"},
		{"gemini-1.5-pro", "gemini"},
		{"gpt-4o", "openai"},
		{"o1-preview", "openai"},
		{"o3-mini", "openai"},
		{"deepseek-chat", "deepseek"},
		{"grok-2", "xai"},
		{"llama-3-70b", "openrouter"},
		{"mistral-large", "mistral"},
		{"unknown-model", "openai"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := InferProviderFromModel(tt.model)
			if got != tt.expected {
				t.Errorf("InferProviderFromModel(%q) = %q, want %q", tt.model, got, tt.expected)
			}
		})
	}
}

func TestResolveProviderAlias(t *testing.T) {
	tests := []struct {
		alias    string
		expected string
	}{
		{"openai", "openai"},
		{"oai", "openai"},
		{"claude", "anthropic"},
		{"anthropic", "anthropic"},
		{"google", "gemini"},
		{"gemini", "gemini"},
		{"or", "openrouter"},
		{"ds", "deepseek"},
		{"sf", "siliconflow"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			got := ResolveProviderAlias(tt.alias)
			if got != tt.expected {
				t.Errorf("ResolveProviderAlias(%q) = %q, want %q", tt.alias, got, tt.expected)
			}
		})
	}
}

func TestComboManager_GetRotatedModels_Fallback(t *testing.T) {
	cm := NewComboManager()
	models := []string{"openai/gpt-4", "anthropic/claude-3", "gemini/gemini-1.5"}

	// Fallback strategy should return models unchanged
	got := cm.GetRotatedModels(models, "test", StrategyFallback, 1)
	if len(got) != 3 || got[0] != "openai/gpt-4" {
		t.Errorf("Fallback strategy should return models unchanged, got %v", got)
	}
}

func TestComboManager_GetRotatedModels_RoundRobin(t *testing.T) {
	cm := NewComboManager()
	models := []string{"a", "b", "c"}

	// First call: index 0 → [a, b, c]
	got := cm.GetRotatedModels(models, "rr", StrategyRoundRobin, 1)
	if got[0] != "a" {
		t.Errorf("First rotation should start at index 0, got %v", got)
	}

	// Second call: index 1 → [b, c, a]
	got = cm.GetRotatedModels(models, "rr", StrategyRoundRobin, 1)
	if got[0] != "b" {
		t.Errorf("Second rotation should start at index 1, got %v", got)
	}

	// Third call: index 2 → [c, a, b]
	got = cm.GetRotatedModels(models, "rr", StrategyRoundRobin, 1)
	if got[0] != "c" {
		t.Errorf("Third rotation should start at index 2, got %v", got)
	}

	// Fourth call: wraps to index 0 → [a, b, c]
	got = cm.GetRotatedModels(models, "rr", StrategyRoundRobin, 1)
	if got[0] != "a" {
		t.Errorf("Fourth rotation should wrap to index 0, got %v", got)
	}
}

func TestComboManager_GetRotatedModels_StickyLimit(t *testing.T) {
	cm := NewComboManager()
	models := []string{"a", "b", "c"}

	// Sticky limit 2: first two calls should use index 0
	got := cm.GetRotatedModels(models, "sticky", StrategyRoundRobin, 2)
	if got[0] != "a" {
		t.Errorf("Sticky call 1 should be index 0, got %v", got)
	}

	got = cm.GetRotatedModels(models, "sticky", StrategyRoundRobin, 2)
	if got[0] != "a" {
		t.Errorf("Sticky call 2 should still be index 0, got %v", got)
	}

	// Third call: should advance to index 1
	got = cm.GetRotatedModels(models, "sticky", StrategyRoundRobin, 2)
	if got[0] != "b" {
		t.Errorf("Sticky call 3 should advance to index 1, got %v", got)
	}
}

func TestComboManager_ResetRotation(t *testing.T) {
	cm := NewComboManager()
	models := []string{"a", "b", "c"}

	// Advance rotation
	cm.GetRotatedModels(models, "reset-test", StrategyRoundRobin, 1)
	cm.GetRotatedModels(models, "reset-test", StrategyRoundRobin, 1)

	// Reset
	cm.ResetRotation("reset-test")

	// Should be back to index 0
	got := cm.GetRotatedModels(models, "reset-test", StrategyRoundRobin, 1)
	if got[0] != "a" {
		t.Errorf("After reset, should start at index 0, got %v", got)
	}
}

func TestComboManager_SingleModel(t *testing.T) {
	cm := NewComboManager()
	models := []string{"only-one"}

	got := cm.GetRotatedModels(models, "single", StrategyRoundRobin, 1)
	if len(got) != 1 || got[0] != "only-one" {
		t.Errorf("Single model should return unchanged, got %v", got)
	}
}

func TestRotateFromIndex(t *testing.T) {
	models := []string{"a", "b", "c", "d"}

	tests := []struct {
		index    int
		expected []string
	}{
		{0, []string{"a", "b", "c", "d"}},
		{1, []string{"b", "c", "d", "a"}},
		{2, []string{"c", "d", "a", "b"}},
		{3, []string{"d", "a", "b", "c"}},
	}

	for _, tt := range tests {
		got := rotateFromIndex(models, tt.index)
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("rotateFromIndex(models, %d) = %v, want %v", tt.index, got, tt.expected)
				break
			}
		}
	}
}

func TestHandleComboFallback_FirstSucceeds(t *testing.T) {
	models := []string{"a", "b", "c"}
	callCount := 0

	result := HandleComboFallback(models, func(modelStr string) ComboResult {
		callCount++
		return ComboResult{Success: true, Status: 200}
	}, nil)

	if !result.Success {
		t.Error("Expected success")
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestHandleComboFallback_SecondSucceeds(t *testing.T) {
	models := []string{"a", "b", "c"}
	callCount := 0

	result := HandleComboFallback(models, func(modelStr string) ComboResult {
		callCount++
		if callCount == 1 {
			return ComboResult{Success: false, Status: 503, ErrorText: "overloaded"}
		}
		return ComboResult{Success: true, Status: 200}
	}, nil)

	if !result.Success {
		t.Error("Expected success on second model")
	}
	if callCount != 2 {
		t.Errorf("Expected 2 calls, got %d", callCount)
	}
}

func TestHandleComboFallback_AllFail(t *testing.T) {
	models := []string{"a", "b"}

	result := HandleComboFallback(models, func(modelStr string) ComboResult {
		return ComboResult{Success: false, Status: 429, ErrorText: "rate limit exceeded"}
	}, nil)

	if result.Success {
		t.Error("Expected failure")
	}
	if result.Status != 429 {
		t.Errorf("Expected status 429, got %d", result.Status)
	}
}
