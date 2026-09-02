package model

// ModelMetadata holds rich metadata for a single model.
type ModelMetadata struct {
	ID            string   `json:"id"`
	DisplayName   string   `json:"display_name,omitempty"`
	ContextLength int      `json:"context_length,omitempty"`
	MaxOutput     int      `json:"max_output_tokens,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	Modalities    []string `json:"modalities,omitempty"`
	Family        string   `json:"family,omitempty"`
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
	// OpenAI GPT-5.x series
	{Pattern: "gpt-5.6-sol", DisplayName: "GPT-5.6 Sol", ContextLength: 256000, MaxOutput: 32768, Capabilities: []string{"chat", "reasoning", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-5"},
	{Pattern: "gpt-5.6-terra", DisplayName: "GPT-5.6 Terra", ContextLength: 256000, MaxOutput: 32768, Capabilities: []string{"chat", "reasoning", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-5"},
	{Pattern: "gpt-5.6-luna", DisplayName: "GPT-5.6 Luna", ContextLength: 256000, MaxOutput: 32768, Capabilities: []string{"chat", "reasoning", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-5"},
	{Pattern: "gpt-5.5", DisplayName: "GPT-5.5", ContextLength: 256000, MaxOutput: 32768, Capabilities: []string{"chat", "reasoning", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-5"},
	{Pattern: "gpt-5.4-mini", DisplayName: "GPT-5.4 Mini", ContextLength: 256000, MaxOutput: 16384, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-5"},
	{Pattern: "gpt-5.4-nano", DisplayName: "GPT-5.4 Nano", ContextLength: 128000, MaxOutput: 16384, Capabilities: []string{"chat", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-5"},
	{Pattern: "gpt-5.4", DisplayName: "GPT-5.4", ContextLength: 256000, MaxOutput: 32768, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-5"},
	{Pattern: "gpt-5.3-codex", DisplayName: "GPT-5.3 Codex", ContextLength: 256000, MaxOutput: 65536, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "gpt-5"},
	{Pattern: "gpt-5.2-codex", DisplayName: "GPT-5.2 Codex", ContextLength: 256000, MaxOutput: 65536, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "gpt-5"},
	{Pattern: "gpt-5.2", DisplayName: "GPT-5.2", ContextLength: 256000, MaxOutput: 32768, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-5"},
	{Pattern: "gpt-5.1", DisplayName: "GPT-5.1", ContextLength: 256000, MaxOutput: 32768, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-5"},
	{Pattern: "gpt-5-mini", DisplayName: "GPT-5 Mini", ContextLength: 256000, MaxOutput: 16384, Capabilities: []string{"chat", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-5"},
	{Pattern: "gpt-5-nano", DisplayName: "GPT-5 Nano", ContextLength: 128000, MaxOutput: 16384, Capabilities: []string{"chat"}, Modalities: []string{"text"}, Family: "gpt-5"},
	{Pattern: "gpt-5", DisplayName: "GPT-5", ContextLength: 256000, MaxOutput: 32768, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-5"},

	// OpenAI GPT-4.x series
	{Pattern: "gpt-4o-mini-tts", DisplayName: "GPT-4o Mini TTS", ContextLength: 128000, MaxOutput: 4096, Capabilities: []string{"tts"}, Modalities: []string{"text", "audio"}, Family: "gpt-4o"},
	{Pattern: "gpt-4o-mini", DisplayName: "GPT-4o Mini", ContextLength: 128000, MaxOutput: 16384, Capabilities: []string{"chat", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-4o"},
	{Pattern: "gpt-4o-transcribe", DisplayName: "GPT-4o Transcribe", Capabilities: []string{"stt"}, Modalities: []string{"audio", "text"}, Family: "gpt-4o"},
	{Pattern: "gpt-4o", DisplayName: "GPT-4o", ContextLength: 128000, MaxOutput: 16384, Capabilities: []string{"chat", "vision"}, Modalities: []string{"text", "image", "audio"}, Family: "gpt-4o"},
	{Pattern: "gpt-4.1-mini", DisplayName: "GPT-4.1 Mini", ContextLength: 1047576, MaxOutput: 32768, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-4"},
	{Pattern: "gpt-4.1-nano", DisplayName: "GPT-4.1 Nano", ContextLength: 1047576, MaxOutput: 32768, Capabilities: []string{"chat", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-4"},
	{Pattern: "gpt-4.1", DisplayName: "GPT-4.1", ContextLength: 1047576, MaxOutput: 32768, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-4"},
	{Pattern: "gpt-4-turbo", DisplayName: "GPT-4 Turbo", ContextLength: 128000, MaxOutput: 4096, Capabilities: []string{"chat", "vision"}, Modalities: []string{"text", "image"}, Family: "gpt-4"},

	// OpenAI reasoning models
	{Pattern: "o3-pro", DisplayName: "O3 Pro", ContextLength: 200000, MaxOutput: 100000, Capabilities: []string{"chat", "reasoning", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "o-series"},
	{Pattern: "o3-mini", DisplayName: "O3 Mini", ContextLength: 200000, MaxOutput: 100000, Capabilities: []string{"chat", "reasoning", "code"}, Modalities: []string{"text"}, Family: "o-series"},
	{Pattern: "o3", DisplayName: "O3", ContextLength: 200000, MaxOutput: 100000, Capabilities: []string{"chat", "reasoning", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "o-series"},
	{Pattern: "o4-mini", DisplayName: "O4 Mini", ContextLength: 200000, MaxOutput: 100000, Capabilities: []string{"chat", "reasoning", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "o-series"},
	{Pattern: "o1-mini", DisplayName: "O1 Mini", ContextLength: 128000, MaxOutput: 65536, Capabilities: []string{"chat", "reasoning", "code"}, Modalities: []string{"text"}, Family: "o-series"},
	{Pattern: "o1", DisplayName: "O1", ContextLength: 200000, MaxOutput: 100000, Capabilities: []string{"chat", "reasoning", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "o-series"},

	// OpenAI image/embedding/audio
	{Pattern: "gpt-image-1", DisplayName: "GPT Image 1", Capabilities: []string{"image-generation"}, Modalities: []string{"text", "image"}, Family: "image"},
	{Pattern: "dall-e-3", DisplayName: "DALL-E 3", Capabilities: []string{"image-generation"}, Modalities: []string{"text", "image"}, Family: "image"},
	{Pattern: "text-embedding-3-large", DisplayName: "Text Embedding 3 Large", ContextLength: 8191, Capabilities: []string{"embeddings"}, Modalities: []string{"text"}, Family: "embedding"},
	{Pattern: "text-embedding-3-small", DisplayName: "Text Embedding 3 Small", ContextLength: 8191, Capabilities: []string{"embeddings"}, Modalities: []string{"text"}, Family: "embedding"},
	{Pattern: "whisper", DisplayName: "Whisper", Capabilities: []string{"stt"}, Modalities: []string{"audio", "text"}, Family: "audio"},
	{Pattern: "tts-1-hd", DisplayName: "TTS-1 HD", Capabilities: []string{"tts"}, Modalities: []string{"text", "audio"}, Family: "audio"},
	{Pattern: "tts-1", DisplayName: "TTS-1", Capabilities: []string{"tts"}, Modalities: []string{"text", "audio"}, Family: "audio"},

	// Anthropic Claude
	{Pattern: "claude-opus-5", DisplayName: "Claude Opus 5", ContextLength: 1000000, MaxOutput: 128000, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude-fable-5", DisplayName: "Claude Fable 5", ContextLength: 200000, MaxOutput: 64000, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude-sonnet-5", DisplayName: "Claude Sonnet 5", ContextLength: 200000, MaxOutput: 64000, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude-haiku-4-5", DisplayName: "Claude 4.5 Haiku", ContextLength: 200000, MaxOutput: 8192, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude-opus-4", DisplayName: "Claude Opus 4", ContextLength: 200000, MaxOutput: 32768, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude-sonnet-4-6", DisplayName: "Claude Sonnet 4.6", ContextLength: 200000, MaxOutput: 64000, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude-sonnet-4", DisplayName: "Claude Sonnet 4", ContextLength: 200000, MaxOutput: 64000, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude-3-7-sonnet", DisplayName: "Claude 3.7 Sonnet", ContextLength: 200000, MaxOutput: 64000, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude-3-5-sonnet", DisplayName: "Claude 3.5 Sonnet", ContextLength: 200000, MaxOutput: 8192, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "claude"},
	{Pattern: "claude", DisplayName: "Claude", ContextLength: 200000, MaxOutput: 8192, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "claude"},
	// Google Gemini
	{Pattern: "gemini-3.6-flash", DisplayName: "Gemini 3.6 Flash", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image", "audio", "video"}, Family: "gemini"},
	{Pattern: "gemini-3.5-flash-lite", DisplayName: "Gemini 3.5 Flash Lite", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gemini"},
	{Pattern: "gemini-3.1-pro", DisplayName: "Gemini 3.1 Pro", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image", "audio", "video"}, Family: "gemini"},
	{Pattern: "gemini-3.1-flash-lite", DisplayName: "Gemini 3.1 Flash Lite", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gemini"},
	{Pattern: "gemini-3.1-flash-image", DisplayName: "Gemini 3.1 Flash Image", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "image-generation", "vision"}, Modalities: []string{"text", "image"}, Family: "gemini"},
	{Pattern: "gemini-3.1-flash-tts", DisplayName: "Gemini 3.1 Flash TTS", ContextLength: 1048576, Capabilities: []string{"tts"}, Modalities: []string{"text", "audio"}, Family: "gemini"},
	{Pattern: "gemini-3-flash", DisplayName: "Gemini 3 Flash", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image", "audio"}, Family: "gemini"},
	{Pattern: "gemini-3-pro-image", DisplayName: "Gemini 3 Pro Image", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "image-generation", "vision"}, Modalities: []string{"text", "image"}, Family: "gemini"},
	{Pattern: "gemini-2.5-pro", DisplayName: "Gemini 2.5 Pro", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image", "audio", "video"}, Family: "gemini"},
	{Pattern: "gemini-2.5-flash-lite", DisplayName: "Gemini 2.5 Flash Lite", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "gemini"},
	{Pattern: "gemini-2.5-flash-image", DisplayName: "Gemini 2.5 Flash Image", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "image-generation", "vision"}, Modalities: []string{"text", "image"}, Family: "gemini"},
	{Pattern: "gemini-2.5-flash", DisplayName: "Gemini 2.5 Flash", ContextLength: 1048576, MaxOutput: 65536, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image", "audio"}, Family: "gemini"},
	{Pattern: "gemini-2.0-flash", DisplayName: "Gemini 2.0 Flash", ContextLength: 1048576, MaxOutput: 8192, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image", "audio"}, Family: "gemini"},
	{Pattern: "gemini-embedding", DisplayName: "Gemini Embedding", ContextLength: 2048, Capabilities: []string{"embeddings"}, Modalities: []string{"text"}, Family: "gemini"},
	{Pattern: "gemma-4", DisplayName: "Gemma 4", ContextLength: 131072, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "gemma"},

	// DeepSeek
	{Pattern: "deepseek-v4-pro-max", DisplayName: "DeepSeek V4 Pro Max", ContextLength: 128000, MaxOutput: 16384, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "deepseek"},
	{Pattern: "deepseek-v4-pro", DisplayName: "DeepSeek V4 Pro", ContextLength: 128000, MaxOutput: 16384, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "deepseek"},
	{Pattern: "deepseek-v4-flash", DisplayName: "DeepSeek V4 Flash", ContextLength: 128000, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "deepseek"},
	{Pattern: "deepseek-v3", DisplayName: "DeepSeek V3", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "deepseek"},
	{Pattern: "deepseek-r1", DisplayName: "DeepSeek R1", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"chat", "reasoning", "code"}, Modalities: []string{"text"}, Family: "deepseek"},
	{Pattern: "deepseek-chat", DisplayName: "DeepSeek Chat", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "deepseek"},
	{Pattern: "deepseek-reasoner", DisplayName: "DeepSeek Reasoner", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"chat", "reasoning", "code"}, Modalities: []string{"text"}, Family: "deepseek"},

	// xAI Grok
	{Pattern: "grok-4", DisplayName: "Grok 4", ContextLength: 131072, MaxOutput: 32768, Capabilities: []string{"chat", "code", "reasoning", "vision"}, Modalities: []string{"text", "image"}, Family: "grok"},
	{Pattern: "grok-3", DisplayName: "Grok 3", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "grok"},
	{Pattern: "grok-code-fast", DisplayName: "Grok Code Fast", ContextLength: 131072, MaxOutput: 32768, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "grok"},

	// Qwen
	{Pattern: "qwen3.7-max", DisplayName: "Qwen 3.7 Max", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwen3.6", DisplayName: "Qwen 3.6", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwen3.5-plus", DisplayName: "Qwen 3.5 Plus", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwen3-coder", DisplayName: "Qwen3 Coder", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwen3-max", DisplayName: "Qwen3 Max", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwen3-235b", DisplayName: "Qwen3 235B", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwen3-32b", DisplayName: "Qwen3 32B", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "qwen"},
	{Pattern: "qwq", DisplayName: "QwQ", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "reasoning", "code"}, Modalities: []string{"text"}, Family: "qwen"},

	// Kimi (Moonshot)
	{Pattern: "kimi-k3", DisplayName: "Kimi K3", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "kimi"},
	{Pattern: "kimi-k2.7", DisplayName: "Kimi K2.7", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "kimi"},
	{Pattern: "kimi-k2.6", DisplayName: "Kimi K2.6", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "kimi"},
	{Pattern: "kimi-k2.5", DisplayName: "Kimi K2.5", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "kimi"},

	// GLM (Zhipu)
	{Pattern: "glm-5.2", DisplayName: "GLM 5.2", ContextLength: 128000, MaxOutput: 16384, Capabilities: []string{"chat", "code", "vision", "reasoning"}, Modalities: []string{"text", "image"}, Family: "glm"},
	{Pattern: "glm-5.1", DisplayName: "GLM 5.1", ContextLength: 128000, MaxOutput: 16384, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "glm"},
	{Pattern: "glm-5", DisplayName: "GLM 5", ContextLength: 128000, MaxOutput: 16384, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "glm"},
	{Pattern: "glm-4.7", DisplayName: "GLM 4.7", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "glm"},
	{Pattern: "glm-4", DisplayName: "GLM 4", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "glm"},

	// MiniMax
	{Pattern: "minimax-m3", DisplayName: "MiniMax M3", ContextLength: 1000000, MaxOutput: 16384, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "minimax"},
	{Pattern: "minimax-m2.7", DisplayName: "MiniMax M2.7", ContextLength: 1000000, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "minimax"},
	{Pattern: "minimax-m2.5", DisplayName: "MiniMax M2.5", ContextLength: 1000000, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "minimax"},
	{Pattern: "minimax-m2", DisplayName: "MiniMax M2", ContextLength: 1000000, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "minimax"},
	{Pattern: "codestral", DisplayName: "Codestral", ContextLength: 256000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "mistral"},
	{Pattern: "mistral-medium", DisplayName: "Mistral Medium", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "mistral"},
	{Pattern: "mistral-embed", DisplayName: "Mistral Embed", ContextLength: 8192, Capabilities: []string{"embeddings"}, Modalities: []string{"text"}, Family: "mistral"},

	// Meta Llama
	{Pattern: "llama-4", DisplayName: "Llama 4", ContextLength: 10000000, MaxOutput: 8192, Capabilities: []string{"chat", "code", "vision"}, Modalities: []string{"text", "image"}, Family: "llama"},
	{Pattern: "llama-3.3", DisplayName: "Llama 3.3", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "llama"},
	{Pattern: "llama-3", DisplayName: "Llama 3", ContextLength: 128000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "llama"},

	// Cohere
	{Pattern: "command-r-plus", DisplayName: "Command R+", ContextLength: 128000, MaxOutput: 4096, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "command"},
	{Pattern: "command-a", DisplayName: "Command A", ContextLength: 256000, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "command"},

	// NVIDIA Nemotron
	{Pattern: "nemotron", DisplayName: "Nemotron", ContextLength: 131072, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "nemotron"},

	// Seed (ByteDance)
	{Pattern: "seed-2", DisplayName: "Seed 2.0", ContextLength: 128000, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "seed"},

	// MiMo (Xiaomi)
	{Pattern: "mimo-v2.5-pro", DisplayName: "MiMo V2.5 Pro", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "mimo"},
	{Pattern: "mimo-v2.5", DisplayName: "MiMo V2.5", ContextLength: 131072, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "mimo"},
	{Pattern: "mimo-v2", DisplayName: "MiMo V2", ContextLength: 131072, MaxOutput: 8192, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "mimo"},

	// Poolside Laguna
	{Pattern: "laguna-s-2.1", DisplayName: "Laguna S 2.1", ContextLength: 1000000, MaxOutput: 32000, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "poolside"},
	{Pattern: "laguna-xs-2.1", DisplayName: "Laguna XS 2.1", ContextLength: 200000, MaxOutput: 32000, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "poolside"},
	{Pattern: "laguna", DisplayName: "Laguna", ContextLength: 200000, MaxOutput: 32000, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "poolside"},

	// Tencent Hunyuan
	{Pattern: "hunyuan-t1", DisplayName: "Hunyuan T1", ContextLength: 256000, MaxOutput: 16384, Capabilities: []string{"chat", "code", "reasoning"}, Modalities: []string{"text"}, Family: "hunyuan"},
	{Pattern: "hunyuan-turbos", DisplayName: "Hunyuan TurboS", ContextLength: 200000, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "hunyuan"},
	{Pattern: "hunyuan", DisplayName: "Hunyuan", ContextLength: 200000, MaxOutput: 16384, Capabilities: []string{"chat", "code"}, Modalities: []string{"text"}, Family: "hunyuan"},
}

// LookupCatalog finds the best matching static catalog entry for a model ID.
// Returns nil if no match found.
func LookupCatalog(modelID string) *ModelMetadata {
	lower := toLower(modelID)
	for i := range StaticCatalog {
		if containsStr(lower, toLower(StaticCatalog[i].Pattern)) {
			e := &StaticCatalog[i]
			name := e.DisplayName
			if name == "" {
				name = modelID
			}
			return &ModelMetadata{
				ID:            modelID,
				DisplayName:   name,
				ContextLength: e.ContextLength,
				MaxOutput:     e.MaxOutput,
				Capabilities:  e.Capabilities,
				Modalities:    e.Modalities,
				Family:        e.Family,
			}
		}
	}
	return nil
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
