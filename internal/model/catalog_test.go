package model

import "testing"

// seedModelsDev installs a models.dev catalog for the duration of a test and
// restores the previous value on cleanup. The static curated table was removed:
// every "guess missing metadata" path now goes exclusively through models.dev,
// so the catalog tests exercise that layer directly.
func seedModelsDev(t *testing.T, entries map[string]ModelsDevEntry) {
	t.Helper()
	prev := GetModelsDevCatalog()
	SetModelsDevCatalog(entries)
	t.Cleanup(func() { SetModelsDevCatalog(prev) })
}

func devFixture() map[string]ModelsDevEntry {
	return map[string]ModelsDevEntry{
		"gpt-5.4":                {Name: "GPT-5.4", Family: "gpt-5", Context: 256000, Output: 32768},
		"gpt-5.4-mini":           {Name: "GPT-5.4 Mini", Family: "gpt-5", Context: 256000, Output: 16384},
		"claude-sonnet-4":        {Name: "Claude Sonnet 4", Family: "claude", Context: 200000, Output: 64000},
		"gemini-2.5-pro":         {Name: "Gemini 2.5 Pro", Family: "gemini", Context: 1048576, Output: 65536},
		"deepseek-v4-pro":        {Name: "DeepSeek V4 Pro", Family: "deepseek", Context: 1000000, Output: 384000},
		"o3":                     {Name: "O3", Family: "o-series", Context: 200000, Output: 100000},
		"text-embedding-3-large": {Name: "Text Embedding 3 Large", Family: "embedding", Context: 8191},
	}
}

