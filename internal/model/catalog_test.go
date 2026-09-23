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
		{"deepseek-v4-pro", "DeepSeek V4 Pro", 1000000, "deepseek"},
		{"kimi-k2.6", "Kimi K2.6", 131072, "kimi"},
		{"glm-5.2", "GLM 5.2", 1000000, "glm"},
		{"o3", "O3", 200000, "o-series"},
		{"text-embedding-3-large", "Text Embedding 3 Large", 8191, "embedding"},
		{"deepseek-v4.1-flash", "DeepSeek V4.1 Flash", 1000000, "deepseek"},
		{"claude-opus-4", "Claude Opus 4", 200000, "claude"},
		{"kimi-k2.7-code", "Kimi K2.7 Code", 262144, "kimi"},
		{"gpt-oss-120b", "GPT-OSS 120B", 131072, "openai"},
		{"qwen3.8-max", "Qwen 3.8 Max", 1048576, "qwen"},
		{"hy4-preview", "Hy4 Preview", 1048576, "hunyuan"},
		{"hy3", "Hy3", 262144, "hunyuan"},
		{"grok-4-fast", "Grok 4 Fast", 2000000, "grok"},
		{"mistral-large-3", "Mistral Large 3", 262144, "mistral"},
		{"llama-4-scout", "Llama 4 Scout", 328000, "llama"},
		{"command-r-plus", "Command R+", 128000, "command"},
		{"hunyuan-pro", "Hunyuan Pro", 256000, "hunyuan"},
		{"seed-2.0-pro", "Doubao Seed 2.0 Pro", 256000, "seed"},
		{"qwen2.5-coder-32b", "Qwen 2.5 Coder 32B", 131072, "qwen"},
		{"claude-3-7-sonnet", "Claude 3.7 Sonnet", 200000, "claude"},
		{"gpt-4o", "GPT-4o", 128000, "gpt-4o"},
		{"sonar-reasoning-pro", "Sonar Reasoning Pro", 128000, "perplexity"},
		{"nova-2-lite", "Nova 2 Lite", 1000000, "nova"},
		{"doubao-pro-128k", "Doubao Pro 128K", 128000, "seed"},
		{"gemini-2.0-flash", "Gemini 2.0 Flash", 1048576, "gemini"},
		{"step-3.7-flash", "Step 3.7 Flash", 262144, "step"},
		{"dall-e-3", "DALL-E 3", 0, "image"},
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

func TestLookupCatalog_DisplayNameFallback(t *testing.T) {
	tests := []struct {
		id          string
		displayName string
		wantCtx     int
		wantOutput  int
		wantFamily  string
	}{
		{"kmodel_latest", "Kimi-K3", 1048576, 131072, "kimi"},
		{"kmodel", "Kimi-K2.8-Preview", 262144, 262144, "kimi"},
		{"dmodel", "DeepSeek-V4-Pro", 1000000, 384000, "deepseek"},
		{"dfmodel", "DeepSeek-V4-Flash", 1000000, 384000, "deepseek"},
		{"gmodel", "GLM-5.3", 1000000, 131072, "glm"},
		{"gfmodel", "GLM-5.3-Flash", 1310720, 131072, "glm"},
		{"mmodel", "MiniMax-M3", 1048576, 512000, "minimax"},
		{"qfmodel", "Qwen3.8-Flash", 1048576, 131072, "qwen"},
		{"qmodel_latest", "Qwen3.7-Max", 131072, 16384, "qwen"},
		{"qmodel", "Qwen3.7-Plus", 131072, 16384, "qwen"},
	}
	for _, tt := range tests {
		t.Run(tt.displayName, func(t *testing.T) {
			got := LookupCatalog(tt.id, tt.displayName)
			if got == nil {
				t.Fatalf("expected match for %s (%s), got nil", tt.id, tt.displayName)
			}
			if got.ContextLength != tt.wantCtx {
				t.Errorf("ContextLength = %d, want %d", got.ContextLength, tt.wantCtx)
			}
			if got.MaxOutput != tt.wantOutput {
				t.Errorf("MaxOutput = %d, want %d", got.MaxOutput, tt.wantOutput)
			}
			if got.Family != tt.wantFamily {
				t.Errorf("Family = %q, want %q", got.Family, tt.wantFamily)
			}
		})
	}
}

func TestEnrichModelsFromCatalog(t *testing.T) {
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
		t.Errorf("unknown model should not be modified: %+v", models[1])
	}
	if models[2].ContextLength != 50000 || models[2].MaxOutput != 4000 {
		t.Errorf("existing values should not be overwritten: %+v", models[2])
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

	// Test Layer 2.5: Fallback to Catalog using DisplayName when ID is opaque
	cachedQoder := &ModelMetadata{
		ID:          "kmodel_latest",
		DisplayName: "Kimi-K3",
	}
	meta = MergeMetadata("kmodel_latest", nil, cachedQoder)
	if meta.ContextLength != 1048576 {
		t.Errorf("expected 1048576 via DisplayName fallback, got %d", meta.ContextLength)
	}
	if meta.MaxOutput != 131072 {
		t.Errorf("expected 131072 via DisplayName fallback, got %d", meta.MaxOutput)
	}
	if meta.Family != "kimi" {
		t.Errorf("expected family kimi, got %q", meta.Family)
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
