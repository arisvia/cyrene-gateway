package provider

import "strings"

// MaxTokensRule defines a max_tokens clamping rule for specific providers/models.
type MaxTokensRule struct {
	Provider     string // empty = any provider
	ModelMatch   string // substring match on model name (case-insensitive)
	MaxOutputCap int    // hard ceiling for max_tokens
}

// MaxTokensRules are checked in order; first match wins.
// Mirrors 9router paramSupport.js STRIP_RULES with maxOutputCap.
var MaxTokensRules = []MaxTokensRule{
	// NVIDIA NIM caps at 4096 for most models
	{Provider: "nvidia", ModelMatch: "", MaxOutputCap: 4096},
	// VolcEngine Ark caps Kimi family at 32768
	{Provider: "volcengine-ark", ModelMatch: "kimi", MaxOutputCap: 32768},
}

// ClampMaxTokens applies provider-specific max_tokens clamping to the request body.
// Modifies body in place. Returns true if clamping was applied.
func ClampMaxTokens(providerID string, modelName string, body map[string]any) bool {
	for _, rule := range MaxTokensRules {
		if rule.Provider != "" && rule.Provider != providerID {
			continue
		}
		if rule.ModelMatch != "" && !strings.Contains(strings.ToLower(modelName), rule.ModelMatch) {
			continue
		}
		if rule.MaxOutputCap <= 0 {
			continue
		}

		clamped := false
		for _, key := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
			if v, ok := body[key]; ok {
				switch n := v.(type) {
				case float64:
					if int(n) > rule.MaxOutputCap {
						body[key] = float64(rule.MaxOutputCap)
						clamped = true
					}
				case int:
					if n > rule.MaxOutputCap {
						body[key] = rule.MaxOutputCap
						clamped = true
					}
				}
			}
		}
		return clamped
	}
	return false
}
