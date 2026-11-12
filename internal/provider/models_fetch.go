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
		URL:     p.ModelsURL,
		Auth:    p.ModelsAuth,
		Headers: p.Headers,
	}
	// The catalog may live under a different client identity than chat
	// (CodeBuddy Intl: IDE for chat, CLI for the model list). Prefer the
	// provider's dedicated catalog headers when declared.
	if len(p.ModelsHeaders) > 0 {
		cfg.Headers = p.ModelsHeaders
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
		if disp, ok := raw["display_name"].(string); ok && disp != "" {
			m.DisplayName = disp
		} else if name, ok := raw["name"].(string); ok && name != "" {
			m.DisplayName = name
		} else if label, ok := raw["label"].(string); ok && label != "" {
			m.DisplayName = label
		} else if title, ok := raw["title"].(string); ok && title != "" {
			m.DisplayName = title
		} else if desc, ok := raw["description"].(string); ok && desc != "" {
			m.DisplayName = desc
		} else {
			m.DisplayName = key
		}
		if maxOut := toInt(raw["max_output_tokens"]); maxOut > 0 {
			m.MaxOutput = maxOut
		}
		if ctxLen := toInt(raw["context_length"]); ctxLen > 0 {
			m.ContextLength = ctxLen
		} else if ctxLen = toInt(raw["max_context_length"]); ctxLen > 0 {
			m.ContextLength = ctxLen
		}
		// A present price_factor of 0 means free, but omitempty erases it later.
		if rawPF := raw["price_factor"]; rawPF != nil {
			if pf := toFloat(rawPF); pf > 0 {
				m.PriceFactor = pf
			} else {
				m.IsFree = true
			}
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
	// Anything the COSY catalog did not send is resolved from models.dev (by id or
	// display name); whatever still misses stays blank for manual entry — no static table.
	model.EnrichModelsFromCatalog(out)
	return out
}

// QoderCatalogModels reads model names straight from the live COSY catalog; there is
// intentionally no hardcoded key→name table here (AGENTS.md #5: keep model catalogs
// upstream-driven, not baked into the binary).
