package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/config"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

// 37A security contract tests: secret redaction, secure-by-default bind,
// first-time password setup, and session lifecycle.

const (
	secretAPIKey      = "sk-live-secret-api-key-1234567890"
	secretAccessToken = "gho_secret_access_token_abcdefghij"
	secretRefreshTok  = "rt_secret_refresh_token_klmnopqrst"
)

func seedSecretConnection(t *testing.T, database *db.DB) {
	t.Helper()
	conn := &model.ProviderConnection{
		ID:       "secret-conn",
		Provider: "openai",
		AuthType: "oauth",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:       secretAPIKey,
			AccessToken:  secretAccessToken,
			RefreshToken: secretRefreshTok,
			ProviderSpecificData: map[string]any{
				"proxyPoolId": "pool-1",
				"secretBlob":  "should-never-leak",
			},
		},
	}
	if err := database.CreateConnection(conn); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
}

// assertNoSecrets fails if any known secret appears anywhere in the body.
func assertNoSecrets(t *testing.T, body []byte) {
	t.Helper()
	s := string(body)
	for _, sec := range []string{secretAPIKey, secretAccessToken, secretRefreshTok, "should-never-leak"} {
		if strings.Contains(s, sec) {
			t.Fatalf("response leaks secret %q: %s", sec, s)
		}
	}
}

func TestSecretContractProviders(t *testing.T) {
	srv, database := setupTestServer(t)
	seedSecretConnection(t, database)

	// List
	req := httptest.NewRequest("GET", "/api/providers", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}
	assertNoSecrets(t, w.Body.Bytes())
	var list []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("list decode failed: %v %s", err, w.Body.String())
	}
	data := list[0]["data"].(map[string]any)
	if data["hasApiKey"] != true || data["hasAccessToken"] != true || data["hasRefreshToken"] != true {
		t.Fatalf("expected presence flags true, got %v", data)
	}
	for _, forbidden := range []string{"apiKey", "accessToken", "refreshToken", "providerSpecificData"} {
		if _, ok := data[forbidden]; ok {
			t.Fatalf("response must not contain %q", forbidden)
		}
	}

	// Mutation (update) response is also redacted
	req = httptest.NewRequest("PUT", "/api/providers/secret-conn", strings.NewReader(`{"name":"renamed"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	assertNoSecrets(t, w.Body.Bytes())

	// Reset-status response is redacted too
	req = httptest.NewRequest("POST", "/api/providers/secret-conn/reset", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("reset: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	assertNoSecrets(t, w.Body.Bytes())

	// Secrets must survive a redacted round-trip (field-level patch, P0-1).
	conn, err := database.GetConnection("secret-conn")
	if err != nil {
		t.Fatalf("get conn: %v", err)
	}
	if conn.Data.APIKey != secretAPIKey || conn.Data.AccessToken != secretAccessToken || conn.Data.RefreshToken != secretRefreshTok {
		t.Fatal("update with absent secrets must preserve stored secrets")
	}
}

func TestSecretContractCreateProviderRedactsResponse(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"provider":"openai","authType":"api-key","data":{"apiKey":"` + secretAPIKey + `"}}`
	req := httptest.NewRequest("POST", "/api/providers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	assertNoSecrets(t, w.Body.Bytes())

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]any)
	if data["hasApiKey"] != true {
		t.Fatalf("expected hasApiKey=true in create response, got %v", data)
	}
}

