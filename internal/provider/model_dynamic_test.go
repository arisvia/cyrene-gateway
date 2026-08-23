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
