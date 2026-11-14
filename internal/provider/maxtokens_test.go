package provider

import (
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// The advertised output limit wins over the rule's number: it is per-model and moves
// when upstream moves it, while the literal only stands in for a cold catalog.
func TestClampMaxTokensPrefersAdvertisedLimit(t *testing.T) {
	prev := model.GetModelsDevCatalog()
	model.SetModelsDevCatalog(map[string]model.ModelsDevEntry{
		"nvidia/deepseek-r1": {Name: "DeepSeek R1", Output: 2048},
		"kimi-k2":            {Name: "Kimi K2", Output: 65536},
	})
	defer model.SetModelsDevCatalog(prev)

	body := map[string]any{"max_tokens": float64(8192)}
	if !ClampMaxTokens("nvidia", "deepseek-r1", body) {
		t.Fatal("advertised 2048 must cap a request of 8192")
	}
	if got := body["max_tokens"]; got != float64(2048) {
		t.Errorf("max_tokens = %v, want 2048", got)
	}

	// An advertised limit above the deployment cap does not lift the cap: the endpoint
	// refuses the larger value whatever the model can theoretically emit.
	body = map[string]any{"max_tokens": float64(131072)}
	if !ClampMaxTokens("volcengine-ark", "kimi-k2", body) {
		t.Fatal("ark kimi must still clamp")
	}
	if got := body["max_tokens"]; got != float64(32768) {
		t.Errorf("max_tokens = %v, want the 32768 deployment cap", got)
	}
}
