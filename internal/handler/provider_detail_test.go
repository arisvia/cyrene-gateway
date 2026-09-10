package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

func TestGetProviderModels(t *testing.T) {
	srv, database := setupTestServer(t)

	conn := &model.ProviderConnection{
		ID:       "detail-conn",
		Provider: "anthropic",
		AuthType: "api-key",
		IsActive: true,
		Data:     model.ConnectionData{APIKey: "sk-ant-test"},
	}
	database.CreateConnection(conn)
	// Seed live cached models as Cyrene Gateway uses purely dynamic model caching
	cachedRaw := `{"provider":"anthropic","models":[{"id":"claude-3-5-sonnet","displayName":"Claude 3.5 Sonnet"}]}`
	database.KVSet("providerModelCache", "anthropic", cachedRaw)
	req := httptest.NewRequest("GET", "/api/providers/detail-conn/models", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["provider"] != "anthropic" {
		t.Fatalf("expected provider=anthropic, got %v", resp["provider"])
	}
	registryModels, ok := resp["registryModels"].([]any)
	if !ok || len(registryModels) == 0 {
		t.Fatalf("expected non-empty registryModels for anthropic, got %v", resp["registryModels"])
	}
	customModels, ok := resp["customModels"].([]any)
	if !ok {
		t.Fatalf("expected customModels to be an array, got %v", resp["customModels"])
	}
	if len(customModels) != 0 {
		t.Fatalf("expected empty customModels initially, got %v", customModels)
	}
}

func TestGetProviderModelsNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/providers/does-not-exist/models", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCustomModelCRUD(t *testing.T) {
	srv, database := setupTestServer(t)

	conn := &model.ProviderConnection{
		ID:       "crud-conn",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data:     model.ConnectionData{APIKey: "sk-test"},
	}
	database.CreateConnection(conn)

	// Add a custom model
	body := `{"id":"my-finetune-v1","name":"My Finetune"}`
	req := httptest.NewRequest("POST", "/api/providers/crud-conn/models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var addResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &addResp)
	custom := addResp["customModels"].([]any)
	if len(custom) != 1 {
		t.Fatalf("expected 1 custom model, got %d", len(custom))
	}

	// Duplicate add should conflict
	req = httptest.NewRequest("POST", "/api/providers/crud-conn/models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate, got %d", w.Code)
	}

	// Verify it shows up in GET
	req = httptest.NewRequest("GET", "/api/providers/crud-conn/models", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	var getResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &getResp)
	if len(getResp["customModels"].([]any)) != 1 {
		t.Fatalf("expected 1 custom model in GET, got %v", getResp["customModels"])
	}

	// Delete the custom model
	delBody := `{"id":"my-finetune-v1"}`
	req = httptest.NewRequest("DELETE", "/api/providers/crud-conn/models", strings.NewReader(delBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Delete non-existent model
	req = httptest.NewRequest("DELETE", "/api/providers/crud-conn/models", strings.NewReader(delBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing model, got %d", w.Code)
	}
}

func TestAddProviderModelValidation(t *testing.T) {
	srv, database := setupTestServer(t)

	conn := &model.ProviderConnection{
		ID:       "val-conn",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data:     model.ConnectionData{APIKey: "sk-test"},
	}
	database.CreateConnection(conn)

	// Missing id
	body := `{"name":"No ID"}`
	req := httptest.NewRequest("POST", "/api/providers/val-conn/models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing id, got %d", w.Code)
	}
}
func TestModelMetadataOverride(t *testing.T) {
	srv, database := setupTestServer(t)

	conn := &model.ProviderConnection{
		ID:       "meta-conn",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data:     model.ConnectionData{APIKey: "sk-test"},
	}
	database.CreateConnection(conn)
	cachedRaw := `{"provider":"openai","models":[{"id":"gpt-4o","displayName":"GPT-4o"}]}`
	database.KVSet("providerModelCache", "openai", cachedRaw)
	// Save custom metadata for gpt-4o
	metaBody := `{"id":"gpt-4o","displayName":"GPT-4o Custom","contextLength":128000,"maxOutputTokens":16384}`
	req := httptest.NewRequest("POST", "/api/providers/meta-conn/models/meta", strings.NewReader(metaBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 saving meta, got %d: %s", w.Code, w.Body.String())
	}

	// Verify handleGetProviderModels reflects the override
	req = httptest.NewRequest("GET", "/api/providers/meta-conn/models", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var detailResp struct {
		RegistryModels []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"contextLength"`
			MaxOutput     int    `json:"maxOutputTokens"`
			HasOverride   bool   `json:"hasOverride"`
			CanEdit       bool   `json:"canEdit"`
		} `json:"registryModels"`
	}
	json.Unmarshal(w.Body.Bytes(), &detailResp)
	var found bool
	for _, m := range detailResp.RegistryModels {
		if m.ID == "gpt-4o" {
			found = true
			if m.Name != "GPT-4o Custom" {
				t.Errorf("expected name 'GPT-4o Custom', got %q", m.Name)
			}
			if m.ContextLength != 128000 {
				t.Errorf("expected contextLength 128000, got %d", m.ContextLength)
			}
			if m.MaxOutput != 16384 {
				t.Errorf("expected maxOutput 16384, got %d", m.MaxOutput)
			}
			if !m.HasOverride {
				t.Errorf("expected hasOverride=true")
			}
			if !m.CanEdit {
				t.Errorf("expected canEdit=true for overridden model")
			}
			break
		}
	}
	if !found {
		t.Fatalf("gpt-4o not found in registry models")
	}

	// Verify /v1/models reflects the override
	req = httptest.NewRequest("GET", "/v1/models", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	var v1Resp struct {
		Data []struct {
			ID            string `json:"id"`
			DisplayName   string `json:"display_name"`
			ContextLength int    `json:"context_length"`
			MaxOutput     int    `json:"max_output_tokens"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &v1Resp)
	var v1Found bool
	for _, m := range v1Resp.Data {
		if m.ID == "openai/gpt-4o" {
			v1Found = true
			if m.DisplayName != "GPT-4o Custom" {
				t.Errorf("v1/models expected display_name 'GPT-4o Custom', got %q", m.DisplayName)
			}
			if m.ContextLength != 128000 {
				t.Errorf("v1/models expected context_length 128000, got %d", m.ContextLength)
			}
			break
		}
	}
	if !v1Found {
		t.Fatalf("openai/gpt-4o not found in v1/models")
	}

	// Reset meta override
	resetBody := `{"id":"gpt-4o"}`
	req = httptest.NewRequest("DELETE", "/api/providers/meta-conn/models/meta", strings.NewReader(resetBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 resetting meta, got %d", w.Code)
	}
}

func TestRefreshModels_CodeBuddy_LiveFailure(t *testing.T) {
	srv, database := setupTestServer(t)

	// Mock upstream server: return 404
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer ts.Close()
	origDevURL := model.ModelsDevURL
	model.ModelsDevURL = ts.URL + "/models.dev"
	defer func() { model.ModelsDevURL = origDevURL }()

	orig := provider.Registry["codebuddy-cn"]
	defer func() { provider.Registry["codebuddy-cn"] = orig }()

	mockInfo := orig
	mockInfo.BaseURL = ts.URL + "/v2/chat/completions"
	mockInfo.ModelsURL = ts.URL + "/v3/config"
	provider.Registry["codebuddy-cn"] = mockInfo

	conn := &model.ProviderConnection{
		ID:       "cb-conn-1",
		Provider: "codebuddy-cn",
		AuthType: "oauth",
		IsActive: true,
		Data: model.ConnectionData{
			AccessToken: "cb-mock-token",
		},
	}
	database.CreateConnection(conn)

	// Pre-seed a cache entry to verify failure doesn't overwrite it
	initialCache := `{"fetchedAt":"2026-09-10T12:00:00Z","models":[{"id":"existing-m1","displayName":"Existing M1"}]}`
	_ = database.KVSet("providerModelCache", "codebuddy-cn", initialCache)

	// POST /api/providers/codebuddy-cn/refresh-models
	// Live fetch fails with 404, without static fallback models it must return 502 Bad Gateway
	req := httptest.NewRequest("POST", "/api/providers/codebuddy-cn/refresh-models", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 Bad Gateway on live failure without static models, got %d: %s", w.Code, w.Body.String())
	}

	// Verify providerModelCache was NOT cleared or overwritten
	cachedRaw, err := database.KVGet("providerModelCache", "codebuddy-cn")
	if err != nil || cachedRaw != initialCache {
		t.Fatalf("expected providerModelCache to retain stale cache on failure, got %s", cachedRaw)
	}
}

func TestRefreshModels_CodeBuddy_LiveConfig(t *testing.T) {
	srv, database := setupTestServer(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/config" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"code": 0,
				"msg": "OK",
				"data": {
					"models": [
						{"id": "default", "name": "Default"},
						{"id": "deepseek-v4.1-flash", "name": "Deepseek-V4.1-Flash", "maxInputTokens": 1000000, "maxOutputTokens": 128000, "supportsImages": true, "supportsToolCall": true, "supportsReasoning": true},
						{"id": "minimax-m3", "name": "MiniMax-M3", "maxInputTokens": 512000, "maxOutputTokens": 128000, "supportsImages": true, "supportsToolCall": true, "supportsReasoning": true}
					]
				}
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	orig := provider.Registry["codebuddy-cn"]
	defer func() { provider.Registry["codebuddy-cn"] = orig }()

	mockInfo := orig
	mockInfo.BaseURL = ts.URL + "/v2/chat/completions"
	mockInfo.ModelsURL = ts.URL + "/v3/config"
	provider.Registry["codebuddy-cn"] = mockInfo

	conn := &model.ProviderConnection{
		ID:       "cb-live-conn",
		Provider: "codebuddy-cn",
		AuthType: "oauth",
		IsActive: true,
		Data: model.ConnectionData{
			AccessToken: "cb-live-token",
		},
	}
	database.CreateConnection(conn)

	req := httptest.NewRequest("POST", "/api/providers/codebuddy-cn/refresh-models", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on live config fetch, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		OK       bool                  `json:"ok"`
		Provider string                `json:"provider"`
		Count    int                   `json:"count"`
		Models   []model.ModelMetadata `json:"models"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Count != 2 || len(resp.Models) != 2 {
		t.Fatalf("expected 2 live models (skipping default), got count=%d, len=%d", resp.Count, len(resp.Models))
	}
	m0 := resp.Models[0]
	if m0.ID != "deepseek-v4.1-flash" || m0.ContextLength != 1000000 || m0.MaxOutput != 128000 {
		t.Errorf("unexpected m0 specs: %+v", m0)
	}

	cachedRaw, err := database.KVGet("providerModelCache", "codebuddy-cn")
	if err != nil || cachedRaw == "" {
		t.Fatalf("expected providerModelCache to be written in DB")
	}
}

func TestGetProviderModels_CodeBuddy_EmptyCache(t *testing.T) {
	srv, database := setupTestServer(t)

	conn := &model.ProviderConnection{
		ID:       "cb-conn-empty-cache",
		Provider: "codebuddy-cn",
		AuthType: "oauth",
		IsActive: true,
		Data: model.ConnectionData{
			AccessToken: "cb-mock-token",
		},
	}
	database.CreateConnection(conn)

	// GET /api/providers/cb-conn-empty-cache/models
	// Dynamic provider has no hardcoded registry models; when cache is empty, returns empty list.
	req := httptest.NewRequest("GET", "/api/providers/cb-conn-empty-cache/models", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Provider       string              `json:"provider"`
		RegistryModels []ProviderModelItem `json:"registryModels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.RegistryModels) != 0 {
		t.Fatalf("expected 0 fallback registry models for dynamic codebuddy-cn, got %d", len(resp.RegistryModels))
	}
}
