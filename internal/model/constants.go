package model

// Centralized KV Store Scope Constants (eliminates magic strings)
const (
	KVScopeDisabledModels     = "disabledModels"
	KVScopeProviderModelCache = "providerModelCache"
	// KVScopeCustomModels holds per-connection user-declared model lists, keyed
	// by connection ID. The value is "providerModels" — the scope actually
	// written since the feature shipped; an earlier name here matched nothing.
	KVScopeCustomModels       = "providerModels"
	KVScopeModelMetaOverrides = "modelMetaOverrides"
)

// Standard Provider Names & Formats
const (
	FormatOpenAI    = "openai"
	FormatAnthropic = "anthropic"
	FormatGemini    = "gemini"
)
