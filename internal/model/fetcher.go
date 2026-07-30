package model

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CacheTTL is how long live-fetched model metadata stays valid.
const CacheTTL = 24 * time.Hour

// CachedModels is the JSON structure stored in KV (scope="providerModelCache", key=providerID).
type CachedModels struct {
	FetchedAt time.Time       `json:"fetched_at"`
	Models    []ModelMetadata `json:"models"`
}

// IsExpired returns true if the cache is older than TTL.
func (c *CachedModels) IsExpired() bool {
	return time.Since(c.FetchedAt) > CacheTTL
}

// FetchModels fetches the model list from a provider's /models endpoint and
// normalizes the response into []ModelMetadata. The providerID determines
// which normalizer to use.
func FetchModels(client *http.Client, providerID, baseURL, apiKey, accessToken string) ([]ModelMetadata, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("no base URL for provider %s", providerID)
	}

	url := strings.TrimRight(baseURL, "/") + "/models"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Set auth
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	} else if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("fetch models: status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read models response: %w", err)
	}

	return normalizeModels(providerID, body)
}

// normalizeModels dispatches to provider-specific normalizers.
func normalizeModels(providerID string, body []byte) ([]ModelMetadata, error) {
	switch {
	case providerID == "openrouter":
		return normalizeOpenRouter(body)
	case providerID == "anthropic" || providerID == "claude":
		return normalizeAnthropic(body)
	case strings.HasPrefix(providerID, "gemini") || providerID == "vertex":
		return normalizeGoogle(body)
	case providerID == "mistral":
		return normalizeMistral(body)
	default:
		return normalizeOpenAICompat(body)
	}
}

// normalizeOpenAICompat handles the standard OpenAI /v1/models response format.
func normalizeOpenAICompat(body []byte) ([]ModelMetadata, error) {
	var resp struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse openai models: %w", err)
	}

	models := make([]ModelMetadata, 0, len(resp.Data))
	for _, m := range resp.Data {
		meta := ModelMetadata{ID: m.ID}
		// Enrich from static catalog
		if cat := LookupCatalog(m.ID); cat != nil {
			meta.DisplayName = cat.DisplayName
			meta.ContextLength = cat.ContextLength
			meta.MaxOutput = cat.MaxOutput
			meta.Capabilities = cat.Capabilities
			meta.Modalities = cat.Modalities
			meta.Family = cat.Family
		}
		models = append(models, meta)
	}
	return models, nil
}

// normalizeOpenRouter handles OpenRouter's /api/v1/models with rich metadata.
func normalizeOpenRouter(body []byte) ([]ModelMetadata, error) {
	var resp struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"context_length"`
			Architecture  struct {
				Modality string `json:"modality"`
			} `json:"architecture"`
			TopProvider struct {
				MaxCompletionTokens int `json:"max_completion_tokens"`
			} `json:"top_provider"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse openrouter models: %w", err)
	}

	models := make([]ModelMetadata, 0, len(resp.Data))
	for _, m := range resp.Data {
		meta := ModelMetadata{
			ID:            m.ID,
			DisplayName:   m.Name,
			ContextLength: m.ContextLength,
			MaxOutput:     m.TopProvider.MaxCompletionTokens,
		}

		// Parse modality string like "text+image->text"
		if m.Architecture.Modality != "" {
			parts := strings.SplitN(m.Architecture.Modality, "->", 2)
			if len(parts) == 2 {
				meta.Modalities = parseModalities(parts[0] + "+" + parts[1])
			}
		}

		// Infer capabilities
		meta.Capabilities = inferCapabilities(m.ID, meta.Modalities)

		models = append(models, meta)
	}
	return models, nil
}

// normalizeAnthropic handles Anthropic's /v1/models response.
func normalizeAnthropic(body []byte) ([]ModelMetadata, error) {
	var resp struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			Type        string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		// Fallback to OpenAI compat
		return normalizeOpenAICompat(body)
	}

	models := make([]ModelMetadata, 0, len(resp.Data))
	for _, m := range resp.Data {
		meta := ModelMetadata{
			ID:          m.ID,
			DisplayName: m.DisplayName,
		}
		// Enrich from catalog
		if cat := LookupCatalog(m.ID); cat != nil {
			if meta.DisplayName == "" {
				meta.DisplayName = cat.DisplayName
			}
			meta.ContextLength = cat.ContextLength
			meta.MaxOutput = cat.MaxOutput
			meta.Capabilities = cat.Capabilities
			meta.Modalities = cat.Modalities
			meta.Family = cat.Family
		}
		models = append(models, meta)
	}
	return models, nil
}

