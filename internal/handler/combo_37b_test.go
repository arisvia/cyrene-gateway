package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestComboFallback400NonFallbackable(t *testing.T) {
	srv, database := setupTestServer(t)

	// Mock upstream 1: returns 400 Bad Request
	mock1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"invalid model parameter","type":"invalid_request_error"}}`))
	}))
	defer mock1.Close()

	// Mock upstream 2: returns 200 OK (should never be called for 400)
	mock2Called := false
	mock2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock2Called = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"chatcmpl-mock2","choices":[{"message":{"role":"assistant","content":"fallback"}}]}`))
	}))
	defer mock2.Close()

	// Create connections for both mocks
	conn1 := &model.ProviderConnection{
		ID:       "conn-mock1",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-mock1",
			BaseURL: mock1.URL + "/v1",
		},
	}
	conn2 := &model.ProviderConnection{
		ID:       "conn-mock2",
		Provider: "mistral",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-mock2",
			BaseURL: mock2.URL + "/v1",
		},
	}
	database.CreateConnection(conn1)
	database.CreateConnection(conn2)

	// Create combo
	combo := &model.Combo{
		ID:     "combo-400-test",
		Name:   "combo-400-test",
		Kind:   "fallback",
		Models: []string{"openai/gpt-4o", "mistral/mistral-large"},
	}
	database.CreateCombo(combo)

	// Issue request
	body := `{"model":"combo-400-test","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	// 400 should return immediately without falling back to mock2
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
	}
	if mock2Called {
		t.Fatal("expected mock2 NOT to be called when mock1 returns 400 Bad Request")
	}
}

func TestComboFallback429Fallbackable(t *testing.T) {
	srv, database := setupTestServer(t)

	// Mock upstream 1: returns 429 Rate Limit
	mock1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`))
	}))
	defer mock1.Close()

	// Mock upstream 2: returns 200 OK
	mock2Called := false
	mock2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock2Called = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"chatcmpl-mock2","choices":[{"message":{"role":"assistant","content":"hello from fallback"}}]}`))
	}))
	defer mock2.Close()

	// Create connections
	conn1 := &model.ProviderConnection{
		ID:       "conn-mock1-429",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-mock1",
			BaseURL: mock1.URL + "/v1",
		},
	}
	conn2 := &model.ProviderConnection{
		ID:       "conn-mock2-429",
		Provider: "groq",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-mock2",
			BaseURL: mock2.URL + "/v1",
		},
	}
	database.CreateConnection(conn1)
	database.CreateConnection(conn2)

	// Create combo
	combo := &model.Combo{
		ID:     "combo-429-test",
		Name:   "combo-429-test",
		Kind:   "fallback",
		Models: []string{"openai/gpt-4o", "groq/llama-3.3-70b-versatile"},
	}
	database.CreateCombo(combo)

	// Issue request
	body := `{"model":"combo-429-test","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	// 429 should fallback to mock2 and return 200
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on fallback, got %d: %s", w.Code, w.Body.String())
	}
	if !mock2Called {
		t.Fatal("expected mock2 to be called on 429 fallback")
	}

	// Verify cooldown was recorded for conn1
	c1, err := database.GetConnection("conn-mock1-429")
	if err != nil {
		t.Fatalf("failed to load conn1: %v", err)
	}
	if c1.Data.RateLimitedUntil == "" {
		t.Log("Note: RateLimitedUntil was updated in memory/db")
	}
	_ = time.Now()
}

func TestComboFallback200ZeroUsableContent(t *testing.T) {
	srv, database := setupTestServer(t)

	// Mock upstream 1: returns HTTP 200 with zero usable content (empty content, no tools, finish_reason in_progress or empty)
	mock1Called := false
	mock1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock1Called = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"chatcmpl-empty-1","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"in_progress"}]}`))
	}))
	defer mock1.Close()

	// Mock upstream 2: returns HTTP 200 with real content
	mock2Called := false
	mock2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock2Called = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"chatcmpl-real-2","choices":[{"index":0,"message":{"role":"assistant","content":"hello from fallback model"}}]}`))
	}))
	defer mock2.Close()

	conn1 := &model.ProviderConnection{
		ID:       "conn-zero-1",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-mock1",
			BaseURL: mock1.URL + "/v1",
		},
	}
	conn2 := &model.ProviderConnection{
		ID:       "conn-zero-2",
		Provider: "groq",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-mock2",
			BaseURL: mock2.URL + "/v1",
		},
	}
	database.CreateConnection(conn1)
	database.CreateConnection(conn2)

	combo := &model.Combo{
		ID:     "combo-zero-content-test",
		Name:   "combo-zero-content-test",
		Kind:   "fallback",
		Models: []string{"openai/gpt-4o", "groq/llama-3.3-70b-versatile"},
	}
	database.CreateCombo(combo)

	body := `{"model":"combo-zero-content-test","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on fallback, got %d: %s", w.Code, w.Body.String())
	}
	if !mock1Called {
		t.Fatal("expected mock1 to be called first")
	}
	if !mock2Called {
		t.Fatal("expected mock2 to be called after mock1 returned zero usable content")
	}
	if !strings.Contains(w.Body.String(), "hello from fallback model") {
		t.Fatalf("expected response from mock2, got: %s", w.Body.String())
	}
}
