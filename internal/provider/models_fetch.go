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
		} else if fallback, ok := qoderModelDisplayNames[key]; ok {
			m.DisplayName = fallback
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
// qoderModelDisplayNames provides human-friendly names for Qoder model keys when the live catalog omits name.
var qoderModelDisplayNames = map[string]string{
	"ultimate":      "Ultimate",
	"auto":          "Auto",
	"performance":   "Performance",
	"efficient":     "Efficient",
	"lite":          "Lite",
	"qmodel_38max":  "Qwen3.8-Max",
	"qmodel_latest": "Qwen3.7-Max",
	"qmodel":        "Qwen3.7-Plus",
	"qfmodel":       "Qwen3.7-Flash",
	"kmodel_latest": "Kimi-K3",
	"kmodel":        "Kimi-K2.7-Code",
	"gmodel":        "GLM-5.3",
	"gfmodel":       "GLM-5.3-Flash",
	"gm51model":     "GLM-5.3",
	"dmodel":        "DeepSeek-V4-Pro",
	"dfmodel":       "DeepSeek-V4-Flash",
	"mmodel":        "MiniMax-M3",
	"cmodel":        "Cantus",
}
