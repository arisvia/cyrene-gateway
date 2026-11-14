package provider

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

// ParseModel splits "provider/model" into provider and model parts.
// If no slash is present, provider is empty and will be inferred.
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
		case "low", "medium", "high", "xhigh", "max", "adaptive", "none", "off":
			clean = clean[:idx]
			effort = cand
		}
	}
	clean, _ = StripModelContextMarker(clean)
	return clean, effort
}

// ParseModel splits "provider/model" into provider and model parts.
// If no slash is present, provider is empty and will be inferred.
func ParseModel(modelStr string) model.ModelInfo {
	cleanModel, _ := ExtractModelReasoningEffort(modelStr)
	if before, after, ok := strings.Cut(cleanModel, "/"); ok {
		return model.ModelInfo{Provider: before, Model: after}
	}
	// Bare model name - the provider still has to be inferred.
	return model.ModelInfo{Provider: "", Model: cleanModel}
}

// ResolveModel resolves a model string to full ModelInfo using:
//  1. Explicit provider prefix (e.g. "openai/gpt-4o")
//  2. Dynamic search in cached models of active connections
//  3. Fallback: prefix-based inference, preferring the credential form (API key
//     vs OAuth) that actually has a connection, then the sole connected provider
//     when the name matches nothing
//  4. An explicit error when a bare name cannot be attributed, rather than a guess
func ResolveModel(modelStr string, database *db.DB) (model.ModelInfo, error) {
	parsed := ParseModel(modelStr)

	// 1. Explicit provider already present (only if it is a known provider or has configured connections)
	if parsed.Provider != "" {
		if _, ok := GetProvider(parsed.Provider); ok {
			return parsed, nil
		}
		if database != nil {
			if conns, err := database.ListConnectionsByProvider(parsed.Provider); err == nil && len(conns) > 0 {
				return parsed, nil
			}
		}
	}

	if database != nil {
		// 2. Dynamic lookup across cached models of active providers
		caches, err := database.KVList(model.KVScopeProviderModelCache)
		if err != nil {
			slog.Warn("Failed to list provider model cache from DB", slog.String("error", err.Error()))
		} else if len(caches) > 0 {
			lowerFull := strings.ToLower(modelStr)
			lowerModel := strings.ToLower(parsed.Model)

			activeProviders := make(map[string]bool)
			if conns, err := database.ListConnections(); err == nil && len(conns) > 0 {
				for _, c := range conns {
					if c.IsActive {
						activeProviders[c.Provider] = true
					}
				}
			}

			var fallbackMatch *model.ModelInfo
			for providerID, raw := range caches {
				var cached model.CachedModels
				if err := json.Unmarshal([]byte(raw), &cached); err != nil {
					continue
				}
				for _, m := range cached.Models {
					if strings.EqualFold(m.ID, lowerFull) || (m.DisplayName != "" && strings.EqualFold(m.DisplayName, lowerFull)) ||
						strings.EqualFold(m.ID, lowerModel) || (m.DisplayName != "" && strings.EqualFold(m.DisplayName, lowerModel)) {
						match := model.ModelInfo{
							Provider: providerID,
							Model:    m.ID,
						}
						if activeProviders[providerID] {
							return match, nil
						}
						if fallbackMatch == nil && len(activeProviders) == 0 {
							fallbackMatch = &match
						}
					}
				}
			}
			if fallbackMatch != nil {
				return *fallbackMatch, nil
			}
		}
	}

	// 4. If an explicit prefix was provided, keep it even if not currently known
	if parsed.Provider != "" {
		return parsed, nil
	}

	// 5. No catalog knows the model: fall back to what its name says, preferring the
	//    credential form (API key vs OAuth) that is actually connected.
	inferred := activeAuthFormProvider(InferProviderFromModel(parsed.Model), database)
	if inferred == "" {
		// A single-provider gateway has one honest answer left: the only provider
		// with credentials is the one the caller meant. With several connected the
		// name has to carry the provider, because guessing used to route unknown ids
		// to OpenAI and report its missing credentials instead of the real problem.
		if sole, ok := soleActiveProvider(database); ok {
			inferred = sole
		}
	}
	if inferred == "" {
		return model.ModelInfo{}, fmt.Errorf("cannot resolve provider for model %q: no connected provider advertises it, address it as provider/model", modelStr)
	}
	return model.ModelInfo{Provider: inferred, Model: parsed.Model}, nil
}

// soleActiveProvider returns the one provider with an active connection, and false
// when none or several qualify. Cold catalogs and brand-new upstream ids are what it
// covers: with a single provider connected there is nothing else the name can mean.
func soleActiveProvider(database *db.DB) (string, bool) {
	if database == nil {
		return "", false
	}
	conns, err := database.ListConnections()
	if err != nil {
		return "", false
	}
	found := ""
	for _, c := range conns {
		if !c.IsActive {
			continue
		}
		if found != "" && found != c.Provider {
			return "", false
		}
		found = c.Provider
	}
	return found, found != ""
}

// anthropicOAuthProvider is the provider id a Claude Code OAuth login connects
// under. The prefix table maps bare claude-* names to "anthropic" (Console API
// key form), so an operator who authorized only the OAuth account — and whose
// synced catalog does not happen to list that id — was told there were no
// credentials for a model their own account serves. openai carries both forms
// under one id (api-key plus Codex OAuth) and needs no sibling.
const anthropicOAuthProvider = "claude"

// activeAuthFormProvider swaps an inferred API-key provider for its OAuth twin
// when the inferred one has no active connection and the twin does.
func activeAuthFormProvider(inferred string, database *db.DB) string {
	if database == nil || inferred != "anthropic" {
		return inferred
	}
	if hasActiveConnection(database, inferred) || !hasActiveConnection(database, anthropicOAuthProvider) {
		return inferred
	}
	return anthropicOAuthProvider
}

func hasActiveConnection(database *db.DB, providerID string) bool {
	conns, err := database.ListConnectionsByProvider(providerID)
	if err != nil {
		return false
	}
	for _, c := range conns {
		if c.IsActive {
			return true
		}
	}
	return false
}

// ResolveCombo checks if a bare model string matches a combo name.
// Returns the combo and true if found, nil and false otherwise.
// Combos only apply to bare model names (no "/" separator).
func ResolveCombo(modelStr string, database *db.DB) (*model.Combo, bool) {
	if database == nil {
		return nil, false
	}
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
// It checks both the full identifier (e.g. "opencode/big-pickle") and bare identifier (e.g. "big-pickle"),
// and resolves a shorthand prefix so callers can pass whatever the client typed.
func IsModelDisabled(modelStr string, database *db.DB) bool {
	if database == nil {
		return false
	}
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
	}
	return false
}
