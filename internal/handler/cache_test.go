package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestResponseCacheIntegration(t *testing.T) {
	srv, database := setupTestServer(t)

	// Enable response cache in settings
	settings, _ := database.GetSettings()
	settings.ResponseCacheEnabled = true
	settings.ResponseCacheTTL = 3600
	settings.ResponseCacheAll = false
	if err := database.SaveSettings(settings); err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	// Mock upstream server
	var upstreamCalls atomic.Int32
	var lastEmbeddingsBody map[string]any
	upstreamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/embeddings" {
			json.NewDecoder(r.Body).Decode(&lastEmbeddingsBody)
			resp := map[string]any{
				"object": "list",
				"data": []any{
					map[string]any{
						"object":    "embedding",
						"embedding": []float64{0.1, 0.2, 0.3},
						"index":     0,
					},
				},
				"model": "text-embedding-3-small",
				"usage": map[string]any{
					"prompt_tokens": 5,
					"total_tokens":  5,
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		resp := map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "mock-gpt",
			"choices": []any{
				map[string]any{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "Hello! I am cached.",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstreamSrv.Close()

	// Register a mock provider connection
	conn := &model.ProviderConnection{
		ID:        "mock-conn-1",
		Provider:  "openai",
		Name:      "Mock OpenAI",
		AuthType:  "api-key",
		IsActive:  true,
		Priority:  1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Data: model.ConnectionData{
			APIKey:  "sk-mock-test",
			BaseURL: upstreamSrv.URL + "/v1",
		},
	}
	if err := database.CreateConnection(conn); err != nil {
		t.Fatalf("failed to create mock connection: %v", err)
	}

	// 1. First request with temperature=0 (deterministic): should MISS and store in cache
	reqBody := map[string]any{
		"model":       "mock-gpt",
		"temperature": 0.0,
		"messages": []any{
			map[string]any{"role": "user", "content": "hello world"},
		},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req1 := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(bodyBytes))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	srv.handleChatCompletions(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w1.Code, w1.Body.String())
	}
	if cacheHdr := w1.Header().Get("X-Cyrene-Cache"); cacheHdr != "MISS" {
		t.Errorf("expected X-Cyrene-Cache: MISS on first call, got %s", cacheHdr)
	}
	if calls := upstreamCalls.Load(); calls != 1 {
		t.Fatalf("expected 1 upstream call, got %d", calls)
	}

	// 2. Second request: exact duplicate -> should HIT, zero upstream calls
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(bodyBytes))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	srv.handleChatCompletions(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w2.Code, w2.Body.String())
	}
	if cacheHdr := w2.Header().Get("X-Cyrene-Cache"); cacheHdr != "HIT" {
		t.Errorf("expected X-Cyrene-Cache: HIT on second call, got %s", cacheHdr)
	}
	if calls := upstreamCalls.Load(); calls != 1 {
		t.Fatalf("expected still 1 upstream call (served from cache), got %d", calls)
	}

	var resp2 map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("failed to decode cached response: %v", err)
	}
	choices := resp2["choices"].([]any)
	firstChoice := choices[0].(map[string]any)
	msg := firstChoice["message"].(map[string]any)
	if msg["content"] != "Hello! I am cached." {
		t.Errorf("expected cached content, got %v", msg["content"])
	}

	// 3. Invalidation test on Settings save: saving settings should purge cache
	putSettingsBody := bytes.NewReader([]byte(`{"rtkEnabled":true,"responseCacheEnabled":true}`))
	putReq := httptest.NewRequest("PUT", "/api/settings", putSettingsBody)
	putReq.Header.Set("Content-Type", "application/json")
	putW := httptest.NewRecorder()
	srv.handlePutSettings(putW, putReq)
	if putW.Code != http.StatusOK {
		t.Fatalf("failed to put settings: %d", putW.Code)
	}

	// After settings save, same request must MISS
	reqAfterSettings := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(bodyBytes))
	reqAfterSettings.Header.Set("Content-Type", "application/json")
	wAfterSettings := httptest.NewRecorder()
	srv.handleChatCompletions(wAfterSettings, reqAfterSettings)
	if wAfterSettings.Header().Get("X-Cyrene-Cache") != "MISS" {
		t.Errorf("expected MISS after settings save invalidation, got %s", wAfterSettings.Header().Get("X-Cyrene-Cache"))
	}
	if calls := upstreamCalls.Load(); calls != 2 {
		t.Errorf("expected upstream call after settings invalidation, got %d", calls)
	}

	// 4. Bypass test: request with Cache-Control: no-cache -> should call upstream
	req3 := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(bodyBytes))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Cache-Control", "no-cache")
	w3 := httptest.NewRecorder()
	srv.handleChatCompletions(w3, req3)

	if calls := upstreamCalls.Load(); calls != 3 {
		t.Errorf("expected upstream call due to Cache-Control: no-cache, got %d", calls)
	}

	// 5. Embeddings cache test with dimensions & encoding_format preservation
	embBody := []byte(`{"model":"openai/text-embedding-3-small","input":"machine learning","dimensions":256,"encoding_format":"float"}`)
	embReq1 := httptest.NewRequest("POST", "/v1/embeddings", bytes.NewReader(embBody))
	embReq1.Header.Set("Content-Type", "application/json")
	embW1 := httptest.NewRecorder()
	srv.handleEmbeddings(embW1, embReq1)

	if embW1.Code != http.StatusOK {
		t.Fatalf("expected 200 from embeddings, got %d: %s", embW1.Code, embW1.Body.String())
	}
	if embW1.Header().Get("X-Cyrene-Cache") != "MISS" {
		t.Errorf("expected MISS on first embeddings call")
	}
	// Verify dimensions and encoding_format were forwarded to upstream
	if dims, ok := lastEmbeddingsBody["dimensions"].(float64); !ok || int(dims) != 256 {
		t.Errorf("expected upstream to receive dimensions: 256, got %v", lastEmbeddingsBody["dimensions"])
	}
	if enc, ok := lastEmbeddingsBody["encoding_format"].(string); !ok || enc != "float" {
		t.Errorf("expected upstream to receive encoding_format: float, got %v", lastEmbeddingsBody["encoding_format"])
	}

	// Duplicate embeddings request -> HIT
	embReq2 := httptest.NewRequest("POST", "/v1/embeddings", bytes.NewReader(embBody))
	embReq2.Header.Set("Content-Type", "application/json")
	embW2 := httptest.NewRecorder()
	srv.handleEmbeddings(embW2, embReq2)

	if embW2.Code != http.StatusOK {
		t.Fatalf("expected 200 from second embeddings, got %d", embW2.Code)
	}
	if embW2.Header().Get("X-Cyrene-Cache") != "HIT" {
		t.Errorf("expected HIT on second embeddings call")
	}

	// 6. Clear cache test
	clearReq := httptest.NewRequest("POST", "/api/cache/clear", nil)
	clearW := httptest.NewRecorder()
	srv.handleClearCache(clearW, clearReq)
	if clearW.Code != http.StatusOK {
		t.Errorf("expected 200 from /api/cache/clear, got %d", clearW.Code)
	}

	// Next request should MISS
	req4 := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(bodyBytes))
	req4.Header.Set("Content-Type", "application/json")
	w4 := httptest.NewRecorder()
	srv.handleChatCompletions(w4, req4)

	if cacheHdr := w4.Header().Get("X-Cyrene-Cache"); cacheHdr != "MISS" {
		t.Errorf("expected X-Cyrene-Cache: MISS after clear, got %s", cacheHdr)
	}
}

