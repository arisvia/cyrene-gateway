package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestAPIKey_ModelWhitelistAndContextInjection(t *testing.T) {
	srv, database := setupTestServer(t)

	// Create an API key with whitelist: ["deepseek/*"] and systemPrompt: "KeyContext:Test"
	keyStr := auth.GenerateAPIKey()
	key := &model.APIKey{
		ID:            "key-whitelist-1",
		Key:           keyStr,
		Name:          "DeepSeek-Only Key",
		IsActive:      true,
		AllowedModels: []string{"deepseek/*"},
		SystemPrompt:  "KeyContext:DeepSeekOnly",
	}
	if err := database.CreateAPIKey(key); err != nil {
		t.Fatalf("create key: %v", err)
	}

	// 1. Request disallowed model -> 403 Forbidden
	bodyDisallowed := `{"model":"openai/gpt-4o","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(bodyDisallowed))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+keyStr)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("disallowed model: want 403, got %d (body: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not allowed for this API key") {
		t.Fatalf("expected error message mentioning not allowed, got %s", w.Body.String())
	}

	// 2. Request allowed model -> Passes whitelist (mock gets downstream or 503 for no provider connection, NOT 403)
	bodyAllowed := `{"model":"deepseek/deepseek-chat","messages":[{"role":"user","content":"hello"}]}`
	req = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(bodyAllowed))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+keyStr)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusForbidden {
		t.Fatalf("allowed model should not get 403: got %d, body: %s", w.Code, w.Body.String())
	}

	// 3. Test PUT /api/keys/{id} to update whitelist and RPM
	updatePayload := `{"name":"Updated DeepSeek Key","allowedModels":["deepseek/*","openai/gpt-4o"],"rpm":50}`
	req = httptest.NewRequest("PUT", "/api/keys/"+key.ID, strings.NewReader(updatePayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/keys/{id}: want 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	var updatedKey model.APIKey
	if err := json.Unmarshal(w.Body.Bytes(), &updatedKey); err != nil {
		t.Fatalf("unmarshal updated key: %v", err)
	}
	if updatedKey.Name != "Updated DeepSeek Key" || updatedKey.RPM != 50 || len(updatedKey.AllowedModels) != 2 {
		t.Fatalf("updated key fields incorrect: %+v", updatedKey)
	}

	// 4. Now openai/gpt-4o is allowed!
	req = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(bodyDisallowed))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+keyStr)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusForbidden {
		t.Fatalf("after whitelist update, gpt-4o should be allowed: got 403, body: %s", w.Body.String())
	}
}

func TestAPIKey_ContextInjectionDiffersInCache(t *testing.T) {
	srv, database := setupTestServer(t)

	// Mock upstream server that returns messages echoed
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		msgs, _ := req["messages"].([]any)
		var systemText string
		for _, m := range msgs {
			msg, _ := m.(map[string]any)
			if msg["role"] == "system" {
				systemText, _ = msg["content"].(string)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 123456,
			"model":   "openai/gpt-4o",
			"choices": []any{
				map[string]any{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "EchoSystem: " + systemText,
					},
					"finish_reason": "stop",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockUpstream.Close()

	// Add provider connection
	database.CreateConnection(&model.ProviderConnection{
		ID:       "conn-openai",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-mock",
			BaseURL: mockUpstream.URL + "/v1",
		},
	})

	// Enable response cache
	settings, _ := database.GetSettings()
	settings.ResponseCacheEnabled = true
	settings.ResponseCacheTTL = 3600
	settings.ResponseCacheAll = true
	database.SaveSettings(settings)

	// Key A with SystemPrompt "Alpha"
	keyAStr := auth.GenerateAPIKey()
	database.CreateAPIKey(&model.APIKey{
		ID:           "key-a",
		Key:          keyAStr,
		Name:         "Key Alpha",
		IsActive:     true,
		SystemPrompt: "ContextAlpha",
	})

	// Key B with SystemPrompt "Beta"
	keyBStr := auth.GenerateAPIKey()
	database.CreateAPIKey(&model.APIKey{
		ID:           "key-b",
		Key:          keyBStr,
		Name:         "Key Beta",
		IsActive:     true,
		SystemPrompt: "ContextBeta",
	})

	body := `{"model":"openai/gpt-4o","messages":[{"role":"user","content":"what is my context?"}]}`

	// Call with Key A
	reqA := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	reqA.Header.Set("Content-Type", "application/json")
	reqA.Header.Set("Authorization", "Bearer "+keyAStr)
	wA := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wA, reqA)
	if wA.Code != http.StatusOK {
		t.Fatalf("call with Key A failed: %d body: %s", wA.Code, wA.Body.String())
	}
	if !strings.Contains(wA.Body.String(), "ContextAlpha") {
		t.Fatalf("expected ContextAlpha in response, got %s", wA.Body.String())
	}

	// Call with Key B with identical incoming body: MUST NOT hit Key A's cache!
	reqB := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	reqB.Header.Set("Content-Type", "application/json")
	reqB.Header.Set("Authorization", "Bearer "+keyBStr)
	wB := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wB, reqB)
	if wB.Code != http.StatusOK {
		t.Fatalf("call with Key B failed: %d body: %s", wB.Code, wB.Body.String())
	}
	if !strings.Contains(wB.Body.String(), "ContextBeta") {
		t.Fatalf("expected ContextBeta for Key B, got %s (did it incorrectly hit Key A's cache?)", wB.Body.String())
	}

	// Second call with Key A: SHOULD hit cache with ContextAlpha
	reqA2 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	reqA2.Header.Set("Content-Type", "application/json")
	reqA2.Header.Set("Authorization", "Bearer "+keyAStr)
	wA2 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wA2, reqA2)
	if wA2.Header().Get("X-Cyrene-Cache") != "HIT" {
		t.Fatalf("expected cache HIT for Key A second call, got %s", wA2.Header().Get("X-Cyrene-Cache"))
	}
	if !strings.Contains(wA2.Body.String(), "ContextAlpha") {
		t.Fatalf("cached response for Key A must have ContextAlpha, got %s", wA2.Body.String())
	}
}
