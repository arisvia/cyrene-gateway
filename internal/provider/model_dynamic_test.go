package provider

import (
	"encoding/json"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestResolveModelDynamicCache(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	// Simulate cached model from dynamic upstream
	cached := model.CachedModels{
		Models: []model.ModelMetadata{
			{
				ID:          "deepseek-reasoner-v3",
				DisplayName: "DeepSeek R1 Advanced",
			},
		},
	}
	rawBytes, _ := json.Marshal(cached)
	database.KVSet("providerModelCache", "deepseek", string(rawBytes))

	// Resolve by ID without provider prefix
	info, err := ResolveModel("deepseek-reasoner-v3", database)
	if err != nil {
		t.Fatalf("failed to resolve model: %v", err)
	}
	if info.Provider != "deepseek" {
		t.Errorf("expected provider=deepseek, got %s", info.Provider)
	}
	if info.Model != "deepseek-reasoner-v3" {
		t.Errorf("expected model=deepseek-reasoner-v3, got %s", info.Model)
	}

	// Resolve by DisplayName
	info, err = ResolveModel("DeepSeek R1 Advanced", database)
	if err != nil {
		t.Fatalf("failed to resolve model by display name: %v", err)
	}
	if info.Provider != "deepseek" {
		t.Errorf("expected provider=deepseek, got %s", info.Provider)
	}
}

func TestResolveModel_NamespacedVendorModel(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	// Simulate OpenRouter cache containing namespaced models like liquid/lfm-2.5-2.6b:free
	cached := model.CachedModels{
		Models: []model.ModelMetadata{
			{
				ID:          "liquid/lfm-2.5-2.6b:free",
				DisplayName: "Liquid LFM 2.5 2.6B (free)",
			},
			{
				ID:          "meta-llama/llama-3.3-70b-instruct:free",
				DisplayName: "Meta Llama 3.3 70B Instruct (free)",
			},
		},
	}
	rawBytes, _ := json.Marshal(cached)
	database.KVSet("providerModelCache", "openrouter", string(rawBytes))

	// 1. Resolve without openrouter/ prefix
	info, err := ResolveModel("liquid/lfm-2.5-2.6b:free", database)
	if err != nil {
		t.Fatalf("failed to resolve namespaced model: %v", err)
	}
	if info.Provider != "openrouter" {
		t.Errorf("expected provider=openrouter, got %s", info.Provider)
	}
	if info.Model != "liquid/lfm-2.5-2.6b:free" {
		t.Errorf("expected model=liquid/lfm-2.5-2.6b:free, got %s", info.Model)
	}

	// 2. Resolve with explicit openrouter/ prefix
	infoExplicit, err := ResolveModel("openrouter/liquid/lfm-2.5-2.6b:free", database)
	if err != nil {
		t.Fatalf("failed to resolve explicit namespaced model: %v", err)
	}
	if infoExplicit.Provider != "openrouter" {
		t.Errorf("expected provider=openrouter, got %s", infoExplicit.Provider)
	}
	if infoExplicit.Model != "liquid/lfm-2.5-2.6b:free" {
		t.Errorf("expected model=liquid/lfm-2.5-2.6b:free, got %s", infoExplicit.Model)
	}
}

func TestExtractModelReasoningEffort(t *testing.T) {
	tests := []struct {
		input       string
		wantModel   string
		wantEffort  string
	}{
		{"claude-3-7-sonnet(high)", "claude-3-7-sonnet", "high"},
		{"antigravity/claude-3-7-sonnet(low)", "antigravity/claude-3-7-sonnet", "low"},
		{"gpt-4o(medium)", "gpt-4o", "medium"},
		{"gemini-2.5-flash(adaptive)", "gemini-2.5-flash", "adaptive"},
		{"claude-opus-5[1m]", "claude-opus-5", ""},
		{"claude-opus-5[1M]", "claude-opus-5", ""},
		{"claude-3-7-sonnet[1m](high)", "claude-3-7-sonnet", "high"},
		{"claude-3-7-sonnet(high)[1m]", "claude-3-7-sonnet", "high"},
		{"cc/claude-sonnet-4.5[1m]", "cc/claude-sonnet-4.5", ""},
		{"deepseek-r1", "deepseek-r1", ""},
		{"custom-model(unknown)", "custom-model(unknown)", ""},
	}
	for _, tt := range tests {
		gotModel, gotEffort := ExtractModelReasoningEffort(tt.input)
		if gotModel != tt.wantModel || gotEffort != tt.wantEffort {
			t.Errorf("ExtractModelReasoningEffort(%q) = (%q, %q), want (%q, %q)",
				tt.input, gotModel, gotEffort, tt.wantModel, tt.wantEffort)
		}
	}
}