func TestResponseCachePanicSafety(t *testing.T) {
	srv, database := setupTestServer(t)

	settings, _ := database.GetSettings()
	settings.ResponseCacheEnabled = true
	settings.ResponseCacheTTL = 3600
	settings.ResponseCacheAll = true
	database.SaveSettings(settings)

	// Upstream server that writes partial bytes then disconnects/panics
	upstreamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"partial":`)) // truncated JSON
		panic("simulated upstream processing panic")
	}))
	defer upstreamSrv.Close()

	conn := &model.ProviderConnection{
		ID:        "mock-panic-conn",
		Provider:  "openai",
		Name:      "Mock Panic",
		AuthType:  "api-key",
		IsActive:  true,
		Priority:  1,
		Data: model.ConnectionData{
			APIKey:  "sk-mock",
			BaseURL: upstreamSrv.URL + "/v1",
		},
	}
	database.CreateConnection(conn)

	reqBody := []byte(`{"model":"mock-panic","messages":[{"role":"user","content":"test"}]}`)
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	// Execute through full handler chain (including Recovery middleware)
	srv.Handler.ServeHTTP(w, req)

	// Verify cache does NOT have the entry
	stats := srv.Cache.Stats()
	if stats.Entries != 0 {
		t.Fatalf("expected 0 cache entries after panicking request, got %d", stats.Entries)
	}
}
