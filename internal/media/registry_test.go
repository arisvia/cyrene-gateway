package media

import (
	"slices"
	"testing"
)

func TestRegistryPopulated(t *testing.T) {
	if len(Registry) == 0 {
		t.Fatal("media registry is empty")
	}
}

// Bare media ids are attributed by the inventory that advertises them, so a request
// for "dall-e-3" means OpenAI without the chat-side naming rule having to know what
// media vendors sell.
func TestProvidersForModel(t *testing.T) {
	if got := ProvidersForModel("dall-e-3", KindImage); !slices.Equal(got, []string{"openai"}) {
		t.Errorf("dall-e-3 image owners = %v, want [openai]", got)
	}
	if got := ProvidersForModel("DALL-E-3", KindImage); !slices.Equal(got, []string{"openai"}) {
		t.Errorf("lookup must ignore case, got %v", got)
	}
	if got := ProvidersForModel("dall-e-3", KindTTS); len(got) != 0 {
		t.Errorf("an image id must not resolve for another kind: %v", got)
	}
	if got := ProvidersForModel("whisper-1", KindSTT); !slices.Equal(got, []string{"openai"}) {
		t.Errorf("whisper-1 stt owners = %v, want [openai]", got)
	}
	if got := ProvidersForModel("model-nobody-sells", KindImage); len(got) != 0 {
		t.Errorf("unknown id matched an owner: %v", got)
	}
}

func TestEmbeddingProviders(t *testing.T) {
	providers := GetProvidersByKind(KindEmbedding)
	if len(providers) < 2 {
		t.Errorf("expected at least 2 embedding providers, got %d", len(providers))
	}

	// OpenAI should support embeddings
	if !SupportsKind("openai", KindEmbedding) {
		t.Error("openai should support embeddings")
	}

	cfg := GetConfig("openai", KindEmbedding)
	if cfg == nil {
		t.Fatal("openai embedding config is nil")
	}
	if cfg.Format != "openai" {
		t.Errorf("expected openai format, got %s", cfg.Format)
	}
	if cfg.BaseURL != "https://api.openai.com/v1/embeddings" {
		t.Errorf("unexpected base URL: %s", cfg.BaseURL)
	}

	// OpenRouter should support embeddings
	if !SupportsKind("openrouter", KindEmbedding) {
		t.Error("openrouter should support embeddings")
	}
}

func TestImageProviders(t *testing.T) {
	providers := GetProvidersByKind(KindImage)
	if len(providers) < 5 {
		t.Errorf("expected at least 5 image providers, got %d", len(providers))
	}

	if !SupportsKind("openai", KindImage) {
		t.Error("openai should support image generation")
	}
	if !SupportsKind("gemini", KindImage) {
		t.Error("gemini should support image generation")
	}
	if !SupportsKind("openrouter", KindImage) {
		t.Error("openrouter should support image generation")
	}
}

func TestTTSProviders(t *testing.T) {
	providers := GetProvidersByKind(KindTTS)
	if len(providers) < 2 {
		t.Errorf("expected at least 2 TTS providers, got %d", len(providers))
	}

	if !SupportsKind("openai", KindTTS) {
		t.Error("openai should support TTS")
	}
	if !SupportsKind("elevenlabs", KindTTS) {
		t.Error("elevenlabs should support TTS")
	}
}

func TestSTTProviders(t *testing.T) {
	providers := GetProvidersByKind(KindSTT)
	if len(providers) < 2 {
		t.Errorf("expected at least 2 STT providers, got %d", len(providers))
	}

	if !SupportsKind("openai", KindSTT) {
		t.Error("openai should support STT")
	}
	if !SupportsKind("deepgram", KindSTT) {
		t.Error("deepgram should support STT")
	}
}

func TestVideoProviders(t *testing.T) {
	providers := GetProvidersByKind(KindVideo)
	if len(providers) < 1 {
		t.Errorf("expected at least 1 video provider, got %d", len(providers))
	}

	if !SupportsKind("xai", KindVideo) {
		t.Error("xai should support video")
	}
}

