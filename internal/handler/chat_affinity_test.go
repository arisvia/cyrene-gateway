package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
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

// Anthropic Messages requests whose model resolves to a COMBO were delegated
// to the chat pipeline before the affinity key was ever derived: the combo
// branch ran ahead of the session-key block, and the translated OpenAI body
// carries none of prompt_cache_key/session_id/metadata.user_id. The key must
// be resolved (and stamped onto the request header) BEFORE that delegation.
func TestMessagesComboAffinitySurvivesDelegation(t *testing.T) {
	srv, database := setupTestServer(t)

	var mu sync.Mutex
	hits1, hits2 := 0, 0
	mock1 := upstreamCounter(t, &hits1, &mu)
	defer mock1.Close()
	mock2 := upstreamCounter(t, &hits2, &mu)
	defer mock2.Close()

	const pid = "audit-combo-aff"
	provider.Registry[pid] = provider.ProviderInfo{ID: pid, Name: pid, BaseURL: mock1.URL, APIType: "openai", AuthType: "api-key"}
	defer delete(provider.Registry, pid)

	for i, url := range []string{mock1.URL, mock2.URL} {
		if err := database.CreateConnection(&model.ProviderConnection{
			ID:       fmt.Sprintf("combo-aff-%d", i),
			Provider: pid,
			AuthType: "api-key",
			IsActive: true,
			Data:     model.ConnectionData{APIKey: fmt.Sprintf("sk-aff-%d", i), BaseURL: url + "/v1"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.CreateCombo(&model.Combo{ID: "aff-combo", Name: "aff-combo", Kind: "fallback", Models: []string{pid + "/gpt-4o"}}); err != nil {
		t.Fatal(err)
	}

	// Three Anthropic-shaped turns of one conversation routed through the combo
	// must stay on one account.
	for turn := range 3 {
		body := fmt.Sprintf(`{"model":"aff-combo","max_tokens":16,"messages":[{"role":"user","content":[{"type":"text","text":"turn %d"}]}],"metadata":{"user_id":"{\"session_id\":\"combo-session\"}"}}`, turn)
		req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	}
	if hits1+hits2 != 3 {
		t.Fatalf("expected 3 upstream hits, got %d+%d", hits1, hits2)
	}
	if hits1 != 0 && hits2 != 0 {
		t.Fatalf("combo turns of one session split across accounts: %d vs %d", hits1, hits2)
	}
}

// Most agent clients (pi, opencode, aider, VS Code BYOK) send no session field
// and no session header. Their consecutive turns must still pin one account via
// the tenant-scoped content fingerprint, while distinct conversations keep
// spreading over the pool — and a session header must override the fingerprint
// for two conversations that happen to open with the same words.
func TestContentFallbackPinsIdlessConversations(t *testing.T) {
	srv, database := setupTestServer(t)

	var mu sync.Mutex
	hits1, hits2 := 0, 0
	mock1 := upstreamCounter(t, &hits1, &mu)
	defer mock1.Close()
	mock2 := upstreamCounter(t, &hits2, &mu)
	defer mock2.Close()

	for i, url := range []string{mock1.URL, mock2.URL} {
		if err := database.CreateConnection(&model.ProviderConnection{
			ID:       fmt.Sprintf("fb-conn-%d", i),
			Provider: "openai",
			AuthType: "api-key",
			IsActive: true,
			Data:     model.ConnectionData{APIKey: fmt.Sprintf("sk-fb-%d", i), BaseURL: url + "/v1"},
		}); err != nil {
			t.Fatal(err)
		}
	}

	post := func(body string, extraHeader [2]string) {
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if extraHeader[0] != "" {
			req.Header.Set(extraHeader[0], extraHeader[1])
		}
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	}

	// One id-less conversation, three growing turns: same opener every time.
	msgs := []string{`{"role":"system","content":"persona"}`, `{"role":"user","content":"deploy the canary please"}`}
	for turn := range 3 {
		if turn > 0 {
			msgs = append(msgs, `{"role":"assistant","content":"ack"}`)
		}
		post(`{"model":"openai/gpt-4o","messages":[`+strings.Join(msgs, ",")+`]}`, [2]string{})
		msgs = append(msgs, fmt.Sprintf(`{"role":"user","content":"follow up %d"}`, turn))
	}
	if hits1+hits2 != 3 {
		t.Fatalf("expected 3 upstream hits, got %d+%d", hits1, hits2)
	}
	if hits1 != 0 && hits2 != 0 {
		t.Fatalf("id-less conversation split across accounts: %d vs %d", hits1, hits2)
	}

	// Distinct openers are distinct conversations: must still load balance.
	hits1, hits2 = 0, 0
	for i := range 4 {
		post(fmt.Sprintf(`{"model":"openai/gpt-4o","messages":[{"role":"user","content":"distinct opener %d"}]}`, i), [2]string{})
	}
	if hits1 == 0 || hits2 == 0 {
		t.Fatalf("distinct id-less conversations must load balance, got %d vs %d", hits1, hits2)
	}

	// Header beats content: identical opener, different session ids, must not
	// share the fingerprint pin.
	hits1, hits2 = 0, 0
	for i := range 2 {
		post(`{"model":"openai/gpt-4o","messages":[{"role":"user","content":"same opener both"}]}`,
			[2]string{"X-Session-Id", fmt.Sprintf("sess-%d", i)})
	}
	if hits1 == 0 || hits2 == 0 {
		t.Fatalf("session header must override the content fingerprint, got %d vs %d", hits1, hits2)
	}
}
