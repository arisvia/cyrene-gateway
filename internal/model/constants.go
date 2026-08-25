package model

// Centralized KV Store Scope Constants (eliminates magic strings)
const (
	KVScopeAliases            = "aliases"
	KVScopeDisabledModels     = "disabledModels"
	KVScopeProviderModelCache = "providerModelCache"
	KVScopeCustomModels       = "customModels"
)

// Standard Provider Names & Formats
const (
	FormatOpenAI    = "openai"
	FormatAnthropic = "anthropic"
	FormatGemini    = "gemini"
)
