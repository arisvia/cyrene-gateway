package model

import (
	"strings"
)

type ModelMetadata struct {
	ID            string   `json:"id"`
	DisplayName   string   `json:"display_name,omitempty"`
	ContextLength int      `json:"context_length,omitempty"`
	MaxOutput     int      `json:"max_output_tokens,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	Modalities    []string `json:"modalities,omitempty"`
	Family        string   `json:"family,omitempty"`
	FromUpstream  bool     `json:"from_upstream,omitempty"`
}

// CatalogEntry is a static catalog entry with pattern-based matching.
type CatalogEntry struct {
	Pattern       string // substring match (case-insensitive)
	DisplayName   string // human-readable name template (empty = use model ID)
	ContextLength int
	MaxOutput     int
	Capabilities  []string
	Modalities    []string
	Family        string
}

// StaticCatalog is a curated list of mainstream model metadata.
// Ordered by specificity: more specific patterns first.
var StaticCatalog = []CatalogEntry{
	// OpenAI (2024–2026 mainstream)
	{Pattern: "gpt-4.5", DisplayName: "GPT-4.5", ContextLength: 128000, MaxOutput: 16384, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-4"},
	{Pattern: "gpt-4o-mini", DisplayName: "GPT-4o Mini", ContextLength: 128000, MaxOutput: 16384, Capabilities: []string{"chat", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-4o"},
	{Pattern: "gpt-4o", DisplayName: "GPT-4o", ContextLength: 128000, MaxOutput: 16384, Capabilities: []string{"chat", "vision"}, Modalities: []string{"text", "image", "audio"}, Family: "gpt-4o"},
	{Pattern: "gpt-4-turbo", DisplayName: "GPT-4 Turbo", ContextLength: 128000, MaxOutput: 4096, Capabilities: []string{"chat", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-4"},
	{Pattern: "gpt-5.4-mini", DisplayName: "GPT-5.4 Mini", ContextLength: 256000, MaxOutput: 16384, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-5"},
	{Pattern: "gpt-5.4", DisplayName: "GPT-5.4", ContextLength: 256000, MaxOutput: 32768, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-5"},
	{Pattern: "gpt-5-mini", DisplayName: "GPT-5 Mini", ContextLength: 256000, MaxOutput: 16384, Capabilities: []string{"chat", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-5"},
	{Pattern: "gpt-5", DisplayName: "GPT-5", ContextLength: 256000, MaxOutput: 32768, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-5"},
	{Pattern: "gpt-oss-120b", DisplayName: "GPT-OSS 120B", ContextLength: 131072, MaxOutput: 32768, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "openai"},
	{Pattern: "gpt-oss-20b", DisplayName: "GPT-OSS 20B", ContextLength: 131072, MaxOutput: 32768, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "openai"},
	{Pattern: "o3-pro", DisplayName: "O3 Pro", ContextLength: 200000, MaxOutput: 100000, Capabilities: []string{"chat", "reasoning", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "o-series"},
	{Pattern: "o3-mini", DisplayName: "O3 Mini", ContextLength: 200000, MaxOutput: 100000, Capabilities: []string{"chat", "reasoning", "code"}, Modalities: []string{"text"}, Family: "o-series"},
	{Pattern: "o3", DisplayName: "O3", ContextLength: 200000, MaxOutput: 100000, Capabilities: []string{"chat", "reasoning", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "o-series"},
	{Pattern: "o1-mini", DisplayName: "O1 Mini", ContextLength: 128000, MaxOutput: 65536, Capabilities: []string{"chat", "reasoning", "code"}, Modalities: []string{"text"}, Family: "o-series"},
	{Pattern: "o1", DisplayName: "O1", ContextLength: 200000, MaxOutput: 100000, Capabilities: []string{"chat", "reasoning", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "o-series"},
	{Pattern: "text-embedding-3-large", DisplayName: "Text Embedding 3 Large", ContextLength: 8191, Capabilities: []string{"embeddings"}, Modalities: []string{"text"}, Family: "embedding"},
	{Pattern: "text-embedding-3-small", DisplayName: "Text Embedding 3 Small", ContextLength: 8191, Capabilities: []string{"embeddings"}, Modalities: []string{"text"}, Family: "embedding"},
	{Pattern: "dall-e-3", DisplayName: "DALL-E 3", Capabilities: []string{"image-generation"}, Modalities: []string{"text", "image"}, Family: "image"},
	{Pattern: "whisper", DisplayName: "Whisper", Capabilities: []string{"stt"}, Modalities: []string{"audio", "text"}, Family: "audio"},
	{Pattern: "tts-1-hd", DisplayName: "TTS-1 HD", Capabilities: []string{"tts"}, Modalities: []string{"text", "audio"}, Family: "audio"},
	{Pattern: "tts-1", DisplayName: "TTS-1", Capabilities: []string{"tts"}, Modalities: []string{"text", "audio"}, Family: "audio"},

	// Anthropic Claude
	{Pattern: "claude-3-7-sonnet", DisplayName: "Claude 3.7 Sonnet", ContextLength: 200000, MaxOutput: 64000, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude-3-5-sonnet", DisplayName: "Claude 3.5 Sonnet", ContextLength: 200000, MaxOutput: 8192, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude-3-5-haiku", DisplayName: "Claude 3.5 Haiku", ContextLength: 200000, MaxOutput: 8192, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude-3-opus", DisplayName: "Claude 3 Opus", ContextLength: 200000, MaxOutput: 4096, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude-3-sonnet", DisplayName: "Claude 3 Sonnet", ContextLength: 200000, MaxOutput: 4096, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude-3-haiku", DisplayName: "Claude 3 Haiku", ContextLength: 200000, MaxOutput: 4096, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude-opus-4", DisplayName: "Claude Opus 4", ContextLength: 200000, MaxOutput: 32768, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude-sonnet-4", DisplayName: "Claude Sonnet 4", ContextLength: 200000, MaxOutput: 64000, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude", DisplayName: "Claude", ContextLength: 200000, MaxOutput: 8192, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "claude"},

	// Google Gemini
	{Pattern: "gemini-2.5-pro", DisplayName: "Gemini 2.5 Pro", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image", "audio", "video"}, Family: "gemini"},
	{Pattern: "gemini-2.5-flash-lite", DisplayName: "Gemini 2.5 Flash Lite", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gemini"},
	{Pattern: "gemini-2.5-flash", DisplayName: "Gemini 2.5 Flash", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image", "audio"}, Family: "gemini"},
	{Pattern: "gemini-2.0-flash-lite", DisplayName: "Gemini 2.0 Flash Lite", ContextLength: 1048576, MaxOutput: 8192, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gemini"},
	{Pattern: "gemini-2.0-flash", DisplayName: "Gemini 2.0 Flash", ContextLength: 1048576, MaxOutput: 8192, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image", "audio"}, Family: "gemini"},
	{Pattern: "gemini-2.0-pro", DisplayName: "Gemini 2.0 Pro", ContextLength: 2097152, MaxOutput: 8192, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image", "audio", "video"}, Family: "gemini"},
	{Pattern: "gemini-1.5-pro", DisplayName: "Gemini 1.5 Pro", ContextLength: 2097152, MaxOutput: 8192, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image", "audio", "video"}, Family: "gemini"},
	{Pattern: "gemini-1.5-flash", DisplayName: "Gemini 1.5 Flash", ContextLength: 1048576, MaxOutput: 8192, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image", "audio"}, Family: "gemini"},
	{Pattern: "gemini-3.8-flash", DisplayName: "Gemini 3.8 Flash", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image", "audio", "video"}, Family: "gemini"},
	{Pattern: "gemini-3.7-flash", DisplayName: "Gemini 3.7 Flash", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image", "audio", "video"}, Family: "gemini"},
	{Pattern: "gemini-3.5-flash-lite", DisplayName: "Gemini 3.5 Flash Lite", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gemini"},
	{Pattern: "gemini-3.5-flash", DisplayName: "Gemini 3.5 Flash", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gemini"},
	{Pattern: "gemini-3.1-pro", DisplayName: "Gemini 3.1 Pro", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image", "audio", "video"}, Family: "gemini"},
	{Pattern: "gemini-3-flash", DisplayName: "Gemini 3 Flash", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image", "audio"}, Family: "gemini"},
	{Pattern: "gemma-3", DisplayName: "Gemma 3", ContextLength: 131072, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "gemma"},
	{Pattern: "gemma-2", DisplayName: "Gemma 2", ContextLength: 8192, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "gemma"},
	{Pattern: "gemini-embedding", DisplayName: "Gemini Embedding", ContextLength: 2048, Capabilities: []string{"embeddings"}, Modalities: []string{"text"}, Family: "gemini"},

	// DeepSeek (2024–2026 official)
	{Pattern: "deepseek-v4.1-flash", DisplayName: "DeepSeek V4.1 Flash", ContextLength: 1000000, MaxOutput: 384000, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "deepseek"},
	{Pattern: "deepseek-v4-pro", DisplayName: "DeepSeek V4 Pro", ContextLength: 1000000, MaxOutput: 384000, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "deepseek"},
	{Pattern: "deepseek-v4-flash", DisplayName: "DeepSeek V4 Flash", ContextLength: 1000000, MaxOutput: 384000, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "deepseek"},
	{Pattern: "deepseek-v3.2", DisplayName: "DeepSeek V3.2", ContextLength: 128000, MaxOutput: 32768, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "deepseek"},
	{Pattern: "deepseek-v3.1", DisplayName: "DeepSeek V3.1", ContextLength: 128000, MaxOutput: 32000, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "deepseek"},
	{Pattern: "deepseek-v3", DisplayName: "DeepSeek V3", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "deepseek"},
	{Pattern: "deepseek-r1", DisplayName: "DeepSeek R1", ContextLength: 128000, MaxOutput: 64000, Capabilities: []string{"chat", "reasoning", "code"}, Modalities: []string{"text"}, Family: "deepseek"},
	{Pattern: "deepseek-chat", DisplayName: "DeepSeek Chat", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "deepseek"},
	{Pattern: "deepseek-reasoner", DisplayName: "DeepSeek Reasoner", ContextLength: 128000, MaxOutput: 64000, Capabilities: []string{"chat", "reasoning", "code"}, Modalities: []string{"text"}, Family: "deepseek"},

	// Moonshot Kimi
	{Pattern: "kimi-k3", DisplayName: "Kimi K3", ContextLength: 1048576, MaxOutput: 131072, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "kimi"},
	{Pattern: "kimi-k2.8", DisplayName: "Kimi K2.8 Preview", ContextLength: 262144, MaxOutput: 262144, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "kimi"},
	{Pattern: "kimi-k2.7-code", DisplayName: "Kimi K2.7 Code", ContextLength: 262144, MaxOutput: 262144, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "kimi"},
	{Pattern: "kimi-k2.7", DisplayName: "Kimi K2.7", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "kimi"},
	{Pattern: "kimi-k2.6", DisplayName: "Kimi K2.6", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "kimi"},
	{Pattern: "kimi-k2-thinking", DisplayName: "Kimi K2 Thinking", ContextLength: 262144, MaxOutput: 65536, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "kimi"},
	{Pattern: "kimi-k2.5", DisplayName: "Kimi K2.5", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "kimi"},
	{Pattern: "moonshot-v1-128k", DisplayName: "Moonshot V1 128K", ContextLength: 128000, MaxOutput: 4096, Capabilities: []string{"chat"}, Modalities: []string{"text"}, Family: "kimi"},
	{Pattern: "moonshot-v1-32k", DisplayName: "Moonshot V1 32K", ContextLength: 32768, MaxOutput: 4096, Capabilities: []string{"chat"}, Modalities: []string{"text"}, Family: "kimi"},
	{Pattern: "moonshot-v1-8k", DisplayName: "Moonshot V1 8K", ContextLength: 8192, MaxOutput: 4096, Capabilities: []string{"chat"}, Modalities: []string{"text"}, Family: "kimi"},

	// Zhipu GLM
	{Pattern: "glm-5.3-flash", DisplayName: "GLM 5.3 Flash", ContextLength: 1310720, MaxOutput: 131072, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "glm"},
	{Pattern: "glm-5.3", DisplayName: "GLM 5.3", ContextLength: 1000000, MaxOutput: 131072, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image"}, Family: "glm"},
	{Pattern: "glm-5.2", DisplayName: "GLM 5.2", ContextLength: 1000000, MaxOutput: 131072, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image"}, Family: "glm"},
	{Pattern: "glm-5.1", DisplayName: "GLM 5.1", ContextLength: 200000, MaxOutput: 131072, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "glm"},
	{Pattern: "glm-5", DisplayName: "GLM 5", ContextLength: 204800, MaxOutput: 131072, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "glm"},
	{Pattern: "glm-4-plus", DisplayName: "GLM 4 Plus", ContextLength: 128000, MaxOutput: 4096, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "glm"},
	{Pattern: "glm-4-flash", DisplayName: "GLM 4 Flash", ContextLength: 128000, MaxOutput: 4096, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "glm"},
	{Pattern: "glm-4-air", DisplayName: "GLM 4 Air", ContextLength: 128000, MaxOutput: 4096, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "glm"},
	{Pattern: "glm-4", DisplayName: "GLM 4", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "glm"},
	{Pattern: "codegeex-4", DisplayName: "CodeGeeX 4", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"code"}, Modalities: []string{"text"}, Family: "glm"},

	// Alibaba Qwen
	{Pattern: "qwen3.8-max", DisplayName: "Qwen 3.8 Max", ContextLength: 1048576, MaxOutput: 131072, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwen3.8-flash", DisplayName: "Qwen 3.8 Flash", ContextLength: 1048576, MaxOutput: 131072, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwen3.8", DisplayName: "Qwen 3.8", ContextLength: 1048576, MaxOutput: 131072, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwen3.7-max", DisplayName: "Qwen 3.7 Max", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwen3.7-plus", DisplayName: "Qwen 3.7 Plus", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwen2.5-max", DisplayName: "Qwen 2.5 Max", ContextLength: 131072, MaxOutput: 8192, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwen2.5-plus", DisplayName: "Qwen 2.5 Plus", ContextLength: 131072, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwen2.5-turbo", DisplayName: "Qwen 2.5 Turbo", ContextLength: 1000000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwen2.5-coder-32b", DisplayName: "Qwen 2.5 Coder 32B", ContextLength: 131072, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwen2.5-coder", DisplayName: "Qwen 2.5 Coder", ContextLength: 131072, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwen2.5-72b", DisplayName: "Qwen 2.5 72B", ContextLength: 131072, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwq-32b", DisplayName: "QwQ 32B", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "reasoning", "code"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwq", DisplayName: "QwQ", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "reasoning", "code"}, Modalities: []string{"text"}, Family: "qwen"},

	// MiniMax
	{Pattern: "minimax-m3", DisplayName: "MiniMax M3", ContextLength: 1048576, MaxOutput: 512000, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "minimax"},
	{Pattern: "minimax-m2.7", DisplayName: "MiniMax M2.7", ContextLength: 204800, MaxOutput: 131072, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "minimax"},
	{Pattern: "minimax-m2.5", DisplayName: "MiniMax M2.5", ContextLength: 1000000, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "minimax"},
	{Pattern: "minimax-m2", DisplayName: "MiniMax M2", ContextLength: 1000000, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "minimax"},
	{Pattern: "abab6.5s", DisplayName: "Abab 6.5s", ContextLength: 245760, MaxOutput: 4096, Capabilities: []string{"chat"}, Modalities: []string{"text"}, Family: "minimax"},
	{Pattern: "abab6.5", DisplayName: "Abab 6.5", ContextLength: 245760, MaxOutput: 4096, Capabilities: []string{"chat"}, Modalities: []string{"text"}, Family: "minimax"},

	// xAI Grok
	{Pattern: "grok-4-fast", DisplayName: "Grok 4 Fast", ContextLength: 2000000, MaxOutput: 2000000, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "grok"},
	{Pattern: "grok-4", DisplayName: "Grok 4", ContextLength: 256000, MaxOutput: 256000, Capabilities: []string{"chat", "code", "reasoning", "vision"}, Modalities: []string{"text", "image"}, Family: "grok"},
	{Pattern: "grok-3-mini", DisplayName: "Grok 3 Mini", ContextLength: 131072, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "grok"},
	{Pattern: "grok-3", DisplayName: "Grok 3", ContextLength: 131072, MaxOutput: 8192, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "grok"},
	{Pattern: "grok-2-mini", DisplayName: "Grok 2 Mini", ContextLength: 131072, MaxOutput: 4096, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "grok"},
	{Pattern: "grok-2", DisplayName: "Grok 2", ContextLength: 131072, MaxOutput: 4096, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "grok"},

	// Mistral AI
	{Pattern: "mistral-large-3", DisplayName: "Mistral Large 3", ContextLength: 262144, MaxOutput: 256000, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "mistral"},
	{Pattern: "mistral-large", DisplayName: "Mistral Large", ContextLength: 128000, MaxOutput: 102400, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "mistral"},
	{Pattern: "mistral-small-3", DisplayName: "Mistral Small 3", ContextLength: 128000, MaxOutput: 32768, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "mistral"},
	{Pattern: "mistral-small", DisplayName: "Mistral Small", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "mistral"},
	{Pattern: "codestral-2508", DisplayName: "Codestral 2508", ContextLength: 256000, MaxOutput: 32768, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "mistral"},
	{Pattern: "codestral", DisplayName: "Codestral", ContextLength: 256000, MaxOutput: 32768, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "mistral"},
	{Pattern: "pixtral-large", DisplayName: "Pixtral Large", ContextLength: 128000, MaxOutput: 32768, Capabilities: []string{"chat", "vision"}, Modalities: []string{"text", "image"}, Family: "mistral"},
	{Pattern: "pixtral-12b", DisplayName: "Pixtral 12B", ContextLength: 128000, MaxOutput: 4096, Capabilities: []string{"chat", "vision"}, Modalities: []string{"text", "image"}, Family: "mistral"},
	{Pattern: "ministral-8b", DisplayName: "Ministral 8B", ContextLength: 131072, MaxOutput: 32768, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "mistral"},
	{Pattern: "ministral-3b", DisplayName: "Ministral 3B", ContextLength: 131072, MaxOutput: 32768, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "mistral"},

	// Meta Llama
	{Pattern: "llama-4-scout", DisplayName: "Llama 4 Scout", ContextLength: 328000, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "llama"},
	{Pattern: "llama-4", DisplayName: "Llama 4", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "llama"},
	{Pattern: "llama-3.3-70b", DisplayName: "Llama 3.3 70B", ContextLength: 128000, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "llama"},
	{Pattern: "llama-3.3", DisplayName: "Llama 3.3", ContextLength: 128000, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "llama"},
	{Pattern: "llama-3.2", DisplayName: "Llama 3.2", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "llama"},
	{Pattern: "llama-3.1", DisplayName: "Llama 3.1", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "llama"},

	// Tencent Hunyuan (Hy)
	{Pattern: "hy4-preview", DisplayName: "Hy4 Preview", ContextLength: 1048576, MaxOutput: 64000, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "hunyuan"},
	{Pattern: "hy4", DisplayName: "Hy4", ContextLength: 1048576, MaxOutput: 64000, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "hunyuan"},
	{Pattern: "hy3", DisplayName: "Hy3", ContextLength: 262144, MaxOutput: 128000, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "hunyuan"},
	{Pattern: "hunyuan-pro", DisplayName: "Hunyuan Pro", ContextLength: 256000, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "hunyuan"},
	{Pattern: "hunyuan-turbos", DisplayName: "Hunyuan TurboS", ContextLength: 200000, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "hunyuan"},
	{Pattern: "hunyuan-t1", DisplayName: "Hunyuan T1", ContextLength: 256000, MaxOutput: 16384, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "hunyuan"},

	// ByteDance Doubao / Seed
	{Pattern: "doubao-pro-128k", DisplayName: "Doubao Pro 128K", ContextLength: 128000, MaxOutput: 4096, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "seed"},
	{Pattern: "doubao-pro-32k", DisplayName: "Doubao Pro 32K", ContextLength: 32768, MaxOutput: 4096, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "seed"},
	{Pattern: "doubao-lite-128k", DisplayName: "Doubao Lite 128K", ContextLength: 128000, MaxOutput: 4096, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "seed"},
	{Pattern: "seed-2.0-pro", DisplayName: "Doubao Seed 2.0 Pro", ContextLength: 256000, MaxOutput: 128000, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "seed"},
	{Pattern: "seed-2.0", DisplayName: "Doubao Seed 2.0", ContextLength: 256000, MaxOutput: 32000, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "seed"},
	{Pattern: "seed-1.6", DisplayName: "Doubao Seed 1.6", ContextLength: 256000, MaxOutput: 32000, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "seed"},

	// StepFun (阶跃星辰)
	{Pattern: "step-3.7-flash", DisplayName: "Step 3.7 Flash", ContextLength: 262144, MaxOutput: 256000, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "step"},
	{Pattern: "step-3.5-flash", DisplayName: "Step 3.5 Flash", ContextLength: 262144, MaxOutput: 65536, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "step"},
	{Pattern: "step-2", DisplayName: "Step 2", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "step"},

	// Perplexity Sonar & Cohere & Amazon Nova
	{Pattern: "sonar-deep-research", DisplayName: "Sonar Deep Research", ContextLength: 128000, MaxOutput: 32768, Capabilities: []string{"chat", "reasoning"}, Modalities: []string{"text"}, Family: "perplexity"},
	{Pattern: "sonar-reasoning-pro", DisplayName: "Sonar Reasoning Pro", ContextLength: 128000, MaxOutput: 4096, Capabilities: []string{"chat", "reasoning"}, Modalities: []string{"text"}, Family: "perplexity"},
	{Pattern: "sonar-pro", DisplayName: "Sonar Pro", ContextLength: 200000, MaxOutput: 8192, Capabilities: []string{"chat"}, Modalities: []string{"text"}, Family: "perplexity"},
	{Pattern: "command-r-plus", DisplayName: "Command R+", ContextLength: 128000, MaxOutput: 4096, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "command"},
	{Pattern: "command-r", DisplayName: "Command R", ContextLength: 128000, MaxOutput: 4000, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "command"},
	{Pattern: "nova-2-pro", DisplayName: "Nova 2 Pro", ContextLength: 1000000, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "nova"},
	{Pattern: "nova-2-lite", DisplayName: "Nova 2 Lite", ContextLength: 1000000, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "nova"},
	{Pattern: "nova-pro", DisplayName: "Nova Pro", ContextLength: 300000, MaxOutput: 5000, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "nova"},
}

// LookupCatalog finds the best matching catalog entry for a model ID or display name.
// It first attempts to resolve via the dynamically synchronized models.dev catalog (if loaded),
// and falls back to the curated static catalog of mainstream foundation models (2024–2026).
func LookupCatalog(identifiers ...string) *ModelMetadata {
	// 1. Dynamic models.dev catalog lookup if cached (covers 3600+ live models)
	if devMeta := LookupModelsDevGlobal(identifiers...); devMeta != nil {
		return devMeta
	}

	// 2. Curated fallback static catalog (2024–2026 mainstream models)
	for _, raw := range identifiers {
		if raw == "" {
			continue
		}
		lower := toLower(raw)
		lowerDashed := strings.ReplaceAll(lower, " ", "-")
		for i := range StaticCatalog {
			pat := toLower(StaticCatalog[i].Pattern)
			if containsStr(lower, pat) || containsStr(lowerDashed, pat) {
				e := &StaticCatalog[i]
				name := e.DisplayName
				if name == "" {
					name = raw
				}
				primaryID := raw
				if len(identifiers) > 0 && identifiers[0] != "" {
					primaryID = identifiers[0]
				}
				return &ModelMetadata{
					ID:            primaryID,
					DisplayName:   name,
					ContextLength: e.ContextLength,
					MaxOutput:     e.MaxOutput,
					Capabilities:  e.Capabilities,
					Modalities:    e.Modalities,
					Family:        e.Family,
				}
			}
		}
	}
	return nil
}

// FormatFallbackDisplayName formats an arbitrary model ID into a readable display name
// when upstream returns an empty name and static catalog does not match.
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

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func containsStr(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
