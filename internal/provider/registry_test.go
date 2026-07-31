package provider

import "testing"

func TestRegistryCompleteness(t *testing.T) {
	// All 112 providers from 9router registry must be present
	expectedProviders := []string{
		"alicode-intl", "alicode", "alims-intl", "anthropic", "antigravity",
		"api-airforce", "assemblyai", "aws-polly", "azure", "baidu",
		"bazaarlink", "black-forest-labs", "blackbox", "bluesminds", "brave-search",
		"byteplus", "cartesia", "cerebras", "chutes", "claude",
		"cline", "clinepass", "cloudflare-ai", "codebuddy-cn", "codebuddy-intl", "codex",
		"cohere", "comfyui", "commandcode", "coqui", "cursor",
		"deepgram", "deepseek", "devin-cli", "edge-tts", "elevenlabs", "exa",
		"fal-ai", "featherless", "firecrawl", "fireworks", "gemini-cli",
		"gemini", "github", "gitlab", "glm-cn", "glm",
		"google-pse", "google-tts", "grok-cli", "grok-web", "groq",
		"huggingface", "hyperbolic", "iflow", "inworld", "jina-ai",
		"jina-reader", "kilo-gateway", "kilocode", "kimchi", "kimi",
		"kiro", "linkup", "llm7", "local-device", "mimo-free",
		"minimax-cn", "minimax", "mistral", "mmf", "morph",
		"nanobanana", "nebius", "nvidia", "ollama-local", "ollama",
		"openai", "opencode-go", "opencode", "openrouter", "perplexity-agent",
		"perplexity-web", "perplexity", "playht", "poolside", "qoder",
		"qwen", "recraft", "runwayml", "sambanova", "sdwebui",
		"searchapi", "searxng", "serper", "siliconflow", "stability-ai",
		"tavily", "tencent", "together", "topaz", "tortoise",
		"venice", "vercel-ai-gateway", "vertex-partner", "vertex", "volcengine-ark",
		"voyage-ai", "xai", "xiaomi-mimo", "xiaomi-tokenplan", "youcom",
	}

	for _, id := range expectedProviders {
		if _, ok := Registry[id]; !ok {
			t.Errorf("Registry missing provider %q", id)
		}
	}

	if len(Registry) != len(expectedProviders) {
		t.Errorf("Registry has %d providers, expected %d", len(Registry), len(expectedProviders))
	}
}

func TestRegistryCategories(t *testing.T) {
	cats := GetRegistryByCategory()

	expectedCats := map[string]int{
		"apikey":    70,
		"oauth":     18,
		"freeTier":  17,
		"free":      5,
		"webCookie": 2,
	}

	for _, c := range cats {
		expected, ok := expectedCats[c.Category]
		if !ok {
			t.Errorf("Unexpected category %q", c.Category)
			continue
		}
		if c.Count != expected {
			t.Errorf("Category %q has %d providers, expected %d", c.Category, c.Count, expected)
		}
		if len(c.Providers) != c.Count {
			t.Errorf("Category %q Count=%d but Providers len=%d", c.Category, c.Count, len(c.Providers))
		}
	}

	if len(cats) != len(expectedCats) {
		t.Errorf("Got %d categories, expected %d", len(cats), len(expectedCats))
	}
}

func TestRegistryProviderFields(t *testing.T) {
	for id, p := range Registry {
		if p.ID != id {
			t.Errorf("Provider %q has mismatched ID field %q", id, p.ID)
		}
		if p.Name == "" {
			t.Errorf("Provider %q has empty Name", id)
		}
		if p.Category == "" {
			t.Errorf("Provider %q has empty Category", id)
		}
		validCats := map[string]bool{"apikey": true, "oauth": true, "freeTier": true, "free": true, "webCookie": true}
		if !validCats[p.Category] {
			t.Errorf("Provider %q has invalid Category %q", id, p.Category)
		}
		if p.APIType == "" {
			t.Errorf("Provider %q has empty APIType", id)
		}
		validAPI := map[string]bool{"openai": true, "anthropic": true, "gemini": true}
		if !validAPI[p.APIType] {
			t.Errorf("Provider %q has invalid APIType %q", id, p.APIType)
		}
	}
}

func TestRegistryOAuthProviders(t *testing.T) {
	// Providers with standard OAuth token endpoints
	oauthWithToken := []string{"claude", "github", "kimi", "qwen", "codex"}
	for _, id := range oauthWithToken {
		p, ok := Registry[id]
		if !ok {
			t.Errorf("Missing OAuth provider %q", id)
			continue
		}
		if p.Category != "oauth" {
			t.Errorf("Provider %q should be oauth category, got %q", id, p.Category)
		}
		if p.TokenURL == "" && p.DeviceCodeURL == "" {
			t.Errorf("OAuth provider %q has no TokenURL or DeviceCodeURL", id)
		}
	}

	// Cursor uses token import (IDE local storage), not standard OAuth endpoints
	p, ok := Registry["cursor"]
	if !ok {
		t.Fatal("Missing provider cursor")
	}
	if p.Category != "oauth" {
		t.Errorf("cursor should be oauth category, got %q", p.Category)
	}
}

// TestRegistryTransportCompleteness verifies every registry entry has a
// non-empty BaseURL and a valid APIType (format). Providers with special
// local/sentinel URLs (e.g. "edge-tts", "local-device") are exempt from
// the HTTP URL check but must still have a format.
func TestRegistryTransportCompleteness(t *testing.T) {
	// Providers with empty or sentinel BaseURL that are still valid.
	exempt := map[string]bool{
		"azure":       true, // user-supplied resource URL
		"antigravity": true, // resolved at runtime via OAuth
		"searxng":     true, // user-supplied instance URL
		"topaz":       true, // local desktop app
		"grok-cli":    true, // resolved at runtime via OAuth
	}

	for id, p := range Registry {
		if p.BaseURL == "" && !exempt[id] {
			t.Errorf("Provider %q has empty BaseURL and is not exempt", id)
		}
		validFormats := map[string]bool{"openai": true, "anthropic": true, "gemini": true}
		if !validFormats[p.APIType] {
			t.Errorf("Provider %q has invalid APIType/format %q", id, p.APIType)
		}
	}
}
