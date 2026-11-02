package provider

import "testing"

// Every chat provider must either expose a live catalog or ship a curated list,
// otherwise the model picker stays empty forever.
func TestRegistryCatalogsResolvable(t *testing.T) {
	for id, p := range Registry {
		switch id {
		case "antigravity", "qoder", "qoder-cn", "claude", "cursor", "github":
			// These have bespoke catalog fetchers in internal/handler (project
			// discovery, COSY signing, Copilot endpoints) — no generic URL.
			continue
		}
		if p.Category == "custom" {
			continue // user-supplied base URL, catalog derives from it
		}
		hasLive := p.ModelsURL != "" || BuildModelsURL(p.BaseURL) != ""
		hasStatic := len(p.Models) > 0
		if !hasLive && !hasStatic {
			t.Errorf("provider %q has neither a models URL nor static models", id)
		}
	}
}

// The media catalog must not double a /models segment: gemini stores the
// catalog as its base, so a naive append requests .../models/models.
func TestDeriveModelsURLNoDoubleModels(t *testing.T) {
	cases := map[string]string{
		"https://generativelanguage.googleapis.com/v1beta/models": "https://generativelanguage.googleapis.com/v1beta/models",
		"https://api.openai.com/v1/chat/completions":              "https://api.openai.com/v1/models",
		"https://api.mistral.ai/v1/chat/completions":              "https://api.mistral.ai/v1/models",
	}
	for in, want := range cases {
		if got := BuildModelsURL(in); got != want {
			t.Errorf("BuildModelsURL(%q) = %q, want %q", in, got, want)
		}
	}
}
