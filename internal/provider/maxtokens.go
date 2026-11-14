package provider

import (
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// deploymentCaps lists endpoints that refuse a max_tokens above a fixed size. The
// number is the cold-cache fallback, not the authority: modelsDevOutputLimit reads the
// output limit the model itself advertises and tightens the cap when it is smaller.
// Mirrors 9router paramSupport.js STRIP_RULES.
var deploymentCaps = []struct {
	provider    string
	modelMatch  string // substring on the model id; "" = any model on that provider
	fallbackCap int
}{
	// NVIDIA NIM serves third-party weights behind a fixed deployment ceiling that the
	// hosted model card does not always state.
	{provider: "nvidia", fallbackCap: 4096},
	// VolcEngine Ark serves the Kimi family with its own output ceiling.
	{provider: "volcengine-ark", modelMatch: "kimi", fallbackCap: 32768},
}

// ClampMaxTokens caps the requested output length at what this endpoint can return,
// modifying body in place. Returns true when a value was lowered.
func ClampMaxTokens(providerID string, modelName string, body map[string]any) bool {
	limit, ok := clampOutputCap(providerID, modelName)
	if !ok {
		return false
	}

	clamped := false
	for _, key := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
		v, exists := body[key]
		if !exists {
			continue
		}
		switch n := v.(type) {
		case float64:
			if int(n) > limit {
				body[key] = float64(limit)
				clamped = true
			}
		case int:
			if n > limit {
				body[key] = limit
				clamped = true
			}
		}
	}
	return clamped
}

// clampOutputCap resolves the ceiling for a request: the endpoint's known refusal
// point, tightened by the limit the model advertises when that is smaller. Providers
// with no documented refusal are left alone — an advertised limit only ever guards a
// request we already know upstream would reject, and never silently shortens an
// answer the endpoint would have produced.
func clampOutputCap(providerID, modelName string) (int, bool) {
	lowerModel := strings.ToLower(modelName)
	for _, rule := range deploymentCaps {
		if rule.provider != "" && rule.provider != providerID {
			continue
		}
		if rule.modelMatch != "" && !strings.Contains(lowerModel, rule.modelMatch) {
			continue
		}
		limit := rule.fallbackCap
		// The advertised limit only lowers the cap. It cannot raise it: a catalogue
		// entry describing what a model can emit says nothing about what this
		// deployment accepts, and sending the larger number is the refusal we are here
		// to prevent.
		if advertised := model.ModelsDevOutputLimit(providerID, modelName); advertised > 0 && advertised < limit {
			limit = advertised
		}
		if limit > 0 {
			return limit, true
		}
		return 0, false
	}
	return 0, false
}
