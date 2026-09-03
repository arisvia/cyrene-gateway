package provider

import "testing"

// TestRegistryCompleteness pins the curated KEEP set (Phase 36 T1): 31 chat
// providers across curated coding agents, direct vendor APIs, E2E-verified
// aggregators/free tiers, and brand pairs. Registry IDs are stable.
func TestRegistryCompleteness(t *testing.T) {
	expectedProviders := []string{
		// Curated coding providers (13)
		"claude", "codex", "github", "grok-cli", "cursor", "qoder",
		"codebuddy-intl", "kimi", "antigravity", "opencode", "openrouter",
		"glm", "glm-cn",
		// Direct vendor APIs (8)
		"anthropic", "openai", "gemini", "vertex", "tencent",
		"xai", "alicode-intl", "alicode",
		// E2E-verified (4)
		"deepseek", "cerebras", "groq", "nvidia",
		// Brand pairs / quota-ported (3)
		"codebuddy-cn", "minimax", "minimax-cn",
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
		"apikey":   12,
		"oauth":    11,
		"freeTier": 4,
		"free":     1,
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

// TestRegistryBrandRegion verifies Phase 36 T3 brand grouping metadata: every
// sibling pair carries the same Brand and a distinct Region.
func TestRegistryBrandRegion(t *testing.T) {
	brands := map[string][]string{
		"GLM":       {"glm", "glm-cn"},
		"MiniMax":   {"minimax", "minimax-cn"},
		"Alibaba":   {"alicode-intl", "alicode"},
	}
	for brand, ids := range brands {
		for _, id := range ids {
			p, ok := Registry[id]
			if !ok {
				t.Errorf("Missing provider %q", id)
				continue
			}
			if p.Brand != brand {
				t.Errorf("Provider %q has Brand %q, want %q", id, p.Brand, brand)
			}
			if p.Region == "" {
				t.Errorf("Provider %q has empty Region", id)
			}
		}
	}
}

// TestRegistryOfficialNames verifies Phase 36 T2 vendor-official display names.
func TestRegistryOfficialNames(t *testing.T) {
	names := map[string]string{
		"grok-cli":       "Grok Build",
		"kimi":           "Kimi Code",
		"antigravity":    "Google Antigravity",
		"codebuddy-intl": "CodeBuddy (Intl)",
		"codebuddy-cn":   "CodeBuddy (CN)",
		"cursor":         "Cursor",
		"opencode":       "OpenCode",
		"claude":         "Claude Code",
		"codex":          "OpenAI Codex",
		"github":         "GitHub Copilot",
		"openrouter":     "OpenRouter",
		"alicode":        "Alibaba (China)",
		"alicode-intl":   "Alibaba (Intl)",
	}
	for id, want := range names {
		p, ok := Registry[id]
		if !ok {
			t.Errorf("Missing provider %q", id)
			continue
		}
		if p.Name != want {
			t.Errorf("Provider %q Name = %q, want %q", id, p.Name, want)
		}
	}
}

// TestRegistryDualAuth verifies Phase 36 T4/T5 dual-auth entries: claude,
// codex and qoder advertise [oauth, apikey]; qoder moved free→oauth.
func TestRegistryDualAuth(t *testing.T) {
	for _, id := range []string{"claude", "codex", "qoder"} {
		p, ok := Registry[id]
		if !ok {
			t.Errorf("Missing provider %q", id)
			continue
		}
		if p.Category != "oauth" {
			t.Errorf("Provider %q Category = %q, want oauth", id, p.Category)
		}
		hasOAuth, hasAPIKey := false, false
		for _, m := range p.AuthModes {
			if m == "oauth" {
				hasOAuth = true
			}
			if m == "apikey" || m == "api-key" {
				hasAPIKey = true
			}
		}
		if !hasOAuth || !hasAPIKey {
			t.Errorf("Provider %q AuthModes = %v, want [oauth, apikey]", id, p.AuthModes)
		}
	}
	if p := Registry["qoder"]; p.AuthHint == "" {
		t.Error("qoder should carry a PAT auth hint")
	}
}

func TestRegistryOAuthProviders(t *testing.T) {
	// Providers with standard OAuth token endpoints
	oauthWithToken := []string{"claude", "github", "kimi", "codex"}
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
// runtime-resolved URLs are exempt from the HTTP URL check but must still
// have a format.
func TestRegistryTransportCompleteness(t *testing.T) {
	exempt := map[string]bool{
		"antigravity": true, // resolved at runtime via OAuth
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