func TestWebProviders(t *testing.T) {
	fetchProviders := GetProvidersByKind(KindWebFetch)
	if len(fetchProviders) < 2 {
		t.Errorf("expected at least 2 web fetch providers, got %d", len(fetchProviders))
	}

	searchProviders := GetProvidersByKind(KindWebSearch)
	if len(searchProviders) < 2 {
		t.Errorf("expected at least 2 web search providers, got %d", len(searchProviders))
	}
}

func TestMultiKindProvider(t *testing.T) {
	// OpenAI supports multiple kinds
	kinds := []Kind{KindEmbedding, KindImage, KindTTS, KindSTT}
	for _, k := range kinds {
		if !SupportsKind("openai", k) {
			t.Errorf("openai should support %s", k)
		}
	}

	// Gemini supports multiple kinds
	geminiKinds := []Kind{KindEmbedding, KindImage, KindTTS, KindSTT}
	for _, k := range geminiKinds {
		if !SupportsKind("gemini", k) {
			t.Errorf("gemini should support %s", k)
		}
	}
}

func TestGetConfigNil(t *testing.T) {
	cfg := GetConfig("nonexistent", KindEmbedding)
	if cfg != nil {
		t.Error("expected nil config for nonexistent provider")
	}

	cfg = GetConfig("openai", KindVideo)
	if cfg != nil {
		t.Error("expected nil config for unsupported kind")
	}
}

func TestPrunedProvidersAbsent(t *testing.T) {
	// fal, cartesia and assemblyai were re-added deliberately; they stay out of
	// this list so a re-prune is a conscious act again.
	removed := []string{
		"recraft", "runwayml", "playht",
		"huggingface", "searxng", "google-pse",
		"voyage-ai", "jina-ai", "jina-reader", "serper", "searchapi",
		"edge-tts",
	}
	for _, id := range removed {
		if _, ok := Registry[id]; ok {
			t.Errorf("pruned provider %q should not be present in Registry", id)
		}
	}
}
func TestTestCredentialsValidation(t *testing.T) {
	client := NewClient()

	// Missing token should fail immediately
	ok, _, err := client.TestCredentials(t.Context(), "elevenlabs", Credentials{}, "")
	if ok || err == nil {
		t.Error("expected error for empty token")
	}

	// Unsupported provider should fail
	ok, _, err = client.TestCredentials(t.Context(), "nonexistent", Credentials{APIKey: "foo"}, "")
	if ok || err == nil {
		t.Error("expected error for nonexistent provider")
	}
}

// The brand metadata table is applied by mutating pointers after registration.
// A change of Registry's value type, or of the loop to copy-by-value, would
// silently stop it landing — which is exactly what it did the first time.
func TestDisplayMetaAppliedToRegistry(t *testing.T) {
	if len(displayMeta) == 0 {
		t.Fatal("displayMeta is empty")
	}
	for id, m := range displayMeta {
		p := Registry[id]
		if p == nil {
			t.Errorf("displayMeta references unknown provider %q", id)
			continue
		}
		if p.Color != m.color || p.Website != m.website || p.APIKeyURL != m.apiKeyURL {
			t.Errorf("%s: metadata not applied: color=%q website=%q keyURL=%q", id, p.Color, p.Website, p.APIKeyURL)
		}
	}
}

// Providers that appear in the media registry without their own displayMeta
// entry: they are also chat providers, whose provider.ProviderInfo supplies the
// brand colour (the panel merges both sources), so duplicating it here would
// create a second source of truth for the same value.
var mediaProvidersWithoutOwnBrand = map[string]bool{
	"groq":        true,
	"xai":         true,
	"openai":      true,
	"openrouter":  true,
	"gemini":      true,
	"antigravity": true,
}

// Every other media provider must carry its own colour, so a new one cannot
// silently land as an initials-only card.
func TestMediaProvidersDeclareBrandMetadata(t *testing.T) {
	for id, p := range Registry {
		if mediaProvidersWithoutOwnBrand[id] {
			continue
		}
		if p.Color == "" {
			t.Errorf("provider %q has no colour and is not in mediaProvidersWithoutOwnBrand", id)
		}
	}
}