// normalizeGoogle handles Google's models list response.
func normalizeGoogle(body []byte) ([]ModelMetadata, error) {
	var resp struct {
		Models []struct {
			Name                       string   `json:"name"`
			DisplayName                string   `json:"displayName"`
			InputTokenLimit            int      `json:"inputTokenLimit"`
			OutputTokenLimit           int      `json:"outputTokenLimit"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return normalizeOpenAICompat(body)
	}

	models := make([]ModelMetadata, 0, len(resp.Models))
	for _, m := range resp.Models {
		// Google model names are like "models/gemini-2.5-pro"
		id := strings.TrimPrefix(m.Name, "models/")
		meta := ModelMetadata{
			ID:            id,
			DisplayName:   m.DisplayName,
			ContextLength: m.InputTokenLimit,
			MaxOutput:     m.OutputTokenLimit,
		}

		// Map generation methods to capabilities
		for _, method := range m.SupportedGenerationMethods {
			switch method {
			case "generateContent":
				meta.Capabilities = append(meta.Capabilities, "chat")
			case "embedContent":
				meta.Capabilities = append(meta.Capabilities, "embeddings")
			case "countTokens":
				// informational only
			}
		}
		if len(meta.Capabilities) == 0 {
			meta.Capabilities = []string{"chat"}
		}

		meta.Modalities = []string{"text", "image"}
		meta.Family = "gemini"

		models = append(models, meta)
	}
	return models, nil
}

// normalizeMistral handles Mistral's /v1/models response with capabilities.
func normalizeMistral(body []byte) ([]ModelMetadata, error) {
	var resp struct {
		Data []struct {
			ID           string `json:"id"`
			Object       string `json:"object"`
			Capabilities struct {
				CompletionChat  bool `json:"completion_chat"`
				CompletionFIM   bool `json:"completion_fim"`
				FunctionCalling bool `json:"function_calling"`
				FineTuning      bool `json:"fine_tuning"`
				Vision          bool `json:"vision"`
				Classification  bool `json:"classification"`
			} `json:"capabilities"`
			MaxContextLength int `json:"max_context_length"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return normalizeOpenAICompat(body)
	}

	models := make([]ModelMetadata, 0, len(resp.Data))
	for _, m := range resp.Data {
		meta := ModelMetadata{
			ID:            m.ID,
			ContextLength: m.MaxContextLength,
			Family:        "mistral",
		}

		if m.Capabilities.CompletionChat {
			meta.Capabilities = append(meta.Capabilities, "chat")
		}
		if m.Capabilities.CompletionFIM {
			meta.Capabilities = append(meta.Capabilities, "code")
		}
		if m.Capabilities.FunctionCalling {
			meta.Capabilities = append(meta.Capabilities, "tools")
		}
		if m.Capabilities.Vision {
			meta.Capabilities = append(meta.Capabilities, "vision")
			meta.Modalities = []string{"text", "image"}
		} else {
			meta.Modalities = []string{"text"}
		}
		if len(meta.Capabilities) == 0 {
			meta.Capabilities = []string{"chat"}
		}

		// Enrich display name from catalog
		if cat := LookupCatalog(m.ID); cat != nil {
			meta.DisplayName = cat.DisplayName
		}

		models = append(models, meta)
	}
	return models, nil
}

// MergeMetadata implements the three-layer merge:
// user override > live cache > static catalog > ID inference.
func MergeMetadata(modelID string, userOverride *ModelMetadata, cached *ModelMetadata) ModelMetadata {
	// Start with static catalog
	result := ModelMetadata{ID: modelID}
	if cat := LookupCatalog(modelID); cat != nil {
		result = *cat
		result.ID = modelID
	}

	// Layer 2: live cache overrides
	if cached != nil {
		if cached.DisplayName != "" {
			result.DisplayName = cached.DisplayName
		}
		if cached.ContextLength > 0 {
			result.ContextLength = cached.ContextLength
		}
		if cached.MaxOutput > 0 {
			result.MaxOutput = cached.MaxOutput
		}
		if len(cached.Capabilities) > 0 {
			result.Capabilities = cached.Capabilities
		}
		if len(cached.Modalities) > 0 {
			result.Modalities = cached.Modalities
		}
		if cached.Family != "" {
			result.Family = cached.Family
		}
	}

	// Layer 3: user override (highest priority)
	if userOverride != nil {
		if userOverride.DisplayName != "" {
			result.DisplayName = userOverride.DisplayName
		}
		if userOverride.ContextLength > 0 {
			result.ContextLength = userOverride.ContextLength
		}
		if userOverride.MaxOutput > 0 {
			result.MaxOutput = userOverride.MaxOutput
		}
		if len(userOverride.Capabilities) > 0 {
			result.Capabilities = userOverride.Capabilities
		}
		if len(userOverride.Modalities) > 0 {
			result.Modalities = userOverride.Modalities
		}
		if userOverride.Family != "" {
			result.Family = userOverride.Family
		}
	}

	// Ensure display name fallback
	if result.DisplayName == "" {
		result.DisplayName = modelID
	}

	return result
}

func parseModalities(s string) []string {
	var mods []string
	for _, part := range strings.Split(s, "+") {
		part = strings.TrimSpace(part)
		if part != "" {
			mods = append(mods, part)
		}
	}
	return mods
}

func inferCapabilities(modelID string, modalities []string) []string {
	lower := toLower(modelID)
	caps := []string{"chat"}
	if containsStr(lower, "embed") {
		return []string{"embeddings"}
	}
	if containsStr(lower, "image") || containsStr(lower, "flux") || containsStr(lower, "dall") {
		caps = append(caps, "image-generation")
	}
	if containsStr(lower, "reason") || containsStr(lower, "thinking") {
		caps = append(caps, "reasoning")
	}
	for _, m := range modalities {
		if m == "image" && !containsStr(lower, "image") {
			caps = append(caps, "vision")
			break
		}
	}
	return caps
}
