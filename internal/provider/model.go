package provider

import (
	"encoding/json"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

// ParseModel splits "provider/model" into provider and model parts.
// If no slash is present, provider is empty and will be inferred or resolved via alias.
func ParseModel(modelStr string) model.ModelInfo {
	if before, after, ok := strings.Cut(modelStr, "/"); ok {
		rawProv := before
		canonicalProv := ResolveProviderAlias(rawProv)
		return model.ModelInfo{
			Provider: canonicalProv,
			Model:    after,
		}
	}
	// Bare model name - will need alias resolution or inference
	return model.ModelInfo{Provider: "", Model: modelStr}
}

// ResolveModel resolves a model string to full ModelInfo using:
// 1. Explicit provider prefix (e.g. "openai/gpt-4o")
// 2. Alias resolution from KV store
// 3. Dynamic search in cached models of active connections
// 4. Fallback: prefix-based inference
func ResolveModel(modelStr string, database *db.DB) (model.ModelInfo, error) {
	parsed := ParseModel(modelStr)

	// 1. Explicit provider already present
	if parsed.Provider != "" {
		return parsed, nil
	}

	// 2. Try model alias from KV store (scope="aliases")
	aliases, err := database.KVList(model.KVScopeAliases)
	if err == nil {
		if target, ok := aliases[parsed.Model]; ok {
			resolved := ParseModel(target)
			if resolved.Provider != "" {
				return resolved, nil
			}
		}
	}

	// 3. Dynamic lookup across cached models of active providers
	if caches, err := database.KVList(model.KVScopeProviderModelCache); err == nil && len(caches) > 0 {
		lowerTarget := strings.ToLower(parsed.Model)
		for providerID, raw := range caches {
			var cached model.CachedModels
			if err := json.Unmarshal([]byte(raw), &cached); err != nil {
				continue
			}
			for _, m := range cached.Models {
				if strings.EqualFold(m.ID, lowerTarget) || (m.DisplayName != "" && strings.EqualFold(m.DisplayName, lowerTarget)) {
					return model.ModelInfo{
						Provider: providerID,
						Model:    m.ID,
					}, nil
				}
			}
		}
	}

	// 4. Dynamic lookup across active connections (e.g. if custom node or openrouter)
	if conns, err := database.ListConnections(); err == nil {
		for _, conn := range conns {
			if !conn.IsActive {
				continue
			}
			if regModels, ok := RegistryModels[conn.Provider]; ok {
				for _, rm := range regModels {
					if strings.EqualFold(rm.ID, parsed.Model) || strings.EqualFold(rm.Name, parsed.Model) {
						return model.ModelInfo{
							Provider: conn.Provider,
							Model:    rm.ID,
						}, nil
					}
				}
			}
		}
	}

	// 5. Fallback: infer provider from model name
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
	disabled, err := database.KVList(model.KVScopeDisabledModels)
	if err != nil {
		return false
	}
	_, ok := disabled[modelStr]
	return ok
}
