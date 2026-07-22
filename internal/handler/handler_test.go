package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/config"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

func setupTestServer(t *testing.T) (*Server, *db.DB) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	cfg := &config.Config{
		Host:    "127.0.0.1",
		Port:    0,
		DBPath:  ":memory:",
		DataDir: t.TempDir(),
	}
	srv := NewServer(database, cfg)
	return srv, database
}

func TestHealthEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Fatalf("expected ok=true, got %v", resp["ok"])
	}
	if resp["service"] != "cyrene-gateway" {
		t.Fatalf("expected service=cyrene-gateway, got %v", resp["service"])
	}
}

func TestModelsEndpoint(t *testing.T) {
	srv, database := setupTestServer(t)

	// Create a connection
	conn := &model.ProviderConnection{
		ID:       "test-conn",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data:     model.ConnectionData{APIKey: "sk-test"},
	}
	database.CreateConnection(conn)

	req := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["object"] != "list" {
		t.Fatalf("expected object=list, got %v", resp["object"])
	}
}

func TestChatCompletionsMissingModel(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestChatCompletionsNoCredentials(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"model":"openai/gpt-4","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestChatCompletionsWithMockUpstream(t *testing.T) {
	// Create mock upstream
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)

		resp := map[string]any{
			"id":     "chatcmpl-test",
			"object": "chat.completion",
			"model":  reqBody["model"],
			"choices": []any{
				map[string]any{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "Hello! How can I help you?",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 8,
				"total_tokens":      18,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	// Create a connection pointing to mock upstream
	conn := &model.ProviderConnection{
		ID:       "test-conn",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-test",
			BaseURL: mockUpstream.URL,
		},
	}
	database.CreateConnection(conn)

	body := `{"model":"openai/gpt-4","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	choices, ok := resp["choices"].([]any)
	if !ok || len(choices) == 0 {
		t.Fatalf("expected choices, got %v", resp)
	}

	choice := choices[0].(map[string]any)
	message := choice["message"].(map[string]any)
	if message["content"] != "Hello! How can I help you?" {
		t.Fatalf("unexpected content: %v", message["content"])
	}
}

func TestChatCompletionsStreaming(t *testing.T) {
	// Create mock upstream that streams SSE
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher := w.(http.Flusher)
		chunks := []string{
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		}
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	conn := &model.ProviderConnection{
		ID:       "test-conn",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-test",
			BaseURL: mockUpstream.URL,
		},
	}
	database.CreateConnection(conn)

	body := `{"model":"openai/gpt-4","messages":[{"role":"user","content":"hello"}],"stream":true}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	responseBody := w.Body.String()
	if !strings.Contains(responseBody, "data: [DONE]") {
		t.Fatalf("expected [DONE] in stream, got: %s", responseBody)
	}
	if !strings.Contains(responseBody, "Hello") {
		t.Fatalf("expected 'Hello' in stream, got: %s", responseBody)
	}
}

func TestMessagesEndpoint(t *testing.T) {
	// Create mock Anthropic upstream
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") == "" {
			t.Error("expected x-api-key header")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Error("expected anthropic-version header")
		}

		resp := map[string]any{
			"id":   "msg_test",
			"type": "message",
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "text", "text": "Hello from Claude!"},
			},
			"model":       "claude-sonnet-4-20250514",
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  10,
				"output_tokens": 5,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	conn := &model.ProviderConnection{
		ID:       "test-conn",
		Provider: "anthropic",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-ant-test",
			BaseURL: mockUpstream.URL,
		},
	}
	database.CreateConnection(conn)

	body := `{"model":"anthropic/claude-sonnet-4-20250514","messages":[{"role":"user","content":"hello"}],"max_tokens":1024}`
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["id"] != "msg_test" {
		t.Fatalf("expected id=msg_test, got %v", resp["id"])
	}
}

func TestCORSHeaders(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("expected CORS header Access-Control-Allow-Origin: *")
	}
}

func TestOptionsPreflight(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("OPTIONS", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS, got %d", w.Code)
	}
}

func TestDisabledModel(t *testing.T) {
	srv, database := setupTestServer(t)

	// Disable a model
	database.KVSet("disabledModels", "openai/gpt-4", "true")

	body := `{"model":"openai/gpt-4","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disabled model, got %d", w.Code)
	}
}

func TestDashboardServesHTML(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html content type, got %s", ct)
	}
	if !strings.Contains(w.Body.String(), "Cyrene") {
		t.Fatal("expected dashboard HTML content")
	}
}

func TestAuthStatusNoLoginRequired(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/auth/status", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["requireLogin"] != false {
		t.Fatalf("expected requireLogin=false, got %v", resp["requireLogin"])
	}
	if resp["authenticated"] != true {
		t.Fatalf("expected authenticated=true when login not required, got %v", resp["authenticated"])
	}
}

func TestLoginWithDefaultPassword(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"password":"123456"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Check that auth_token cookie is set
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "auth_token" && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected auth_token cookie to be set")
	}
}

func TestLoginWithWrongPassword(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"password":"wrongpass"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestLogout(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Check that auth_token cookie is cleared
	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "auth_token" && c.MaxAge != -1 {
			t.Fatal("expected auth_token cookie to be cleared")
		}
	}
}

func TestAPIKeyAuthEnforcement(t *testing.T) {
	srv, database := setupTestServer(t)

	// Enable requireApiKey
	settings, _ := database.GetSettings()
	settings.RequireAPIKey = true
	database.SaveSettings(settings)

	// Request without API key should be rejected
	body := `{"model":"openai/gpt-4","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without API key, got %d", w.Code)
	}
}

