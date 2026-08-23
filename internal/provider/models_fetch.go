package provider

import (
	"net/http"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// ModelsFetchFor resolves the live model-catalog fetch config for a provider
// (Phase 36 T6). Explicit registry fields win; otherwise the scheme follows the
// wire format (anthropic → x-api-key raw, gemini → ?key= query, else bearer).
func ModelsFetchFor(p ProviderInfo) model.ModelsFetchConfig {
	cfg := model.ModelsFetchConfig{
		URL:  p.ModelsURL,
		Auth: p.ModelsAuth,
	}
	if cfg.Auth == "" {
		switch p.APIType {
		case "anthropic":
			cfg.Auth = "raw"
			cfg.AuthHeader = "x-api-key"
		case "gemini":
			cfg.Auth = "query"
		default:
			cfg.Auth = "bearer"
		}
	}
	return cfg
}

// QoderCatalogModels converts Qoder's live COSY-signed model catalog into
// []model.ModelMetadata (9router services/qoderModels.js parity). The catalog
// is fetched on demand with the given COSY creds (callers resolve PATs first).
func QoderCatalogModels(creds QoderCosyCreds, client *http.Client, force bool) []model.ModelMetadata {
	entry := qoderResolveCatalog(creds, client, force)
	if entry == nil {
		return nil
	}

	out := make([]model.ModelMetadata, 0, len(entry.rawConfigs))
	for key, raw := range entry.rawConfigs {
		m := model.ModelMetadata{ID: key}
		if name, ok := raw["name"].(string); ok && name != "" {
			m.DisplayName = name
		}
		if maxOut := toInt(raw["max_output_tokens"]); maxOut > 0 {
			m.MaxOutput = maxOut
		}
		if ctxLen := toInt(raw["context_length"]); ctxLen > 0 {
			m.ContextLength = ctxLen
		} else if ctxLen = toInt(raw["max_context_length"]); ctxLen > 0 {
			m.ContextLength = ctxLen
		}
		m.Capabilities = []string{"chat"}
		m.Modalities = []string{"text"}
		if isReasoning, _ := raw["is_reasoning"].(bool); isReasoning {
			m.Capabilities = append(m.Capabilities, "reasoning")
		}
		if hasVision, _ := raw["has_vision"].(bool); hasVision || strings.Contains(strings.ToLower(key), "vl") || strings.Contains(strings.ToLower(key), "vision") {
			m.Capabilities = append(m.Capabilities, "vision")
			m.Modalities = append(m.Modalities, "image")
		}
		if hasAudio, _ := raw["has_audio"].(bool); hasAudio || strings.Contains(strings.ToLower(key), "audio") || strings.Contains(strings.ToLower(key), "omni") {
			m.Capabilities = append(m.Capabilities, "audio")
			m.Modalities = append(m.Modalities, "audio")
		}
		out = append(out, m)
	}
	return out
}
