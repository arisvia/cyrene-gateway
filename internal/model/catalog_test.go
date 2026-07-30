package model

import "testing"

func TestLookupCatalog(t *testing.T) {
	tests := []struct {
		modelID    string
		wantName   string
		wantCtx    int
		wantFamily string
	}{
		{"gpt-5.4", "GPT-5.4", 256000, "gpt-5"},
		{"gpt-5.4-mini", "GPT-5.4 Mini", 256000, "gpt-5"},
		{"claude-sonnet-4-20250514", "Claude Sonnet 4", 200000, "claude"},
		{"gemini-2.5-pro", "Gemini 2.5 Pro", 1048576, "gemini"},
		{"deepseek-v4-pro", "DeepSeek V4 Pro", 128000, "deepseek"},
		{"kimi-k2.6", "Kimi K2.6", 131072, "kimi"},
		{"glm-5.2", "GLM 5", 128000, "glm"},
		{"o3", "O3", 200000, "o-series"},
		{"text-embedding-3-large", "Text Embedding 3 Large", 8191, "embedding"},
		{"unknown-model-xyz", "", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			got := LookupCatalog(tt.modelID)
			if tt.wantName == "" {
				if got != nil {
					t.Errorf("expected nil for %q, got %+v", tt.modelID, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected match for %q, got nil", tt.modelID)
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

func TestMergeMetadata(t *testing.T) {
	// Static catalog only
	meta := MergeMetadata("gpt-5.4", nil, nil)
	if meta.DisplayName != "GPT-5.4" {
		t.Errorf("expected GPT-5.4, got %q", meta.DisplayName)
	}
	if meta.ContextLength != 256000 {
		t.Errorf("expected 256000, got %d", meta.ContextLength)
	}

	// Live cache overrides static
	cached := &ModelMetadata{
		ID:            "gpt-5.4",
		DisplayName:   "GPT-5.4 (Live)",
		ContextLength: 300000,
	}
	meta = MergeMetadata("gpt-5.4", nil, cached)
	if meta.DisplayName != "GPT-5.4 (Live)" {
		t.Errorf("expected live override, got %q", meta.DisplayName)
	}
	if meta.ContextLength != 300000 {
		t.Errorf("expected 300000, got %d", meta.ContextLength)
	}

	// User override wins over all
	user := &ModelMetadata{
		ID:          "gpt-5.4",
		DisplayName: "My Custom Name",
	}
	meta = MergeMetadata("gpt-5.4", user, cached)
	if meta.DisplayName != "My Custom Name" {
		t.Errorf("expected user override, got %q", meta.DisplayName)
	}
	// Context from cache still applies (user didn't override it)
	if meta.ContextLength != 300000 {
		t.Errorf("expected 300000 from cache, got %d", meta.ContextLength)
	}

	// Unknown model falls back to ID as display name
	meta = MergeMetadata("totally-unknown-model", nil, nil)
	if meta.DisplayName != "totally-unknown-model" {
		t.Errorf("expected ID fallback, got %q", meta.DisplayName)
	}
}

func TestNormalizeOpenAICompat(t *testing.T) {
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
