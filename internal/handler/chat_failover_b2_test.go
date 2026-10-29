package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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

// Dedicated-handler providers (antigravity) ship with an empty registry base
// URL; the OpenAI-style /embeddings proxy cannot construct a target for them.
// Must fail with a clean 400 instead of dialing the relative "/embeddings".
func TestEmbeddingsEmptyBaseURLRejectsCleanly(t *testing.T) {
	const pid = "audit-emb-emptybase"
	provider.Registry[pid] = provider.ProviderInfo{ID: pid, Name: pid, BaseURL: "", APIType: "openai", AuthType: "oauth"}
	defer delete(provider.Registry, pid)

	srv, database := setupTestServer(t)
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "emb-eb", Provider: pid, AuthType: "oauth", IsActive: true,
		Data: model.ConnectionData{AccessToken: "tok"},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/v1/embeddings", strings.NewReader(`{"model":"`+pid+`/m","input":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty base URL must yield 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no base URL") {
		t.Fatalf("400 body must explain the cause, got %s", w.Body.String())
	}
}

// A client hang-up must not cool down the account the request happened to sit
// on: the transport error is the client leaving, not an upstream failure, and
// the failover walk would otherwise cascade the cooldown across the whole pool.
func TestMessagesPassthroughClientCancelDoesNotCoolDown(t *testing.T) {
	hit := make(chan struct{})
	release := make(chan struct{})
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(hit)
		select {
		case <-r.Context().Done(): // the client left
		case <-release: // teardown
		}
	}))
	defer func() { close(release); mock.Close() }()

	const pid = "audit-msgcancel"
	provider.Registry[pid] = provider.ProviderInfo{ID: pid, Name: pid, BaseURL: mock.URL, APIType: "anthropic", AuthType: "api-key"}
	defer delete(provider.Registry, pid)

	srv, database := setupTestServer(t)
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "msg-cancel", Provider: pid, AuthType: "api-key", IsActive: true,
		Data: model.ConnectionData{APIKey: "k1", BaseURL: mock.URL},
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	body := `{"model":"` + pid + `/claude-x","max_tokens":16,"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctx)

	done := make(chan struct{})
	go func() {
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		close(done)
	}()

	select {
	case <-hit:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("upstream was never reached")
	}
	cancel()
	<-done
	// The aborted upstream connection may linger in the gateway's transport;
	// cut it so the server-side handler unblocks and Close() returns.
	mock.CloseClientConnections()

	conn, err := database.GetConnection("msg-cancel")
	if err != nil {
		t.Fatal(err)
	}
	if conn.Data.RateLimitedUntil != "" || conn.Data.TestStatus == "error" {
		t.Fatalf("client cancellation must not cool down the account: rateLimitedUntil=%q testStatus=%q",
			conn.Data.RateLimitedUntil, conn.Data.TestStatus)
	}
}

// Embeddings persists error state on 4xx/5xx; the success path must clear it
// again — parity with every sibling path that writes error state. Without the
// reset, a single 429 leaves the account on an elevated backoff level forever.
func TestEmbeddingsSuccessResetsErrorState(t *testing.T) {
	var mu sync.Mutex
	throttle := true
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		isThrottle := throttle
		throttle = false
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if isThrottle {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"message":"rate limit exceeded"}}`))
			return
		}
		w.Write([]byte(`{"object":"list","data":[{"object":"embedding","embedding":[0.1],"index":0}],"model":"emb-1","usage":{"prompt_tokens":1,"total_tokens":1}}`))
	}))
	defer mock.Close()

	const pid = "audit-emb-reset"
	provider.Registry[pid] = provider.ProviderInfo{ID: pid, Name: pid, BaseURL: mock.URL + "/v1", APIType: "openai", AuthType: "api-key"}
	defer delete(provider.Registry, pid)

	srv, database := setupTestServer(t)
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "emb-reset", Provider: pid, AuthType: "api-key", IsActive: true,
		Data: model.ConnectionData{APIKey: "sk-emb"},
	}); err != nil {
		t.Fatal(err)
	}

	post := func() int {
		req := httptest.NewRequest("POST", "/v1/embeddings", strings.NewReader(`{"model":"`+pid+`/emb-1","input":"hi"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		return w.Code
	}

	if code := post(); code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", code)
	}
	conn, err := database.GetConnection("emb-reset")
	if err != nil {
		t.Fatal(err)
	}
	if conn.Data.RateLimitedUntil == "" || conn.Data.BackoffLevel == 0 || conn.Data.TestStatus != "error" {
		t.Fatalf("429 must persist error state: rateLimitedUntil=%q backoff=%d testStatus=%q",
			conn.Data.RateLimitedUntil, conn.Data.BackoffLevel, conn.Data.TestStatus)
	}

	// Simulate the cooldown expiring (backoff level + status stay stale).
	conn.Data.RateLimitedUntil = ""
	if err := database.UpdateConnectionHealth(conn.ID, conn.Data); err != nil {
		t.Fatal(err)
	}

	if code := post(); code != http.StatusOK {
		t.Fatalf("expected 200 after throttle clears, got %d", code)
	}
	conn, err = database.GetConnection("emb-reset")
	if err != nil {
		t.Fatal(err)
	}
	if conn.Data.BackoffLevel != 0 || conn.Data.TestStatus != "active" || conn.Data.LastError != "" {
		t.Fatalf("success must reset error state: backoff=%d testStatus=%q lastError=%q",
			conn.Data.BackoffLevel, conn.Data.TestStatus, conn.Data.LastError)
	}
}
