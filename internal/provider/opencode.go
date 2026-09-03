package provider

import "strings"

// IsOpenCodeFreeModel checks whether an OpenCode model is available on the free tier (no API key required).
func IsOpenCodeFreeModel(id string) bool {
	idLower := strings.ToLower(id)
	return strings.HasSuffix(idLower, "-free") || idLower == "big-pickle"
}

// GetOpenCodeFreeModels returns standard fallback free models for OpenCode.
func GetOpenCodeFreeModels() []ModelRef {
	return []ModelRef{
		{ID: "big-pickle", Name: "Big Pickle (Free)"},
		{ID: "mimo-v2.5-free", Name: "Mimo v2.5 (Free)"},
		{ID: "ling-3.0-flash-fin-free", Name: "Ling 3.0 Flash (Free)"},
		{ID: "deepseek-v4-flash-free", Name: "DeepSeek V4 Flash (Free)"},
		{ID: "nemotron-3-ultra-free", Name: "Nemotron 3 Ultra (Free)"},
		{ID: "nemotron-3.5-lightning-free", Name: "Nemotron 3.5 Lightning (Free)"},
		{ID: "laguna-s-2.1-free", Name: "Laguna S 2.1 (Free)"},
		{ID: "muse-spark-1.3-contributor-free", Name: "Muse Spark 1.3 (Free)"},
		{ID: "muse-spark-1.2-contributor-free", Name: "Muse Spark 1.2 (Free)"},
	}
}
