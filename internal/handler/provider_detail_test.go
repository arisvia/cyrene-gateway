package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
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
