package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// An account whose own quota is exhausted must not fail the client's request
// when another account for the same provider still has capacity.
func TestSingleModel_FailsOverWhenAccountQuotaExhausted(t *testing.T) {
	var deadHits, liveHits atomic.Int32
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"You have exhausted your capacity on this model."}}`))
	}))
	defer dead.Close()
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		liveHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-live", "object": "chat.completion", "model": "m",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "served by backup"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 5, "completion_tokens": 3, "total_tokens": 8},
		})
	}))
	defer live.Close()

	srv, database := setupTestServer(t)
	for _, c := range []*model.ProviderConnection{
		{ID: "dead", Provider: "openai", AuthType: "api-key", IsActive: true, Priority: 0,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
			Data: model.ConnectionData{APIKey: "sk-dead", BaseURL: dead.URL + "/chat/completions"}},
		{ID: "live", Provider: "openai", AuthType: "api-key", IsActive: true, Priority: 0,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
			Data: model.ConnectionData{APIKey: "sk-live", BaseURL: live.URL + "/chat/completions"}},
	} {
		if err := database.CreateConnection(c); err != nil {
			t.Fatal(err)
		}
	}

	// Every request must succeed even though one of the two accounts is dead.
	for i := range 4 {
		body := `{"model":"openai/m","messages":[{"role":"user","content":"hi"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("request %d failed with %d: %s", i, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "served by backup") {
			t.Fatalf("request %d did not reach the healthy account: %s", i, rec.Body.String())
		}
	}
	if liveHits.Load() != 4 {
		t.Errorf("healthy account should serve all 4 requests, got %d", liveHits.Load())
	}
}

// The dead account must be parked, so later requests skip it entirely.
func TestSingleModel_ExhaustedAccountIsParked(t *testing.T) {
	var deadHits atomic.Int32
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"quota exhausted"}}`))
	}))
	defer dead.Close()
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","object":"chat.completion","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer live.Close()

	srv, database := setupTestServer(t)
	// Distinct priorities make the attempt order deterministic: the exhausted
	// account is always preferred, so parking it is what the test observes.
	database.CreateConnection(&model.ProviderConnection{
		ID: "dead", Provider: "openai", AuthType: "api-key", IsActive: true, Priority: 0,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Data: model.ConnectionData{APIKey: "sk-dead", BaseURL: dead.URL + "/chat/completions"},
	})
	database.CreateConnection(&model.ProviderConnection{
		ID: "live", Provider: "openai", AuthType: "api-key", IsActive: true, Priority: 10,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Data: model.ConnectionData{APIKey: "sk-live", BaseURL: live.URL + "/chat/completions"},
	})

	send := func() int {
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"openai/m","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := send(); code != http.StatusOK {
		t.Fatalf("first request should fail over and succeed, got %d", code)
	}
	first := deadHits.Load()
	if first == 0 {
		t.Fatal("the dead account should have been attempted at least once")
	}

	// Once parked, the dead account must not be dialed again.
	for range 3 {
		if code := send(); code != http.StatusOK {
			t.Fatalf("subsequent request failed: %d", code)
		}
	}
	if deadHits.Load() != first {
		t.Errorf("parked account was retried: hits went %d -> %d", first, deadHits.Load())
	}
}

// Request-scoped 4xx must not fan out across the whole pool.
func TestSingleModel_ClientErrorDoesNotFailover(t *testing.T) {
	var hits atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"context length exceeded"}}`))
	}))
	defer up.Close()

	srv, database := setupTestServer(t)
	for _, id := range []string{"a", "b"} {
		database.CreateConnection(&model.ProviderConnection{
			ID: id, Provider: "openai", AuthType: "api-key", IsActive: true,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
			Data: model.ConnectionData{APIKey: "sk", BaseURL: up.URL + "/chat/completions"},
		})
	}

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"openai/m","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 passthrough, got %d", rec.Code)
	}
	if hits.Load() != 1 {
		t.Errorf("a client error must not be retried on other accounts, got %d hits", hits.Load())
	}
}

// A client-pinned connection must not be silently swapped for another account.
func TestSingleModel_PinnedConnectionDoesNotFailOver(t *testing.T) {
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","object":"chat.completion","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"other"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer other.Close()
	pinnedUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"quota exhausted"}}`))
	}))
	defer pinnedUp.Close()

	srv, database := setupTestServer(t)
	database.CreateConnection(&model.ProviderConnection{
		ID: "pinned", Provider: "openai", AuthType: "api-key", IsActive: true, Priority: 0,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Data: model.ConnectionData{APIKey: "sk-pinned", BaseURL: pinnedUp.URL + "/chat/completions"},
	})
	database.CreateConnection(&model.ProviderConnection{
		ID: "other", Provider: "openai", AuthType: "api-key", IsActive: true, Priority: 10,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Data: model.ConnectionData{APIKey: "sk-other", BaseURL: other.URL + "/chat/completions"},
	})

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"openai/m","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Cyrene-Connection-ID", "pinned")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected the pinned account's error to surface, got %d: %s", rec.Code, rec.Body.String())
	}
	if otherHits.Load() != 0 {
		t.Errorf("pinned request must not be served by another account, got %d hits", otherHits.Load())
	}
}

func TestMessagesViaChat_RotatesEvenlyAcrossEqualPriorityAccounts(t *testing.T) {
	var hitsA, hitsB atomic.Int32
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsA.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-a", "object": "chat.completion", "model": "gemini-2.5-flash",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "from-a"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7},
		})
	}))
	defer serverA.Close()

	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsB.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-b", "object": "chat.completion", "model": "gemini-2.5-flash",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "from-b"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7},
		})
	}))
	defer serverB.Close()

	srv, database := setupTestServer(t)
	for _, c := range []*model.ProviderConnection{
		{ID: "conn-a", Provider: "openai", AuthType: "api-key", IsActive: true, Priority: 0,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
			Data: model.ConnectionData{APIKey: "sk-a", BaseURL: serverA.URL + "/chat/completions"}},
		{ID: "conn-b", Provider: "openai", AuthType: "api-key", IsActive: true, Priority: 0,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
			Data: model.ConnectionData{APIKey: "sk-b", BaseURL: serverB.URL + "/chat/completions"}},
	} {
		if err := database.CreateConnection(c); err != nil {
			t.Fatal(err)
		}
	}

	// Six DISTINCT conversations (id-less clients fall back to the content
	// fingerprint, so distinct openers = distinct sessions), two growing turns
	// each. Conversation turns must reuse their binding without advancing the
	// pool, and fresh conversations must still alternate 6/6 — the delegation
	// branch selecting a connection twice per request would double-advance the
	// round-robin counter and collapse every binding onto one account.
	for i := range 6 {
		first := fmt.Sprintf(`{"model":"openai/gpt-4o","max_tokens":100,"messages":[{"role":"user","content":"hi %d"}]}`, i)
		second := fmt.Sprintf(`{"model":"openai/gpt-4o","max_tokens":100,"messages":[{"role":"user","content":"hi %d"},{"role":"assistant","content":"yo"},{"role":"user","content":"more %d"}]}`, i, i)
		for _, body := range []string{first, second} {
			req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("request %d failed with status %d: %s", i, rec.Code, rec.Body.String())
			}
		}
	}

	// Both accounts must alternate evenly (6 and 6), not 12 and 0.
	if hitsA.Load() != 6 || hitsB.Load() != 6 {
		t.Fatalf("expected 6 hits each on equal priority accounts, got A=%d, B=%d", hitsA.Load(), hitsB.Load())
	}
}
