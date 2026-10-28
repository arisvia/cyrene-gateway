package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// upstreamCounter is a minimal OpenAI-shaped mock with a hit counter.
func upstreamCounter(t *testing.T, hits *int, mu *sync.Mutex) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*hits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"chatcmpl-mock","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`))
	}))
}

func TestChatSessionAffinityPinsAccount(t *testing.T) {
	srv, database := setupTestServer(t)

	var mu sync.Mutex
	hits1, hits2 := 0, 0
	mock1 := upstreamCounter(t, &hits1, &mu)
	defer mock1.Close()
	mock2 := upstreamCounter(t, &hits2, &mu)
	defer mock2.Close()

	for i, url := range []string{mock1.URL, mock2.URL} {
		database.CreateConnection(&model.ProviderConnection{
			ID:       fmt.Sprintf("aff-conn-%d", i),
			Provider: "openai",
			AuthType: "api-key",
			IsActive: true,
			Data:     model.ConnectionData{APIKey: fmt.Sprintf("sk-aff-%d", i), BaseURL: url + "/v1"},
		})
	}

	post := func(body string) {
		t.Helper()
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	}

	// Three turns of one conversation share a single account...
	for turn := range 3 {
		post(fmt.Sprintf(`{"model":"openai/gpt-4o","prompt_cache_key":"sess-pin","messages":[{"role":"user","content":"turn %d"}]}`, turn))
	}
	if hits1+hits2 != 3 {
		t.Fatalf("expected 3 upstream hits, got %d+%d", hits1, hits2)
	}
	if hits1 != 0 && hits2 != 0 {
		t.Fatalf("same session split across accounts: %d vs %d", hits1, hits2)
	}

	// ...while distinct sessions still spread over the pool.
	hits1, hits2 = 0, 0
	for i := range 4 {
		post(fmt.Sprintf(`{"model":"openai/gpt-4o","prompt_cache_key":"sess-spread-%d","messages":[{"role":"user","content":"hi %d"}]}`, i, i))
	}
	if hits1 == 0 || hits2 == 0 {
		t.Fatalf("distinct sessions must load balance, got %d vs %d", hits1, hits2)
	}
}

// The Anthropic Messages path for non-native providers is translated to the
// OpenAI shape, which drops metadata.user_id; the session key must survive the
// delegation via the internal header.
func TestMessagesSessionAffinitySurvivesTranslation(t *testing.T) {
	srv, database := setupTestServer(t)

	var mu sync.Mutex
	hits1, hits2 := 0, 0
	mock1 := upstreamCounter(t, &hits1, &mu)
	defer mock1.Close()
	mock2 := upstreamCounter(t, &hits2, &mu)
	defer mock2.Close()

	for i, url := range []string{mock1.URL, mock2.URL} {
		database.CreateConnection(&model.ProviderConnection{
			ID:       fmt.Sprintf("msg-conn-%d", i),
			Provider: "openai",
			AuthType: "api-key",
			IsActive: true,
			Data:     model.ConnectionData{APIKey: fmt.Sprintf("sk-msg-%d", i), BaseURL: url + "/v1"},
		})
	}

	for turn := range 3 {
		body := fmt.Sprintf(`{"model":"openai/gpt-4o","max_tokens":16,"messages":[{"role":"user","content":[{"type":"text","text":"turn %d"}]}],"metadata":{"user_id":"{\"session_id\":\"claude-code-abc\"}"}}`, turn)
		req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	}
	if hits1 != 0 && hits2 != 0 {
		t.Fatalf("one Anthropic session split across accounts: %d vs %d", hits1, hits2)
	}
	if hits1+hits2 != 3 {
		t.Fatalf("expected 3 upstream hits, got %d+%d", hits1, hits2)
	}
}
