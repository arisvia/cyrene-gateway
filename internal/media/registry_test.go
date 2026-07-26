package media

import "testing"

func TestRegistryPopulated(t *testing.T) {
	if len(Registry) == 0 {
		t.Fatal("media registry is empty")
	}
}

func TestEmbeddingProviders(t *testing.T) {
	providers := GetProvidersByKind(KindEmbedding)
	if len(providers) < 10 {
		t.Errorf("expected at least 10 embedding providers, got %d", len(providers))
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
}

func TestTTSProviders(t *testing.T) {
	providers := GetProvidersByKind(KindTTS)
	if len(providers) < 5 {
		t.Errorf("expected at least 5 TTS providers, got %d", len(providers))
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
	if len(providers) < 4 {
		t.Errorf("expected at least 4 STT providers, got %d", len(providers))
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
	if len(providers) < 2 {
		t.Errorf("expected at least 2 video providers, got %d", len(providers))
	}

	if !SupportsKind("xai", KindVideo) {
		t.Error("xai should support video")
	}
}

func TestWebProviders(t *testing.T) {
	fetchProviders := GetProvidersByKind(KindWebFetch)
	if len(fetchProviders) < 3 {
		t.Errorf("expected at least 3 web fetch providers, got %d", len(fetchProviders))
	}

	searchProviders := GetProvidersByKind(KindWebSearch)
	if len(searchProviders) < 5 {
		t.Errorf("expected at least 5 web search providers, got %d", len(searchProviders))
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
