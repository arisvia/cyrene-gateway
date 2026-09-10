package handler

import (
	"encoding/json"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestAntigravityThinkingTierRouting(t *testing.T) {
	cases := []struct {
		model          string
		effort         string
		expectedTarget string
		expectedTier   string
	}{
		// 1. gemini-3.8-flash default medium
		{
			model:          "antigravity/gemini-3.8-flash",
			effort:         "",
			expectedTarget: "gemini-3.8-flash-medium",
			expectedTier:   "medium",
		},
		// 2. gemini-3.8-flash explicit high
		{
			model:          "ag/gemini-3.8-flash",
			effort:         "high",
			expectedTarget: "gemini-3.8-flash-high",
			expectedTier:   "high",
		},
		// 3. gemini-3.8-flash explicit low
		{
			model:          "gemini-3.8-flash",
			effort:         "low",
			expectedTarget: "gemini-3.8-flash-low",
			expectedTier:   "low",
		},
		// 4. gemini-3.5-flash high
		{
			model:          "antigravity/gemini-3.5-flash",
			effort:         "high",
			expectedTarget: "gemini-3.5-flash-high",
			expectedTier:   "high",
		},
		// 5. gemini-3.5-flash low -> extra-low
		{
			model:          "antigravity/gemini-3.5-flash",
			effort:         "low",
			expectedTarget: "gemini-3.5-flash-extra-low",
			expectedTier:   "low",
		},
		// 6. explicit model suffix preservation
		{
			model:          "gemini-3.8-flash-high",
			effort:         "low", // model suffix takes precedence
			expectedTarget: "gemini-3.8-flash-high",
			expectedTier:   "high",
		},
	}

	for _, c := range cases {
		t.Run(c.model+"_"+c.effort, func(t *testing.T) {
			targetModel, tier := resolveAntigravityModelTier(c.model, c.effort)
			if targetModel != c.expectedTarget {
				t.Errorf("model=%s effort=%s: expected target %s, got %s", c.model, c.effort, c.expectedTarget, targetModel)
			}
			if tier != c.expectedTier {
				t.Errorf("model=%s effort=%s: expected tier %s, got %s", c.model, c.effort, c.expectedTier, tier)
			}
		})
	}
}

func TestAntigravityAdaptiveModelResolution(t *testing.T) {
	mockCatalog := []model.ModelMetadata{
		{ID: "gemini-3.8-flash-high", Family: "gemini", Capabilities: []string{"chat", "code", "reasoning"}},
		{ID: "gemini-3.8-flash-medium", Family: "gemini", Capabilities: []string{"chat", "code", "reasoning"}},
		{ID: "gemini-3.8-flash-low", Family: "gemini", Capabilities: []string{"chat", "code", "reasoning"}},
		{ID: "gemini-4.0-flash-high", Family: "gemini", Capabilities: []string{"chat", "code", "reasoning"}},
		{ID: "claude-sonnet-4-6", Family: "claude", Capabilities: []string{"chat", "code"}},
		{ID: "gpt-oss-120b-medium", Family: "gpt-oss", Capabilities: []string{"chat", "code"}},
		{ID: "gemini-3.1-flash-image", Family: "gemini", Capabilities: []string{"image-generation"}},
	}

	// 1. Exact match for Claude: thinking MUST NOT be injected
	target, _, injectThinking, isImg := resolveAntigravityModel("claude-sonnet-4-6", "high", mockCatalog)
	if target != "claude-sonnet-4-6" {
		t.Errorf("expected claude-sonnet-4-6, got %s", target)
	}
	if injectThinking {
		t.Errorf("claude-sonnet-4-6 must never inject thinkingConfig")
	}
	if isImg {
		t.Errorf("claude-sonnet-4-6 is not an image model")
	}

	// 2. Exact match for GPT-OSS: thinking MUST NOT be injected
	target, _, injectThinking, _ = resolveAntigravityModel("gpt-oss-120b-medium", "medium", mockCatalog)
	if target != "gpt-oss-120b-medium" {
		t.Errorf("expected gpt-oss-120b-medium, got %s", target)
	}
	if injectThinking {
		t.Errorf("gpt-oss-120b-medium must never inject thinkingConfig")
	}

	// 3. Dynamic effort probing for new future model (gemini-4.0-flash + high)
	target, tier, injectThinking, _ := resolveAntigravityModel("gemini-4.0-flash", "high", mockCatalog)
	if target != "gemini-4.0-flash-high" {
		t.Errorf("expected gemini-4.0-flash-high via dynamic probe, got %s", target)
	}
	if tier != "high" {
		t.Errorf("expected tier high, got %s", tier)
	}
	if !injectThinking {
		t.Errorf("gemini-4.0-flash-high with reasoning capability should inject thinkingConfig")
	}

	// 4. Image model detection
	target, _, injectThinking, isImg = resolveAntigravityModel("gemini-3.1-flash-image", "", mockCatalog)
	if !isImg {
		t.Errorf("expected isImage=true for gemini-3.1-flash-image")
	}
	if injectThinking {
		t.Errorf("image model must not inject thinking")
	}
}

func TestAntigravityConvertOpenAIToGeminiContents(t *testing.T) {
	messages := []Message{
		{
			Role:    "system",
			Content: json.RawMessage(`"You are a helpful assistant"`),
		},
		{
			Role:    "user",
			Content: json.RawMessage(`"What is the weather?"`),
		},
		{
			Role:    "assistant",
			Content: json.RawMessage(`""`),
			ToolCalls: json.RawMessage(`[
				{
					"id": "call_123",
					"type": "function",
					"function": {
						"name": "get_weather",
						"arguments": "{\"location\":\"Beijing\"}"
					}
				}
			]`),
		},
		{
			Role:    "tool",
			Content: json.RawMessage(`"Sunny, 25C"`),
		},
	}

	contents, systemParts := convertOpenAIToGeminiContents(messages)

	if len(systemParts) != 1 || systemParts[0]["text"] != "You are a helpful assistant" {
		t.Fatalf("unexpected systemParts: %+v", systemParts)
	}
	if len(contents) != 3 {
		t.Fatalf("expected 3 contents, got %d", len(contents))
	}

	// Check assistant message has functionCall with defaultThinkingAgSignature
	modelContent := contents[1]
	if modelContent["role"] != "model" {
		t.Errorf("expected role model, got %s", modelContent["role"])
	}
	parts, ok := modelContent["parts"].([]map[string]any)
	if !ok || len(parts) < 2 {
		t.Fatalf("expected at least 2 parts (text + functionCall), got %+v", modelContent["parts"])
	}
	fcPart := parts[1]
	if fcPart["thoughtSignature"] != defaultThinkingAgSignature {
		t.Errorf("expected thoughtSignature %q, got %v", defaultThinkingAgSignature, fcPart["thoughtSignature"])
	}
	fc, ok := fcPart["functionCall"].(map[string]any)
	if !ok || fc["name"] != "get_weather" {
		t.Errorf("expected functionCall get_weather, got %+v", fc)
	}
}