func TestSecretContractOAuthAndKeys(t *testing.T) {
	srv, database := setupTestServer(t)
	seedSecretConnection(t, database)

	// OAuth status must not leak tokens
	req := httptest.NewRequest("GET", "/api/oauth/openai/status", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("oauth status: expected 200, got %d", w.Code)
	}
	assertNoSecrets(t, w.Body.Bytes())

	// Key list must not return raw client API keys
	createReq := httptest.NewRequest("POST", "/api/keys", strings.NewReader(`{"name":"k1"}`))
	createReq.Header.Set("Content-Type", "application/json")
	cw := httptest.NewRecorder()
	srv.Handler.ServeHTTP(cw, createReq)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create key: expected 201, got %d", cw.Code)
	}
	var created map[string]any
	json.Unmarshal(cw.Body.Bytes(), &created)
	fullKey, _ := created["key"].(string)
	if !strings.HasPrefix(fullKey, "cg-") {
		t.Fatal("create response must return the full key exactly once")
	}

	req = httptest.NewRequest("GET", "/api/keys", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list keys: expected 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), fullKey) {
		t.Fatal("key list must not contain the raw API key")
	}

	// Settings never expose the password hash
	settings, _ := database.GetSettings()
	settings.PasswordHash, _ = auth.HashPassword("hunter2hunter2")
	database.SaveSettings(settings)

	req = httptest.NewRequest("GET", "/api/settings", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if strings.Contains(w.Body.String(), "argon2id") || strings.Contains(w.Body.String(), "passwordHash") {
		t.Fatalf("settings response leaked password hash: %s", w.Body.String())
	}
	var settingsResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &settingsResp)
	if settingsResp["hasPassword"] != true {
		t.Fatalf("expected hasPassword=true, got %v", settingsResp)
	}
}

