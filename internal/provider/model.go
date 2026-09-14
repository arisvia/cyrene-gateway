package provider

import (
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

// ParseModel splits "provider/model" into provider and model parts.
// If no slash is present, provider is empty and will be inferred or resolved via alias.
// StripModelContextMarker removes trailing context window markers such as "[1m]" or "[1M]"
// appended by clients like Claude Code with the 1M-context beta enabled.
func StripModelContextMarker(modelStr string) (string, string) {
	trimmed := strings.TrimSpace(modelStr)
	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, "[1m]") {
		return trimmed[:len(trimmed)-4], "1m"
	}
	return modelStr, ""
}

// ExtractModelReasoningEffort parses and strips reasoning effort suffixes like "(low)", "(medium)", "(high)", "(adaptive)", "(none)"
// as well as context markers like "[1m]" from model strings,
// e.g. "claude-3-7-sonnet(high)" -> ("claude-3-7-sonnet", "high")
// and "claude-opus-5[1m]" -> ("claude-opus-5", "").
func ExtractModelReasoningEffort(modelStr string) (string, string) {
	clean, _ := StripModelContextMarker(modelStr)
	effort := ""
	if idx := strings.LastIndex(clean, "("); idx != -1 && strings.HasSuffix(clean, ")") {
		cand := strings.ToLower(clean[idx+1 : len(clean)-1])
		switch cand {
		case "low", "medium", "high", "max", "adaptive", "none", "off":
			clean = clean[:idx]
			effort = cand
		}
	}
	clean, _ = StripModelContextMarker(clean)
	return clean, effort
}

// ParseModel splits "provider/model" into provider and model parts.
// If no slash is present, provider is empty and will be inferred or resolved via alias.
func ParseModel(modelStr string) model.ModelInfo {
	cleanModel, _ := ExtractModelReasoningEffort(modelStr)
	if before, after, ok := strings.Cut(cleanModel, "/"); ok {
		rawProv := before
		canonicalProv := ResolveProviderAlias(rawProv)
		return model.ModelInfo{
			Provider: canonicalProv,
			Model:    after,
		}
	}
	// Bare model name - will need alias resolution or inference
	return model.ModelInfo{Provider: "", Model: cleanModel}
}

// ResolveModel resolves a model string to full ModelInfo using:
// 1. Explicit provider prefix (e.g. "openai/gpt-4o")
// 2. Alias resolution from KV store
// 3. Dynamic search in cached models of active connections
// 4. Fallback: prefix-based inference
func ResolveModel(modelStr string, database *db.DB) (model.ModelInfo, error) {
	parsed := ParseModel(modelStr)

	// 1. Explicit provider already present (only if it is a known provider or has configured connections)
	if parsed.Provider != "" {
		if _, ok := GetProvider(parsed.Provider); ok {
			return parsed, nil
		}
		if conns, err := database.ListConnectionsByProvider(parsed.Provider); err == nil && len(conns) > 0 {
			return parsed, nil
		}
	}

	// 2. Try model alias from KV store (scope="aliases")
	aliases, err := database.KVList(model.KVScopeAliases)
	if err != nil {
		slog.Warn("Failed to list model aliases from DB", slog.String("error", err.Error()))
	} else {
		if target, ok := aliases[modelStr]; ok {
			resolved := ParseModel(target)
			if resolved.Provider != "" {
				return resolved, nil
			}
		}
		if target, ok := aliases[parsed.Model]; ok {
			resolved := ParseModel(target)
			if resolved.Provider != "" {
				return resolved, nil
			}
		}
	}

	// 3. Dynamic lookup across cached models of active providers
	caches, err := database.KVList(model.KVScopeProviderModelCache)
	if err != nil {
		slog.Warn("Failed to list provider model cache from DB", slog.String("error", err.Error()))
	} else if len(caches) > 0 {
		lowerFull := strings.ToLower(modelStr)
		lowerModel := strings.ToLower(parsed.Model)
		for providerID, raw := range caches {
			var cached model.CachedModels
			if err := json.Unmarshal([]byte(raw), &cached); err != nil {
				continue
			}
			for _, m := range cached.Models {
				if strings.EqualFold(m.ID, lowerFull) || (m.DisplayName != "" && strings.EqualFold(m.DisplayName, lowerFull)) ||
					strings.EqualFold(m.ID, lowerModel) || (m.DisplayName != "" && strings.EqualFold(m.DisplayName, lowerModel)) {
					return model.ModelInfo{
						Provider: providerID,
						Model:    m.ID,
					}, nil
				}
			}
		}
	}

	// 4. If an explicit prefix was provided, keep it even if not currently known
	if parsed.Provider != "" {
		return parsed, nil
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
// It checks both the full identifier (e.g. "opencode/big-pickle") and bare identifier (e.g. "big-pickle").
func IsModelDisabled(modelStr string, database *db.DB) bool {
	disabled, err := database.KVList(model.KVScopeDisabledModels)
	if err != nil || len(disabled) == 0 {
		return false
	}
	if _, ok := disabled[modelStr]; ok {
		return true
	}
	// If modelStr is "provider/model", check bare model as well
	if idx := strings.Index(modelStr, "/"); idx != -1 {
		bare := modelStr[idx+1:]
		if _, ok := disabled[bare]; ok {
			return true
		}
	} else {
		// If modelStr is bare, check if any "*/bare" entry is disabled
		suffix := "/" + modelStr
		for k := range disabled {
			if strings.HasSuffix(k, suffix) {
				return true
			}
		}
	}
	return false
}
