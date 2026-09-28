package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	req := newLocalRequest("GET", "/api/providers/detail-conn/models", nil)
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

func TestGetProviderModels_DisplayNameFallback(t *testing.T) {
	srv, database := setupTestServer(t)

	conn := &model.ProviderConnection{
		ID:       "qoder-conn",
		Provider: "qoder",
		AuthType: "oauth",
		IsActive: true,
	}
	database.CreateConnection(conn)

	// Qoder returns cryptic model ID "kmodel_latest" with display name "Kimi-K3" and zero context length
	cachedRaw := `{"provider":"qoder","models":[{"id":"kmodel_latest","display_name":"Kimi-K3"}]}`
	database.KVSet("providerModelCache", "qoder", cachedRaw)

	req := newLocalRequest("GET", "/api/providers/qoder-conn/models", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		RegistryModels []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"contextLength"`
			MaxOutput     int    `json:"maxOutputTokens"`
			CanEdit       bool   `json:"canEdit"`
		} `json:"registryModels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.RegistryModels) != 1 {
		t.Fatalf("expected 1 model, got %d", len(resp.RegistryModels))
	}
	m := resp.RegistryModels[0]
	if m.ID != "kmodel_latest" {
		t.Errorf("expected ID kmodel_latest, got %s", m.ID)
	}
	if m.Name != "Kimi-K3" {
		t.Errorf("expected Name Kimi-K3, got %s", m.Name)
	}
	if m.ContextLength != 1048576 {
		t.Errorf("expected ContextLength 1048576 via DisplayName fallback, got %d", m.ContextLength)
	}
	if m.MaxOutput != 131072 {
		t.Errorf("expected MaxOutput 131072 via DisplayName fallback, got %d", m.MaxOutput)
	}
	if !m.CanEdit {
		t.Errorf("expected CanEdit=true for fallback-matched model, got false")
	}
}

func TestGetProviderModelsNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := newLocalRequest("GET", "/api/providers/does-not-exist/models", nil)
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
	req := newLocalRequest("POST", "/api/providers/crud-conn/models", strings.NewReader(body))
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
	req = newLocalRequest("POST", "/api/providers/crud-conn/models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate, got %d", w.Code)
	}

	// Verify it shows up in GET
	req = newLocalRequest("GET", "/api/providers/crud-conn/models", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	var getResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &getResp)
	if len(getResp["customModels"].([]any)) != 1 {
		t.Fatalf("expected 1 custom model in GET, got %v", getResp["customModels"])
	}

	// Delete the custom model
	delBody := `{"id":"my-finetune-v1"}`
	req = newLocalRequest("DELETE", "/api/providers/crud-conn/models", strings.NewReader(delBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Delete non-existent model
	req = newLocalRequest("DELETE", "/api/providers/crud-conn/models", strings.NewReader(delBody))
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
	req := newLocalRequest("POST", "/api/providers/val-conn/models", strings.NewReader(body))
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
	req := newLocalRequest("POST", "/api/providers/meta-conn/models/meta", strings.NewReader(metaBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 saving meta, got %d: %s", w.Code, w.Body.String())
	}

	// Verify handleGetProviderModels reflects the override
	req = newLocalRequest("GET", "/api/providers/meta-conn/models", nil)
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
	req = newLocalRequest("DELETE", "/api/providers/meta-conn/models/meta", strings.NewReader(resetBody))
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
	req := newLocalRequest("POST", "/api/providers/codebuddy-cn/refresh-models", nil)
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

	req := newLocalRequest("POST", "/api/providers/codebuddy-cn/refresh-models", nil)
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

func TestRefreshModels_CodeBuddyIntl_LiveConfig(t *testing.T) {
	srv, database := setupTestServer(t)
	defer database.Close()

	origDevURL := model.ModelsDevURL
	model.ModelsDevURL = ""
	defer func() { model.ModelsDevURL = origDevURL }()

	var receivedUA, receivedDomain string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/config" {
			receivedUA = r.Header.Get("User-Agent")
			receivedDomain = r.Header.Get("X-Domain")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"code": 0,
				"data": {
					"agents": [{"name": "cli", "models": ["hy4-preview", "kimi-k3"]}],
					"models": [
						{"id": "default", "name": "Default"},
						{"id": "hy4-preview", "name": "Hunyuan 4", "maxInputTokens": 1000000, "maxOutputTokens": 64000, "supportsImages": true, "supportsToolCall": true},
						{"id": "kimi-k3", "name": "Kimi K3", "maxInputTokens": 1000000, "maxOutputTokens": 32000, "supportsToolCall": true}
					]
				}
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	orig := provider.Registry["codebuddy"]
	defer func() { provider.Registry["codebuddy"] = orig }()

	mockInfo := orig
	mockInfo.BaseURL = ts.URL + "/v2/chat/completions"
	mockInfo.ModelsURL = ts.URL + "/v3/config"
	provider.Registry["codebuddy"] = mockInfo

	conn := &model.ProviderConnection{
		ID:       "cb-intl-live-conn",
		Provider: "codebuddy",
		AuthType: "oauth",
		IsActive: true,
		Data: model.ConnectionData{
			AccessToken: "cb-intl-token",
		},
	}
	database.CreateConnection(conn)

	req := newLocalRequest("POST", "/api/providers/codebuddy/refresh-models", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on live config fetch, got %d: %s", w.Code, w.Body.String())
	}
	if receivedUA != "CLI/2.108.1 CodeBuddy/2.108.1" {
		t.Errorf("expected User-Agent CLI/2.108.1 CodeBuddy/2.108.1, got %q", receivedUA)
	}
	if receivedDomain != "www.codebuddy.ai" {
		t.Errorf("expected X-Domain www.codebuddy.ai, got %q", receivedDomain)
	}

	var resp struct {
		OK     bool                  `json:"ok"`
		Count  int                   `json:"count"`
		Models []model.ModelMetadata `json:"models"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Count != 2 || len(resp.Models) != 2 {
		t.Fatalf("expected 2 models, got count=%d, len=%d", resp.Count, len(resp.Models))
	}

	// Verify GET /api/providers/{id}/models returns CanEdit=false (locked) for upstream CodeBuddy models
	getReq := newLocalRequest("GET", "/api/providers/cb-intl-live-conn/models", nil)
	getW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", getW.Code, getW.Body.String())
	}
	var detailResp struct {
		RegistryModels []struct {
			ID      string `json:"id"`
			CanEdit bool   `json:"canEdit"`
		} `json:"registryModels"`
	}
	if err := json.Unmarshal(getW.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("failed to decode detail response: %v", err)
	}
	if len(detailResp.RegistryModels) == 0 {
		t.Fatalf("expected models in detail response")
	}
	for _, m := range detailResp.RegistryModels {
		if m.CanEdit {
			t.Errorf("model %s should have CanEdit=false (locked) because CodeBuddy upstream provided explicit metadata", m.ID)
		}
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
	req := newLocalRequest("GET", "/api/providers/cb-conn-empty-cache/models", nil)
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
func TestGetProviderModels_UpstreamFreeFlag(t *testing.T) {
	srv, database := setupTestServer(t)

	conn := &model.ProviderConnection{
		ID:       "qoder-conn-free",
		Provider: "qoder",
		AuthType: "oauth",
		IsActive: true,
		Data:     model.ConnectionData{AccessToken: "dt-mock"},
	}
	database.CreateConnection(conn)

	// Qoder's live catalog sends an explicit price_factor 0 for free models, which
	// omitempty erases; the parser records that as a free flag instead.
	database.KVSet("providerModelCache", "qoder", `{"fetchedAt":"2026-09-28T00:00:00Z","models":[`+
		`{"id":"qfmodel","display_name":"Qwen3.8-Flash","price_factor":0,"is_free":true},`+
		`{"id":"cmodel","display_name":"Cantus","price_factor":4}]}`)

	req := newLocalRequest("GET", "/api/providers/qoder-conn-free/models", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		RegistryModels []ProviderModelItem `json:"registryModels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	free, paid := ProviderModelItem{ID: "unset"}, ProviderModelItem{ID: "unset"}
	for _, m := range resp.RegistryModels {
		switch m.ID {
		case "qfmodel":
			free = m
		case "cmodel":
			paid = m
		}
	}
	if !free.IsFree {
		t.Errorf("model with explicit upstream price_factor 0 must be free: %+v", free)
	}
	if paid.IsFree {
		t.Errorf("priced model must not be marked free: %+v", paid)
	}
	if paid.PriceFactor != 4 {
		t.Errorf("expected paid PriceFactor 4, got %f", paid.PriceFactor)
	}
}

func TestGetProviderModels_CachedPriceFactor(t *testing.T) {
	srv, database := setupTestServer(t)

	conn := &model.ProviderConnection{
		ID:       "qoder-conn-pf",
		Provider: "qoder",
		AuthType: "oauth",
		IsActive: true,
		Data: model.ConnectionData{
			AccessToken: "dt-mock",
		},
	}
	database.CreateConnection(conn)

	cached := model.CachedModels{
		FetchedAt: time.Now().UTC(),
		Models: []model.ModelMetadata{
			{ID: "smodel", DisplayName: "Sonus", PriceFactor: 8.0},
			{ID: "qmodel", DisplayName: "Qwen3.7-Plus", PriceFactor: 0.1},
		},
	}
	data, _ := json.Marshal(cached)
	database.KVSet("providerModelCache", "qoder", string(data))

	req := newLocalRequest("GET", "/api/providers/qoder-conn-pf/models", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		RegistryModels []ProviderModelItem `json:"registryModels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.RegistryModels) != 2 {
		t.Fatalf("expected 2 models, got %d", len(resp.RegistryModels))
	}
	for _, m := range resp.RegistryModels {
		if m.ID == "smodel" && m.PriceFactor != 8.0 {
			t.Errorf("expected Sonus PriceFactor 8.0, got %f", m.PriceFactor)
		}
		if m.ID == "qmodel" && m.PriceFactor != 0.1 {
			t.Errorf("expected Qwen3.7-Plus PriceFactor 0.1, got %f", m.PriceFactor)
		}
	}
}