func TestLookupCatalog(t *testing.T) {
	seedModelsDev(t, devFixture())

	tests := []struct {
		id         string
		wantName   string
		wantCtx    int
		wantFamily string
	}{
		{"gpt-5.4", "GPT-5.4", 256000, "gpt-5"},
		{"gpt-5.4-mini", "GPT-5.4 Mini", 256000, "gpt-5"},
		// id not a key, but contains the seeded "claude-sonnet-4" -> longest-fuzzy hit
		{"claude-sonnet-4-20250514", "Claude Sonnet 4", 200000, "claude"},
		{"gemini-2.5-pro", "Gemini 2.5 Pro", 1048576, "gemini"},
		{"deepseek-v4-pro", "DeepSeek V4 Pro", 1000000, "deepseek"},
		{"o3", "O3", 200000, "o-series"},
		{"text-embedding-3-large", "Text Embedding 3 Large", 8191, "embedding"},
		// provider prefix is stripped before matching
		{"openai/gpt-5.4", "GPT-5.4", 256000, "gpt-5"},
		{"totally-unknown-model", "", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := LookupCatalog(tt.id)
			if tt.wantName == "" {
				if got != nil {
					t.Errorf("expected nil for %q, got %+v", tt.id, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected match for %q, got nil", tt.id)
			}
			if got.DisplayName != tt.wantName {
				t.Errorf("DisplayName = %q, want %q", got.DisplayName, tt.wantName)
			}
			if got.ContextLength != tt.wantCtx {
				t.Errorf("ContextLength = %d, want %d", got.ContextLength, tt.wantCtx)
			}
			if got.Family != tt.wantFamily {
				t.Errorf("Family = %q, want %q", got.Family, tt.wantFamily)
			}
		})
	}
}

// TestLookupCatalog_DisplayNameFallback proves the core of the manual-fill policy:
// an opaque provider id (Qoder's "kmodel_latest", "qmodel_latest") that models.dev
// cannot resolve by key is still resolved by its human-readable display NAME.
func TestLookupCatalog_DisplayNameFallback(t *testing.T) {
	seedModelsDev(t, map[string]ModelsDevEntry{
		// Key intentionally differs from Name so only the name pass can match.
		"moonshot-k2": {Name: "Kimi K3", Family: "kimi", Context: 1048576, Output: 131072},
		"qwen4-max":   {Name: "Qwen 4 Max", Family: "qwen", Context: 1048576, Output: 131072},
	})

	// Opaque id misses every key match; the display name "Kimi-K3" matches entry Name.
	got := LookupCatalog("kmodel_latest", "Kimi-K3")
	if got == nil {
		t.Fatalf("expected display-name match for kmodel_latest (Kimi-K3), got nil")
	}
	if got.ID != "kmodel_latest" {
		t.Errorf("expected ID preserved as kmodel_latest, got %q", got.ID)
	}
	if got.ContextLength != 1048576 || got.MaxOutput != 131072 || got.Family != "kimi" {
		t.Errorf("unexpected enrich: %+v", got)
	}

	// A floating "-preview" alias still fuzzy-matches the seeded key by display name.
	if got := LookupCatalog("qmodel_latest", "Qwen4-Max-Preview"); got == nil || got.ContextLength != 1048576 {
		t.Errorf("expected Qwen4-Max-Preview to resolve via models.dev, got %+v", got)
	}

	// With neither id nor name present upstream, nothing is guessed.
	if got := LookupCatalog("cmodel", ""); got != nil {
		t.Errorf("expected nil for unknown opaque id, got %+v", got)
	}
}

func TestEnrichModelsFromCatalog(t *testing.T) {
	seedModelsDev(t, map[string]ModelsDevEntry{
		"z": {Name: "Kimi K3", Family: "kimi", Context: 1048576, Output: 131072},
	})
	models := []ModelMetadata{
		{ID: "kmodel_latest", DisplayName: "Kimi-K3"},
		{ID: "cmodel", DisplayName: "Cantus"},
		{ID: "already_set", DisplayName: "Kimi-K3", ContextLength: 50000, MaxOutput: 4000},
	}
	EnrichModelsFromCatalog(models)
	if models[0].ContextLength != 1048576 || models[0].MaxOutput != 131072 || models[0].Family != "kimi" {
		t.Errorf("failed to enrich models[0]: %+v", models[0])
	}
	if models[1].ContextLength != 0 {
		t.Errorf("model absent from models.dev should stay blank for manual entry: %+v", models[1])
	}
	if models[2].ContextLength != 50000 || models[2].MaxOutput != 4000 {
		t.Errorf("existing values should not be overwritten: %+v", models[2])
	}
}

func TestMergeMetadata(t *testing.T) {
	seedModelsDev(t, map[string]ModelsDevEntry{
		"gpt-5.4":     {Name: "GPT-5.4", Family: "gpt-5", Context: 256000, Output: 32768},
		"moonshot-k2": {Name: "Kimi K3", Family: "kimi", Context: 1048576, Output: 131072},
	})

	// models.dev resolution (no static table anymore).
	meta := MergeMetadata("gpt-5.4", nil, nil)
	if meta.DisplayName != "GPT-5.4" || meta.ContextLength != 256000 {
		t.Errorf("expected models.dev GPT-5.4/256000, got %+v", meta)
	}

	// Live cache overrides models.dev.
	cached := &ModelMetadata{ID: "gpt-5.4", DisplayName: "GPT-5.4 (Live)", ContextLength: 300000}
	meta = MergeMetadata("gpt-5.4", nil, cached)
	if meta.DisplayName != "GPT-5.4 (Live)" || meta.ContextLength != 300000 {
		t.Errorf("expected live override, got %+v", meta)
	}

	// User override wins over all.
	user := &ModelMetadata{ID: "gpt-5.4", DisplayName: "My Custom Name"}
	meta = MergeMetadata("gpt-5.4", user, cached)
	if meta.DisplayName != "My Custom Name" || meta.ContextLength != 300000 {
		t.Errorf("expected user override name + cache context, got %+v", meta)
	}

	// Unknown model falls back to ID as display name.
	meta = MergeMetadata("totally-unknown-model", nil, nil)
	if meta.DisplayName != "totally-unknown-model" {
		t.Errorf("expected ID fallback, got %q", meta.DisplayName)
	}

	// Layer 2.5: opaque ID resolved via display name against models.dev.
	cachedQoder := &ModelMetadata{ID: "kmodel_latest", DisplayName: "Kimi-K3"}
	meta = MergeMetadata("kmodel_latest", nil, cachedQoder)
	if meta.ContextLength != 1048576 || meta.MaxOutput != 131072 || meta.Family != "kimi" {
		t.Errorf("expected 1048576/131072/kimi via DisplayName models.dev fallback, got %+v", meta)
	}
}

func TestNormalizeOpenAICompat(t *testing.T) {
	seedModelsDev(t, map[string]ModelsDevEntry{
		"gpt-5.4": {Name: "GPT-5.4", Family: "gpt-5", Context: 256000, Output: 32768},
	})
	body := []byte(`{"data":[{"id":"gpt-5.4","object":"model","owned_by":"openai"},{"id":"custom-model","object":"model","owned_by":"user"}]}`)
	models, err := normalizeOpenAICompat(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].DisplayName != "GPT-5.4" {
		t.Errorf("expected enriched display name, got %q", models[0].DisplayName)
	}
	if models[1].ID != "custom-model" {
		t.Errorf("expected custom-model, got %q", models[1].ID)
	}
}

func TestNormalizeOpenRouter(t *testing.T) {
	// OpenRouter supplies explicit limits, so FromUpstream metadata is used and
	// no catalog lookup is required.
	body := []byte(`{"data":[{"id":"openai/gpt-5.4","name":"GPT-5.4","context_length":256000,"architecture":{"modality":"text+image->text"},"top_provider":{"max_completion_tokens":32768}}]}`)
	models, err := normalizeOpenRouter(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	m := models[0]
	if m.DisplayName != "GPT-5.4" {
		t.Errorf("expected GPT-5.4, got %q", m.DisplayName)
	}
	if m.ContextLength != 256000 {
		t.Errorf("expected 256000, got %d", m.ContextLength)
	}
	if m.MaxOutput != 32768 {
		t.Errorf("expected 32768, got %d", m.MaxOutput)
	}
}
