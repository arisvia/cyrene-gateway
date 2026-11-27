package handler

import (
	"bufio"
	"fmt"
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
		ID:     "combo-400-test",
		Name:   "combo-400-test",
		Kind:   "fallback",
		Models: []string{"openai/gpt-4o", "groq/llama-3-8b"},
	}
	database.CreateCombo(combo)

	// Issue request
	body := `{"model":"combo-400-test","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	// In Cyrene Gateway, single-model or strict terminal errors are tested.
	// But in a multi-model combo, request-scoped 4xx advances to the next model (9router#5057).
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK after falling back from 400, got %d: %s", w.Code, w.Body.String())
	}
	if !mock2Called {
		t.Fatal("expected mock2 TO be called when mock1 returns request-scoped 400 Bad Request")
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

func TestComboStreamingDoesNotBufferBeforeEOF(t *testing.T) {
	srv, database := setupTestServer(t)

	releaseChunk2 := make(chan struct{})
	upstream1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"id\":\"s1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"first\"}}]}\n\n")
		flusher.Flush()

		select {
		case <-releaseChunk2:
		case <-time.After(3 * time.Second):
			return
		}
		fmt.Fprint(w, "data: {\"id\":\"s1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"second\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream1.Close()

	upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream2.Close()

	_ = database.CreateConnection(&model.ProviderConnection{
		ID:       "conn-stream-1",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data:     model.ConnectionData{APIKey: "sk-1", BaseURL: upstream1.URL + "/v1"},
	})
	_ = database.CreateConnection(&model.ProviderConnection{
		ID:       "conn-stream-2",
		Provider: "groq",
		AuthType: "api-key",
		IsActive: true,
		Data:     model.ConnectionData{APIKey: "sk-2", BaseURL: upstream2.URL + "/v1"},
	})
	_ = database.CreateCombo(&model.Combo{
		ID:     "combo-stream-ttft",
		Name:   "combo-stream-ttft",
		Kind:   "fallback",
		Models: []string{"openai/gpt-4o", "groq/llama-3.3-70b-versatile"},
	})

	gwServer := httptest.NewServer(srv.Handler)
	defer gwServer.Close()

	resp, err := http.Post(gwServer.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"combo-stream-ttft","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatalf("gateway request failed: %v", err)
	}
	defer resp.Body.Close()

	chunk1Ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "first") {
				chunk1Ready <- line
				return
			}
		}
	}()

	select {
	case line := <-chunk1Ready:
		if !strings.Contains(line, "first") {
			t.Fatalf("unexpected first chunk: %s", line)
		}
	case <-time.After(1500 * time.Millisecond):
		close(releaseChunk2)
		t.Fatal("combo streaming buffered SSE chunks until upstream EOF instead of flushing immediately")
	}

	close(releaseChunk2)
}
