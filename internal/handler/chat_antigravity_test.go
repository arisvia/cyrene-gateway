package handler

import (
	"testing"
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
