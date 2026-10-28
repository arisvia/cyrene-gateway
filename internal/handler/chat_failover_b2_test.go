package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// B2a: the native Anthropic passthrough must walk the account pool like the
// chat pipeline — an account-scoped 429 retries on the next account instead of
// failing the client. Also pins the output_config strip: after removing
// .format, an EMPTY output_config object must be dropped from the outbound
// body entirely (third-party anthropic-compatible endpoints reject it).
func TestMessagesPassthroughFailsOverToHealthyAccount(t *testing.T) {
	var mu sync.Mutex
	var coolBody map[string]any
	mockCool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"quota exhausted"}}`))
	}))
	defer mockCool.Close()
	mockOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		_ = json.NewDecoder(r.Body).Decode(&coolBody)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"msg-ok","type":"message","role":"assistant","content":[{"type":"text","text":"fine"}],"model":"claude-x","stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2}}`))
	}))
	defer mockOK.Close()

	const pid = "audit-msgfail"
	provider.Registry[pid] = provider.ProviderInfo{ID: pid, Name: pid, BaseURL: mockCool.URL, APIType: "anthropic", AuthType: "api-key"}
	defer delete(provider.Registry, pid)

	srv, database := setupTestServer(t)
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "msg-cool", Provider: pid, AuthType: "api-key", IsActive: true, Priority: 0,
		Data: model.ConnectionData{APIKey: "k1", BaseURL: mockCool.URL},
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "msg-ok", Provider: pid, AuthType: "api-key", IsActive: true, Priority: 1,
		Data: model.ConnectionData{APIKey: "k2", BaseURL: mockOK.URL},
	}); err != nil {
		t.Fatal(err)
	}

	body := `{"model":"` + pid + `/claude-x","max_tokens":16,"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"output_config":{"format":{"type":"json_schema","json_schema":{"name":"t","schema":{"type":"object"}}}}}`
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected failover to healthy account to yield 200, got %d: %s", w.Code, w.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if coolBody == nil {
		t.Fatal("healthy account was never reached")
	}
	if oc, present := coolBody["output_config"]; present {
		if m, isMap := oc.(map[string]any); isMap && len(m) == 0 {
			t.Fatalf("empty output_config must be stripped from the outbound body, got %v", oc)
		}
	}
	// The cooling-down account must have its cooldown persisted.
	conn, err := database.GetConnection("msg-cool")
	if err != nil {
		t.Fatal(err)
	}
	if conn.Data.RateLimitedUntil == "" {
		t.Fatal("429 account must be marked cooling down after passthrough failure")
	}
}

// B2b: a combo step must exhaust the accounts of the SAME model before
// advancing to the next model — one throttled account must not demote the
// model while a healthy sibling exists.
func TestComboRetriesNextAccountBeforeNextModel(t *testing.T) {
	var mu sync.Mutex
	coolHits, okHits, nextModelHits := 0, 0, 0
	mockCool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		coolHits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"quota"}}`))
	}))
	defer mockCool.Close()
	completion := `{"id":"cb","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	mockOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		okHits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(completion))
	}))
	defer mockOK.Close()
	mockNext := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		nextModelHits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(completion))
	}))
	defer mockNext.Close()

	const p1, p2 = "audit-cb1", "audit-cb2"
	provider.Registry[p1] = provider.ProviderInfo{ID: p1, Name: p1, BaseURL: mockCool.URL, APIType: "openai", AuthType: "api-key"}
	provider.Registry[p2] = provider.ProviderInfo{ID: p2, Name: p2, BaseURL: mockNext.URL, APIType: "openai", AuthType: "api-key"}
	defer delete(provider.Registry, p1)
	defer delete(provider.Registry, p2)

	srv, database := setupTestServer(t)
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "cb-cool", Provider: p1, AuthType: "api-key", IsActive: true, Priority: 0,
		Data: model.ConnectionData{APIKey: "k1", BaseURL: mockCool.URL},
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "cb-ok", Provider: p1, AuthType: "api-key", IsActive: true, Priority: 1,
		Data: model.ConnectionData{APIKey: "k2", BaseURL: mockOK.URL},
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "cb-next", Provider: p2, AuthType: "api-key", IsActive: true,
		Data: model.ConnectionData{APIKey: "k3"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateCombo(&model.Combo{ID: "cb-combo", Name: "cb-combo", Kind: "fallback", Models: []string{p1 + "/m", p2 + "/m"}}); err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"model":"cb-combo","messages":[{"role":"user","content":"hi"}],"max_tokens":8}`)
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected second account of the same model to serve 200, got %d: %s", w.Code, w.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if coolHits != 1 || okHits != 1 {
		t.Fatalf("expected exactly one fail per throttled account (cool=%d ok=%d)", coolHits, okHits)
	}
	if nextModelHits != 0 {
		t.Fatal("combo advanced to the next model despite a healthy account on the first model")
	}
}

// B1: embeddings must authenticate through the unified credential path — an
// OAuth-only connection (no static API key) previously reached upstream with
// NO Authorization header at all.
func TestEmbeddingsOAuthConnectionSendsBearer(t *testing.T) {
	var mu sync.Mutex
	var authHeader string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authHeader = r.Header.Get("Authorization")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"object":"list","data":[{"object":"embedding","embedding":[0.1,0.2],"index":0}],"model":"emb-1","usage":{"prompt_tokens":1,"total_tokens":1}}`))
	}))
	defer mock.Close()

	const pid = "audit-emb"
	provider.Registry[pid] = provider.ProviderInfo{ID: pid, Name: pid, BaseURL: mock.URL + "/v1", APIType: "openai", AuthType: "oauth"}
	defer delete(provider.Registry, pid)

	srv, database := setupTestServer(t)
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "emb-oauth", Provider: pid, AuthType: "oauth", IsActive: true,
		Data: model.ConnectionData{AccessToken: "tok-emb-audit"},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/v1/embeddings", strings.NewReader(`{"model":"`+pid+`/emb-1","input":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if authHeader != "Bearer tok-emb-audit" {
		t.Fatalf("OAuth embeddings request must carry the access token, got %q", authHeader)
	}
}
