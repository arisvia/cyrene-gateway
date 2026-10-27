package provider

import (
	"slices"
	"strings"
	"testing"
)

// TestRegistryCompleteness pins the curated KEEP set (Phase 36 T1): 31 chat
// providers across curated coding agents, direct vendor APIs, E2E-verified
// aggregators/free tiers, and brand pairs. Registry IDs are stable.
func TestRegistryCompleteness(t *testing.T) {
	expectedProviders := []string{
		// Curated coding providers (12)
		"claude", "github", "grok-cli", "cursor", "qoder",
		"codebuddy", "kimi", "antigravity", "opencode", "openrouter",
		"glm", "glm-cn",
		"anthropic", "openai", "gemini", "vertex", "tencent",
		"xai", "alicode", "alicode-cn",
		// E2E-verified (4)
		"deepseek", "cerebras", "groq", "nvidia",
		// Brand pairs / quota-ported (5)
		"codebuddy-cn", "minimax", "minimax-cn", "opencode-go", "qoder-cn",
		"custom-openai", "custom-anthropic",
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
		"apikey":   14,
		"oauth":    11,
		"freeTier": 4,
		"custom":   2,
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
		validCats := map[string]bool{"apikey": true, "oauth": true, "freeTier": true, "custom": true}
		if !validCats[p.Category] {
			t.Errorf("Provider %q has invalid Category %q", id, p.Category)
		}
		if p.APIType == "" {
			t.Errorf("Provider %q has empty APIType", id)
		}
		validAPI := map[string]bool{"openai": true, "anthropic": true, "gemini": true, "responses": true}
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
		"Alibaba":   {"alicode", "alicode-cn"},
		"CodeBuddy": {"codebuddy", "codebuddy-cn"},
		"OpenCode":  {"opencode", "opencode-go"},
		"Qoder":     {"qoder", "qoder-cn"},
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
		"grok-cli":     "Grok Build",
		"kimi":         "Kimi Code",
		"antigravity":  "Antigravity",
		"codebuddy":    "CodeBuddy (Intl)",
		"codebuddy-cn": "CodeBuddy (CN)",
		"cursor":       "Cursor",
		"opencode":     "OpenCode",
		"opencode-go":  "OpenCode Go",
		"claude":       "Claude Code",
		"github":       "GitHub Copilot",
		"alicode":      "Alibaba (Intl)",
		"alicode-cn":   "Alibaba (CN)",
		// 区域拆分提供商统一：国际 = 官方产品线名 + (Intl)，国内 = 产品线名 + (CN)。
		"glm":        "GLM Coding (Intl)",
		"glm-cn":     "GLM (CN)",
		"minimax":    "MiniMax Coding (Intl)",
		"minimax-cn": "MiniMax (CN)",
		"qoder":      "Qoder (Intl)",
		"qoder-cn":   "Qoder (CN)",
	}
	// 任何区域型提供商都必须按该规范收尾，新增者自动受检；
	// Region 还用于同品牌非地域变体（opencode 的 zen/go），那类不在此列。
	for id, p := range Registry {
		var want string
		switch p.Region {
		case "intl":
			want = " (Intl)"
		case "cn":
			want = " (CN)"
		default:
			continue
		}
		if !strings.HasSuffix(p.Name, want) {
			t.Errorf("Provider %q Name = %q, want it to end with %q", id, p.Name, want)
		}
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
	for _, id := range []string{"claude", "qoder"} {
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

func TestOpenAIDualAuth(t *testing.T) {
	p, ok := Registry["openai"]
	if !ok {
		t.Fatal("Missing provider openai")
	}
	if !slices.Contains(p.AuthModes, "oauth") || !slices.Contains(p.AuthModes, "api-key") {
		t.Errorf("openai AuthModes = %v, want both oauth and api-key", p.AuthModes)
	}
	if p.OAuthBaseURL != "https://chatgpt.com/backend-api/codex/responses" || p.OAuthAPIType != "responses" {
		t.Errorf("openai OAuth endpoint mismatch: %s / %s", p.OAuthBaseURL, p.OAuthAPIType)
	}

	// Test EffectiveBaseURL
	uAPI, fmtAPI := p.EffectiveBaseURL("api-key", true)
	if uAPI != p.BaseURL || fmtAPI != p.APIType {
		t.Errorf("EffectiveBaseURL(api-key) = (%s, %s), want (%s, %s)", uAPI, fmtAPI, p.BaseURL, p.APIType)
	}
	uOAuth, fmtOAuth := p.EffectiveBaseURL("oauth", false)
	if uOAuth != p.OAuthBaseURL || fmtOAuth != p.OAuthAPIType {
		t.Errorf("EffectiveBaseURL(oauth) = (%s, %s), want (%s, %s)", uOAuth, fmtOAuth, p.OAuthBaseURL, p.OAuthAPIType)
	}
}

func TestRegistryOAuthProviders(t *testing.T) {
	// Providers with standard OAuth token endpoints
	oauthWithToken := []string{"claude", "github", "kimi"}
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
		validFormats := map[string]bool{"openai": true, "anthropic": true, "gemini": true, "responses": true}
		if !validFormats[p.APIType] {
			t.Errorf("Provider %q has invalid APIType/format %q", id, p.APIType)
		}
	}
}

// TestRegistryAliasesBrevityAndUniqueness verifies that provider aliases are
// strictly concise (<= 5 chars), non-colliding, and resolve correctly.
func TestRegistryAliasesBrevityAndUniqueness(t *testing.T) {
	seenAliases := make(map[string]string)

	for id, p := range Registry {
		// Canonical ID must resolve to itself
		if got := ResolveProviderAlias(id); got != id {
			t.Errorf("ResolveProviderAlias(%q) = %q, want %q", id, got, id)
		}

		if p.Alias == "" {
			continue
		}

		// Brevity rule: shorthand alias must be 5 chars or fewer (e.g. cbcn, qdcn)
		if len(p.Alias) > 5 {
			t.Errorf("Provider %q has alias %q which exceeds 5 characters", id, p.Alias)
		}

		// Shorthand must not collide with another canonical ID
		if other, exists := Registry[p.Alias]; exists && p.Alias != id {
			t.Errorf("Alias %q of provider %q collides with canonical provider %q", p.Alias, id, other.ID)
		}

		// Uniqueness: no two providers may claim the same alias
		if prev, exists := seenAliases[p.Alias]; exists {
			t.Errorf("Alias %q is claimed by both %q and %q", p.Alias, prev, id)
		}
		seenAliases[p.Alias] = id

		// Alias must resolve to canonical ID
		if got := ResolveProviderAlias(p.Alias); got != id {
			t.Errorf("ResolveProviderAlias(%q) = %q, want %q", p.Alias, got, id)
		}
	}
}

// TestGetProviderIsCanonicalOnly pins where a shorthand may enter: aliases are
// resolved inside ParseModel, where a client types "qd/qmodel", and nowhere
// else. Everything past that point is a canonical id, so the registry lookup
// must reject a shorthand rather than quietly accepting it and letting the
// alias become a second identity for the same provider.
func TestGetProviderIsCanonicalOnly(t *testing.T) {
	for id, p := range Registry {
		if _, ok := GetProvider(id); !ok {
			t.Fatalf("GetProvider(%q) missed its own canonical record", id)
		}
		if p.Alias == "" || p.Alias == id {
			continue
		}
		if got, ok := GetProvider(p.Alias); ok {
			t.Errorf("GetProvider(%q) resolved an alias to %q, want a miss", p.Alias, got.ID)
		}
	}

	// The client-facing form still resolves, which is the alias's only purpose.
	if got := ParseModel("oai/gpt-4o"); got.Provider != "openai" || got.Model != "gpt-4o" {
		t.Errorf("ParseModel(oai/gpt-4o) = %+v, want provider openai and the bare model", got)
	}
	if got := ParseModel("openai/gpt-4o"); got.Provider != "openai" {
		t.Errorf("ParseModel with a canonical prefix = %+v, want provider openai", got)
	}

	// Unknown ids stay unknown: no fuzzy or prefix matching.
	if _, ok := GetProvider("no-such-provider"); ok {
		t.Error("GetProvider resolved an unknown provider")
	}
	if got := ResolveProviderAlias("no-such-provider"); got != "no-such-provider" {
		t.Errorf("ResolveProviderAlias rewrote an unknown id to %q", got)
	}
}

// sanitizeCodebuddyRequest handles the two regional shapes explicitly, so a new
// codebuddy-* sibling would silently lose system-prompt handling. This keeps that
// drift in the registry where the provider is added instead of deep in the executor.
func TestRegistryCoversEveryCodebuddyShape(t *testing.T) {
	for id := range Registry {
		if !strings.HasPrefix(id, "codebuddy") {
			continue
		}
		switch id {
		case "codebuddy", "codebuddy-cn":
		default:
			t.Errorf("provider %q needs an explicit branch in sanitizeCodebuddyRequest", id)
		}
	}
}
