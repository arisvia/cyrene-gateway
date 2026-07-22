package provider

import (
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

// ParseModel parses a model string into provider and model name.
// Supports formats: "provider/model", "alias/model", or bare "model" (alias lookup).
func ParseModel(modelStr string) model.ModelInfo {
	if modelStr == "" {
		return model.ModelInfo{}
	}

	// Standard format: provider/model or alias/model
	if idx := strings.Index(modelStr, "/"); idx > 0 {
		providerOrAlias := modelStr[:idx]
		modelName := modelStr[idx+1:]
		provider := ResolveProviderAlias(providerOrAlias)
		return model.ModelInfo{Provider: provider, Model: modelName}
	}

	// Bare model name - will need alias resolution or inference
	return model.ModelInfo{Provider: "", Model: modelStr}
}

// ResolveModel resolves a model string to full ModelInfo using DB aliases and inference.
func ResolveModel(modelStr string, database *db.DB) (model.ModelInfo, error) {
	parsed := ParseModel(modelStr)

	// Already has provider
	if parsed.Provider != "" {
		return parsed, nil
	}

	// Try model alias from KV store (scope="aliases")
	aliases, err := database.KVList("aliases")
	if err == nil {
		if target, ok := aliases[parsed.Model]; ok {
			resolved := ParseModel(target)
			if resolved.Provider != "" {
				return resolved, nil
			}
		}
	}

	// Fallback: infer provider from model name
	return model.ModelInfo{
		Provider: InferProviderFromModel(parsed.Model),
		Model:    parsed.Model,
	}, nil
}

// ResolveCombo checks if a bare model string matches a combo name.
// Returns the combo and true if found, nil and false otherwise.
// Combos only apply to bare model names (no "/" separator).
func ResolveCombo(modelStr string, database *db.DB) (*model.Combo, bool) {
	// Don't check combos for provider/model format
	if strings.Contains(modelStr, "/") {
		return nil, false
	}

	combo, err := database.GetComboByName(modelStr)
	if err != nil || combo == nil || len(combo.Models) == 0 {
		return nil, false
	}
	return combo, true
}

// IsModelDisabled checks if a model is in the disabled models list.
func IsModelDisabled(modelStr string, database *db.DB) bool {
	disabled, err := database.KVList("disabledModels")
	if err != nil {
		return false
	}
	_, ok := disabled[modelStr]
	return ok
}
