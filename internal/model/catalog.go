package model

import (
	"strings"
)

type ModelMetadata struct {
	ID            string   `json:"id"`
	DisplayName   string   `json:"display_name,omitempty"`
	ContextLength int      `json:"context_length,omitempty"`
	MaxOutput     int      `json:"max_output_tokens,omitempty"`
	PriceFactor   float64  `json:"price_factor,omitempty"`
	IsFree        bool     `json:"is_free,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	Modalities    []string `json:"modalities,omitempty"`
	Family        string   `json:"family,omitempty"`
	FromUpstream  bool     `json:"from_upstream,omitempty"`
}

// LookupCatalog resolves a model id or a human-readable display name against the
// dynamically synchronized models.dev catalog. There is intentionally no curated
// static table here: metadata the upstream provider did not send is either matched
// from models.dev (by id or by display name) or left blank for operators to fill in
// manually via the model-detail override UI.
func LookupCatalog(identifiers ...string) *ModelMetadata {
	return LookupModelsDevGlobal(identifiers...)
}

// FormatFallbackDisplayName formats an arbitrary model ID into a readable display name
// when upstream returns an empty name and models.dev does not match.
// E.g. "gemini-3.7-flash-tiered" -> "Gemini 3.7 Flash (Tiered)"
func FormatFallbackDisplayName(modelID string) string {
	if cat := LookupCatalog(modelID); cat != nil && cat.DisplayName != "" && cat.DisplayName != modelID {
		return cat.DisplayName
	}
	parts := strings.Split(modelID, "-")
	var titleParts []string
	for _, p := range parts {
		if p == "" {
			continue
		}
		lower := strings.ToLower(p)
		switch lower {
		case "gpt":
			titleParts = append(titleParts, "GPT")
		case "oss":
			titleParts = append(titleParts, "OSS")
		case "claude":
			titleParts = append(titleParts, "Claude")
		case "gemini":
			titleParts = append(titleParts, "Gemini")
		case "gemma":
			titleParts = append(titleParts, "Gemma")
		case "deepseek":
			titleParts = append(titleParts, "DeepSeek")
		case "qwen":
			titleParts = append(titleParts, "Qwen")
		case "glm":
			titleParts = append(titleParts, "GLM")
		case "kimi":
			titleParts = append(titleParts, "Kimi")
		case "minimax":
			titleParts = append(titleParts, "MiniMax")
		case "tiered":
			titleParts = append(titleParts, "(Tiered)")
		case "high":
			titleParts = append(titleParts, "(High)")
		case "medium":
			titleParts = append(titleParts, "(Medium)")
		case "low":
			titleParts = append(titleParts, "(Low)")
		case "pro":
			titleParts = append(titleParts, "Pro")
		case "flash":
			titleParts = append(titleParts, "Flash")
		case "lite":
			titleParts = append(titleParts, "Lite")
		case "image":
			titleParts = append(titleParts, "Image")
		default:
			if len(p) > 0 {
				titleParts = append(titleParts, strings.ToUpper(p[:1])+p[1:])
			}
		}
	}
	if len(titleParts) == 0 {
		return modelID
	}
	return strings.Join(titleParts, " ")
}