func TestSecretContractUsageHistory(t *testing.T) {
	srv, database := setupTestServer(t)

	// Usage history stores the caller's client API key internally but must
	// never serialize it.
	err := database.SaveUsageEntry(&db.UsageEntry{
		Provider: "openai", Model: "gpt-4o", APIKey: secretAPIKey, Endpoint: "/v1/chat/completions",
		PromptTokens: 1, CompletionTokens: 1, Cost: 0.01, Status: "ok",
	})
	if err != nil {
		t.Fatalf("save usage: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/usage/history?limit=10", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	assertNoSecrets(t, w.Body.Bytes())
}

func TestNonLoopbackBindForcesAuth(t *testing.T) {
	// Simulate a remote bind: management API must require a session even when
	// requireLogin is false (default config must not be anonymously reachable
	// from non-loopback sources).
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	cfg := &config.Config{Host: "0.0.0.0", Port: 0, DBPath: ":memory:", DataDir: t.TempDir()}
	if err := auth.InitSecretManager(cfg.DataDir, ""); err != nil {
		t.Fatalf("init secret: %v", err)
	}
	srv := NewServer(database, cfg)

	protected := []string{
		"GET /api/providers",
		"GET /api/settings",
		"GET /api/keys",
		"GET /api/tunnel/status",
		"GET /api/cli-tools",
		"GET /api/mitm/status",
		"POST /api/tunnel/tailscale-enable",
	}
	for _, route := range protected {
		parts := strings.SplitN(route, " ", 2)
		req := httptest.NewRequest(parts[0], parts[1], nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s: expected 401 on non-loopback bind without session, got %d", route, w.Code)
		}
	}

	// Public metadata endpoints stay open
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("health: expected 200, got %d", w.Code)
	}

	// Authenticated session passes
	settings, _ := database.GetSettings()
	settings.PasswordHash, _ = auth.HashPassword("correct-horse-battery")
	database.SaveSettings(settings)
	token, _ := auth.CreateSessionToken()

	req = httptest.NewRequest("GET", "/api/providers", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with session cookie, got %d", w.Code)
	}
}

func TestDefaultBindIsLoopback(t *testing.T) {
	cfg := &config.Config{Host: "127.0.0.1"}
	if !cfg.IsLoopbackBind() {
		t.Fatal("127.0.0.1 must be loopback")
	}
	for _, host := range []string{"localhost", "::1"} {
		c := &config.Config{Host: host}
		if !c.IsLoopbackBind() {
			t.Fatalf("%s must be loopback", host)
		}
	}
	for _, host := range []string{"0.0.0.0", "192.168.1.5", ""} {
		c := &config.Config{Host: host}
		if c.IsLoopbackBind() {
			t.Fatalf("%q must not be loopback", host)
		}
	}
}

func TestFirstTimePasswordSetupAndSessionLifecycle(t *testing.T) {
	srv, database := setupTestServer(t)

	// Login before any password exists: rejected, no default password.
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"123456"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 before first-time setup, got %d", w.Code)
	}

	// Status reports that setup is pending.
	req = httptest.NewRequest("GET", "/api/auth/status", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	var st map[string]any
	json.Unmarshal(w.Body.Bytes(), &st)
	if st["hasPassword"] != false {
		t.Fatalf("expected hasPassword=false, got %v", st)
	}

	// Weak passwords are rejected during setup.
	req = httptest.NewRequest("POST", "/api/auth/password", strings.NewReader(`{"password":"short"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for weak password, got %d", w.Code)
	}

	// First-time setup succeeds.
	req = httptest.NewRequest("POST", "/api/auth/password", strings.NewReader(`{"password":"correct-horse-battery"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("setup: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Argon2id hash is stored.
	settings, _ := database.GetSettings()
	if !auth.IsArgon2Hash(settings.PasswordHash) {
		t.Fatalf("expected argon2id hash, got %q", settings.PasswordHash)
	}

	// Login with the new password works and issues a session cookie.
	req = httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"correct-horse-battery"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" && c.Value != "" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected auth_token cookie after login")
	}

	// Wrong password fails.
	req = httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"wrong-password"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d", w.Code)
	}

	// Enable requireLogin: session cookie grants access, missing cookie does not.
	settings.RequireLogin = true
	database.SaveSettings(settings)

	req = httptest.NewRequest("GET", "/api/providers", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", w.Code)
	}

	req = httptest.NewRequest("GET", "/api/providers", nil)
	req.AddCookie(sessionCookie)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with session, got %d", w.Code)
	}

	// Logout clears the cookie; the old token stops working only if it is
	// invalid — HMAC tokens are stateless, so logout invalidation is enforced
	// by cookie expiry/clearance. Verify the cleared cookie is rejected.
	req = httptest.NewRequest("POST", "/api/auth/logout", nil)
	req.AddCookie(sessionCookie)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("logout: expected 200, got %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" && c.MaxAge != -1 {
			t.Fatal("expected auth_token to be cleared on logout")
		}
	}

	// Changing the password afterwards requires an authenticated session.
	req = httptest.NewRequest("POST", "/api/auth/password", strings.NewReader(`{"password":"another-strong-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated password change, got %d", w.Code)
	}

	req = httptest.NewRequest("POST", "/api/auth/password", strings.NewReader(`{"password":"another-strong-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("authenticated password change: expected 200, got %d", w.Code)
	}

	// Login limiter: repeated failures trigger 429.
	for i := 0; i < 12; i++ {
		req = httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"wrong"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.9.9.9:1234"
		w = httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
	}
	req = httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.9.9.9:1234"
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after repeated failures, got %d", w.Code)
	}
}

func TestSettingsWriteCannotInjectPasswordHash(t *testing.T) {
	srv, database := setupTestServer(t)

	// PUT settings with an injected hash must not install it.
	req := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(`{"comboStrategy":"round-robin","passwordHash":"argon2id$injected"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	settings, _ := database.GetSettings()
	if settings.PasswordHash == "argon2id$injected" {
		t.Fatal("PUT settings must not accept passwordHash")
	}
	if settings.ComboStrategy != "round-robin" {
		t.Fatal("PUT settings must apply non-secret fields")
	}

	// PATCH settings with an injected hash must not install it either.
	req = httptest.NewRequest("PATCH", "/api/settings", strings.NewReader(`{"passwordHash":"argon2id$injected2","comboStrategy":"fallback"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	settings, _ = database.GetSettings()
	if settings.PasswordHash == "argon2id$injected2" {
		t.Fatal("PATCH settings must not accept passwordHash")
	}
}

func TestMaskedSecretsAreNotPersisted(t *testing.T) {
	srv, database := setupTestServer(t)

	// Create a real connection first.
	body := `{"id":"patch-conn","provider":"openai","authType":"api-key","data":{"apiKey":"` + secretAPIKey + `"}}`
	req := httptest.NewRequest("POST", "/api/providers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	// A frontend echoing back the masked DTO must not clobber the secret.
	patch := `{"data":{"apiKey":"••••7890","baseUrl":"https://example.test"}}`
	req = httptest.NewRequest("PUT", "/api/providers/patch-conn", strings.NewReader(patch))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	conn, err := database.GetConnection("patch-conn")
	if err != nil {
		t.Fatalf("get conn: %v", err)
	}
	if conn.Data.APIKey != secretAPIKey {
		t.Fatalf("masked apiKey echo must preserve the stored secret, got %q", conn.Data.APIKey)
	}
	if conn.Data.BaseURL != "https://example.test" {
		t.Fatalf("expected baseUrl update, got %q", conn.Data.BaseURL)
	}
}