func TestAPIKeyAuthWithValidKey(t *testing.T) {
	srv, database := setupTestServer(t)

	// Enable requireApiKey
	settings, _ := database.GetSettings()
	settings.RequireAPIKey = true
	database.SaveSettings(settings)

	// Create an API key
	key := &model.APIKey{
		ID:       "test-key-id",
		Key:      "cg-testkey123",
		Name:     "test",
		IsActive: true,
	}
	database.CreateAPIKey(key)

	// Request with valid API key should pass through (will get 503 for no credentials, not 401)
	body := `{"model":"openai/gpt-4","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer cg-testkey123")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("expected non-401 with valid API key, got %d", w.Code)
	}
}

func TestDashboardAuthEnforcement(t *testing.T) {
	srv, database := setupTestServer(t)

	// Enable requireLogin
	settings, _ := database.GetSettings()
	settings.RequireLogin = true
	database.SaveSettings(settings)

	// Request to protected API without session should be rejected
	req := httptest.NewRequest("GET", "/api/providers", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", w.Code)
	}

	// Public paths should still work
	req = httptest.NewRequest("GET", "/api/health", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for public path, got %d", w.Code)
	}
}

func TestCreateAPIKeyEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"name":"my-key"}`
	req := httptest.NewRequest("POST", "/api/keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	key, ok := resp["key"].(string)
	if !ok || !strings.HasPrefix(key, "cg-") {
		t.Fatalf("expected cg- prefixed key, got %v", resp["key"])
	}
	if !strings.Contains(key, ".") {
		t.Fatalf("expected HMAC-signed key with dot separator, got %s", key)
	}
}

func TestHealthEndpointEnhanced(t *testing.T) {
	srv, database := setupTestServer(t)

	// Create a connection to verify connection count
	conn := &model.ProviderConnection{
		ID:       "health-test-conn",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data:     model.ConnectionData{APIKey: "sk-test"},
	}
	database.CreateConnection(conn)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["db"] != "ok" {
		t.Fatalf("expected db=ok, got %v", resp["db"])
	}
	if _, ok := resp["uptimeSeconds"]; !ok {
		t.Fatal("expected uptimeSeconds field in health response")
	}
	if resp["connections"] != float64(1) {
		t.Fatalf("expected connections=1, got %v", resp["connections"])
	}
	if resp["activeConnections"] != float64(1) {
		t.Fatalf("expected activeConnections=1, got %v", resp["activeConnections"])
	}
}

func TestProxyPoolCRUD(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Create
	body := `{"name":"test-proxy","proxyUrl":"http://127.0.0.1:7890","type":"http","strictProxy":true}`
	req := httptest.NewRequest("POST", "/api/proxy-pools", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var createResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &createResp)
	pool := createResp["proxyPool"].(map[string]any)
	poolID := pool["id"].(string)
	if poolID == "" {
		t.Fatal("expected non-empty pool id")
	}
	poolData := pool["data"].(map[string]any)
	if poolData["name"] != "test-proxy" {
		t.Fatalf("expected name=test-proxy, got %v", poolData["name"])
	}
	if poolData["proxyUrl"] != "http://127.0.0.1:7890" {
		t.Fatalf("expected proxyUrl, got %v", poolData["proxyUrl"])
	}

	// List
	req = httptest.NewRequest("GET", "/api/proxy-pools", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var listResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &listResp)
	pools := listResp["proxyPools"].([]any)
	if len(pools) != 1 {
		t.Fatalf("expected 1 pool, got %d", len(pools))
	}

	// Get by ID
	req = httptest.NewRequest("GET", "/api/proxy-pools/"+poolID, nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Update
	body = `{"name":"updated-proxy","isActive":false}`
	req = httptest.NewRequest("PUT", "/api/proxy-pools/"+poolID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var updateResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &updateResp)
	updatedPool := updateResp["proxyPool"].(map[string]any)
	updatedData := updatedPool["data"].(map[string]any)
	if updatedData["name"] != "updated-proxy" {
		t.Fatalf("expected name=updated-proxy, got %v", updatedData["name"])
	}
	if updatedPool["isActive"] != false {
		t.Fatalf("expected isActive=false, got %v", updatedPool["isActive"])
	}

	// Delete
	req = httptest.NewRequest("DELETE", "/api/proxy-pools/"+poolID, nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify deleted
	req = httptest.NewRequest("GET", "/api/proxy-pools/"+poolID, nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", w.Code)
	}
}

func TestProxyPoolCreateValidation(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Missing name
	body := `{"proxyUrl":"http://127.0.0.1:7890"}`
	req := httptest.NewRequest("POST", "/api/proxy-pools", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing name, got %d", w.Code)
	}

	// Missing proxyUrl
	body = `{"name":"test"}`
	req = httptest.NewRequest("POST", "/api/proxy-pools", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing proxyUrl, got %d", w.Code)
	}
}

func TestProxyPoolDeleteConflict(t *testing.T) {
	srv, database := setupTestServer(t)

	// Create a proxy pool
	body := `{"name":"bound-proxy","proxyUrl":"http://127.0.0.1:7890"}`
	req := httptest.NewRequest("POST", "/api/proxy-pools", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	var createResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &createResp)
	pool := createResp["proxyPool"].(map[string]any)
	poolID := pool["id"].(string)

	// Create a connection bound to this pool
	conn := &model.ProviderConnection{
		ID:       "bound-conn",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:               "sk-test",
			ProviderSpecificData: map[string]any{"proxyPoolId": poolID},
		},
	}
	database.CreateConnection(conn)

	// Attempt delete should return 409
	req = httptest.NewRequest("DELETE", "/api/proxy-pools/"+poolID, nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for in-use pool, got %d: %s", w.Code, w.Body.String())
	}
}
