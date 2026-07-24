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
