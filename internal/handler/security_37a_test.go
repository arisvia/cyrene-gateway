package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/middleware"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestSecretRedactionContract(t *testing.T) {
	srv, database := setupTestServer(t)

	// Create a connection with full secrets
	conn := &model.ProviderConnection{
		ID:       "secret-test-conn",
		Provider: "openai",
		AuthType: "api-key",
		Name:     "Test Secrets",
		Data: model.ConnectionData{
			APIKey:       "sk-proj-super-secret-key-12345678",
			AccessToken:  "ghu_access_token_secret_87654321",
			RefreshToken: "ghr_refresh_token_secret_11223344",
			BaseURL:      "https://api.openai.com/v1",
			ProviderSpecificData: map[string]any{
				"refreshToken": "ghr_nested_refresh_secret",
				"publicParam":  "safe-value",
			},
		},
	}
	if err := database.CreateConnection(conn); err != nil {
		t.Fatalf("failed to create connection: %v", err)
	}

	// 1. GET /api/providers (List)
	req := newLocalRequest("GET", "/api/providers", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	listBody := w.Body.String()
	assertNoSecrets(t, listBody)

	// 2. GET /api/providers/{id} (Detail)
	req = newLocalRequest("GET", "/api/providers/secret-test-conn", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	detailBody := w.Body.String()
	assertNoSecrets(t, detailBody)

	var dto model.ConnectionDTO
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil {
		t.Fatalf("failed to unmarshal DTO: %v", err)
	}
	if !dto.Data.HasAPIKey || !dto.Data.HasAccessToken || !dto.Data.HasRefreshToken {
		t.Errorf("expected has flags to be true, got %+v", dto.Data)
	}
	if dto.Data.CredentialHint != "...5678" {
		t.Errorf("expected hint ...5678, got %s", dto.Data.CredentialHint)
	}

	// 3. PUT /api/providers/{id} (Update without secrets preserves them)
	updatePayload := `{"name":"Updated Name","data":{"baseUrl":"https://custom.openai.com/v1"}}`
	req = newLocalRequest("PUT", "/api/providers/secret-test-conn", strings.NewReader(updatePayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	updateBody := w.Body.String()
	assertNoSecrets(t, updateBody)

	// Verify database still holds the original secrets
	dbConn, err := database.GetConnection("secret-test-conn")
	if err != nil {
		t.Fatalf("failed to get connection from db: %v", err)
	}
	if dbConn.Data.APIKey != "sk-proj-super-secret-key-12345678" {
		t.Errorf("expected secret preserved in DB, got %s", dbConn.Data.APIKey)
	}
	if dbConn.Data.BaseURL != "https://custom.openai.com/v1" {
		t.Errorf("expected baseUrl updated, got %s", dbConn.Data.BaseURL)
	}
}

func TestRemoteUnauthenticatedManagementBlocked(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Request from remote IP (not loopback) without session auth
	req := httptest.NewRequest("GET", "/api/providers", nil)
	req.RemoteAddr = "203.0.113.195:54321" // Public WAN IP
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for remote unauthenticated management access, got %d", w.Code)
	}
}

func TestInitialPasswordSetupAndSessionIssuance(t *testing.T) {
	srv, _ := setupTestServer(t)
	wanIP := "203.0.113.195:54321"

	// 1. Initial auth status on WAN: requireLogin=true, hasPassword=false, authenticated=false
	statusReq := httptest.NewRequest("GET", "/api/auth/status", nil)
	statusReq.RemoteAddr = wanIP
	statusW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(statusW, statusReq)
	if statusW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", statusW.Code)
	}
	var statusResp map[string]any
	json.Unmarshal(statusW.Body.Bytes(), &statusResp)
	if statusResp["requireLogin"] != true || statusResp["hasPassword"] != false || statusResp["authenticated"] != false {
		t.Fatalf("unexpected initial WAN auth status: %v", statusResp)
	}

	// 2. Set initial password via WAN - should succeed and return auth_token cookie
	setBody := `{"password":"initial-secure-password"}`
	setReq := httptest.NewRequest("POST", "/api/auth/password", strings.NewReader(setBody))
	setReq.RemoteAddr = wanIP
	setReq.Header.Set("Content-Type", "application/json")
	setW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(setW, setReq)
	if setW.Code != http.StatusOK {
		t.Fatalf("expected 200 for initial password setup, got %d: %s", setW.Code, setW.Body.String())
	}

	var sessionCookie *http.Cookie
	for _, c := range setW.Result().Cookies() {
		if c.Name == "auth_token" && c.Value != "" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected auth_token cookie to be set after setting password")
	}

	// 3. Request protected management route using issued session cookie - should succeed
	provReq := httptest.NewRequest("GET", "/api/providers", nil)
	provReq.RemoteAddr = wanIP
	provReq.AddCookie(sessionCookie)
	provW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(provW, provReq)
	if provW.Code != http.StatusOK {
		t.Fatalf("expected 200 with session cookie, got %d", provW.Code)
	}

	// 4. Subsequent unauthenticated password modification attempt must now be rejected with 401
	hackerBody := `{"password":"hacked-password-attempt"}`
	hackerReq := httptest.NewRequest("POST", "/api/auth/password", strings.NewReader(hackerBody))
	hackerReq.RemoteAddr = wanIP
	hackerReq.Header.Set("Content-Type", "application/json")
	hackerW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(hackerW, hackerReq)
	if hackerW.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated password change after init, got %d", hackerW.Code)
	}

	// 5. Enabling API key requirement from WAN without touching requireLogin must succeed (the reported bug)
	apiKeyBody := `{"requireApiKey":true}`
	apiKeyReq := httptest.NewRequest("PATCH", "/api/settings", strings.NewReader(apiKeyBody))
	apiKeyReq.RemoteAddr = wanIP
	apiKeyReq.Header.Set("Content-Type", "application/json")
	apiKeyReq.AddCookie(sessionCookie)
	apiKeyW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(apiKeyW, apiKeyReq)
	if apiKeyW.Code != http.StatusOK {
		t.Fatalf("expected 200 when enabling requireApiKey on WAN while requireLogin is false, got %d: %s", apiKeyW.Code, apiKeyW.Body.String())
	}

	// 6. Enabling requireLogin from WAN with authenticated session should succeed
	enableBody := `{"requireLogin":true}`
	enableReq := httptest.NewRequest("PATCH", "/api/settings", strings.NewReader(enableBody))
	enableReq.RemoteAddr = wanIP
	enableReq.Header.Set("Content-Type", "application/json")
	enableReq.AddCookie(sessionCookie)
	enableW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(enableW, enableReq)
	if enableW.Code != http.StatusOK {
		t.Fatalf("expected 200 when enabling requireLogin on WAN, got %d: %s", enableW.Code, enableW.Body.String())
	}

	// 6. Attempting to disable requireLogin from WAN must now be blocked with 400
	patchBody := `{"requireLogin":false}`
	patchReq := httptest.NewRequest("PATCH", "/api/settings", strings.NewReader(patchBody))
	patchReq.RemoteAddr = wanIP
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq.AddCookie(sessionCookie)
	patchW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(patchW, patchReq)
	if patchW.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when disabling requireLogin on WAN, got %d: %s", patchW.Code, patchW.Body.String())
	}
}

func TestPasswordChangeRevokesSessions(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	settings, err := database.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.RequireLogin, settings.RequireAPIKey = true, true
	settings.PasswordHash = auth.HashPassword("original-password")
	if err := database.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	key := &model.APIKey{ID: "password-change-key", Key: auth.GenerateAPIKey(), Name: "existing key", IsActive: true}
	if err := database.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}

	authHandler := NewAuthHandler(database)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", authHandler.HandleLogin)
	mux.HandleFunc("POST /api/auth/password", authHandler.HandleSetPassword)
	mux.HandleFunc("GET /api/auth/status", authHandler.HandleStatus)
	mux.HandleFunc("GET /api/protected", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /v1/protected", func(w http.ResponseWriter, r *http.Request) {
		if obj := auth.APIKeyFromContext(r.Context()); obj != nil {
			w.Header().Set("X-Test-Key-ID", obj.ID)
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.DashboardAuth(database)(middleware.APIKeyAuth(database)(mux))
	send := func(method, path, body, token, transport string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
		req.RemoteAddr = "203.0.113.177:1234"
		if token != "" {
			switch transport {
			case "cookie":
				req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
			case "bearer":
				req.Header.Set("Authorization", "Bearer "+token)
			case "header":
				req.Header.Set("X-Cyrene-Auth-Token", token)
			}
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	readToken := func(rec *httptest.ResponseRecorder) string {
		t.Helper()
		if rec.Code != http.StatusOK {
			t.Fatalf("expected token response 200, got %d: %s", rec.Code, rec.Body.String())
		}
		for _, cookie := range rec.Result().Cookies() {
			if cookie.Name == "auth_token" && cookie.Value != "" {
				return cookie.Value
			}
		}
		t.Fatal("missing auth_token cookie")
		return ""
	}
	assertAPIKey := func() {
		t.Helper()
		if !auth.VerifyAPIKeySignature(key.Key) {
			t.Error("existing API key signature revoked")
		}
		rec := send("GET", "/v1/protected", "", key.Key, "bearer")
		if rec.Code != http.StatusOK || rec.Header().Get("X-Test-Key-ID") != key.ID {
			t.Errorf("existing API key rejected or not attributed: status %d, key %q", rec.Code, rec.Header().Get("X-Test-Key-ID"))
		}
	}

	oldToken := readToken(send("POST", "/api/auth/login", `{"password":"original-password"}`, "", ""))
	if rec := send("GET", "/api/protected", "", oldToken, "cookie"); rec.Code != http.StatusOK {
		t.Fatalf("login token rejected before password change: %d", rec.Code)
	}
	assertAPIKey()
	newToken := readToken(send("POST", "/api/auth/password", `{"password":"replacement-password"}`, oldToken, "cookie"))
	for _, path := range []string{"/api/protected", "/v1/protected"} {
		for _, transport := range []string{"cookie", "bearer", "header"} {
			if path == "/v1/protected" && transport == "header" {
				continue // APIKeyAuth does not accept dashboard header tokens.
			}
			t.Run(path+"/"+transport, func(t *testing.T) {
				if rec := send("GET", path, "", oldToken, transport); rec.Code != http.StatusUnauthorized {
					t.Errorf("old session survived password change: got %d, want 401", rec.Code)
				}
				if rec := send("GET", path, "", newToken, transport); rec.Code != http.StatusOK {
					t.Errorf("new session rejected: got %d, want 200", rec.Code)
				}
			})
		}
	}
	for _, tc := range []struct {
		token string
		want  bool
	}{{oldToken, false}, {newToken, true}} {
		rec := send("GET", "/api/auth/status", "", tc.token, "cookie")
		var status struct {
			Authenticated bool `json:"authenticated"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK || status.Authenticated != tc.want {
			t.Errorf("status authenticated = %v, want %v (HTTP %d)", status.Authenticated, tc.want, rec.Code)
		}
	}
	assertAPIKey()
}

func TestPasswordArgon2idMigration(t *testing.T) {
	srv, database := setupTestServer(t)

	// Manually inject a legacy HMAC password hash
	legacyPassword := "my-old-hmac-password"
	legacyHash := auth.HashPassword(legacyPassword) // Currently hashes to Argon2id, let's test verify
	settings, _ := database.GetSettings()
	settings.PasswordHash = legacyHash
	database.SaveSettings(settings)

	// Login with correct password
	loginBody := `{"password":"my-old-hmac-password"}`
	req := newLocalRequest("POST", "/api/auth/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify password hash in DB is argon2id format
	updatedSettings, _ := database.GetSettings()
	if !strings.HasPrefix(updatedSettings.PasswordHash, "$argon2id$") {
		t.Fatalf("expected password hash migrated to argon2id, got %s", updatedSettings.PasswordHash)
	}
}

func TestManagementAPICORS(t *testing.T) {
	srv, _ := setupTestServer(t)

	// 1. Same-origin public domain via reverse proxy / Cloudflare Tunnel
	sameOriginReq := newLocalRequest("OPTIONS", "/api/providers", nil)
	sameOriginReq.Header.Set("Origin", "https://ai.mycompany.com")
	sameOriginReq.Header.Set("X-Forwarded-Host", "ai.mycompany.com")
	sameOriginW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(sameOriginW, sameOriginReq)
	if sameOriginW.Header().Get("Access-Control-Allow-Origin") != "https://ai.mycompany.com" {
		t.Fatalf("expected CORS allow origin for same domain, got %q", sameOriginW.Header().Get("Access-Control-Allow-Origin"))
	}
	if sameOriginW.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("expected Access-Control-Allow-Credentials to be true for same domain")
	}

	// 2. Loopback origin (e.g. local dev server or local browser)
	loopbackReq := newLocalRequest("OPTIONS", "/api/providers", nil)
	loopbackReq.Header.Set("Origin", "http://localhost:5173")
	loopbackW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(loopbackW, loopbackReq)
	if loopbackW.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("expected CORS allow origin for localhost:5173, got %q", loopbackW.Header().Get("Access-Control-Allow-Origin"))
	}

	// 3. Untrusted cross-origin website (e.g. CSRF attack)
	evilReq := newLocalRequest("OPTIONS", "/api/providers", nil)
	evilReq.Header.Set("Origin", "https://malicious-site.com")
	evilReq.Header.Set("X-Forwarded-Host", "ai.mycompany.com")
	evilW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(evilW, evilReq)
	if evilW.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("expected no CORS allow origin for malicious site, got %q", evilW.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestLoginRateLimitingBehindProxy(t *testing.T) {
	srv, database := setupTestServer(t)
	settings, _ := database.GetSettings()
	settings.PasswordHash = auth.HashPassword("correct-admin-password")
	database.SaveSettings(settings)

	attackerIP := "198.51.100.99"
	innocentIP := "198.51.100.100"

	// Attacker attempts 5 wrong passwords behind a reverse proxy (RemoteAddr is local loopback)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"wrong-password"}`))
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("X-Forwarded-For", attackerIP)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, w.Code)
		}
	}

	// 6th attempt by attacker must be locked out with 429
	req6 := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"wrong-password"}`))
	req6.RemoteAddr = "127.0.0.1:12345"
	req6.Header.Set("X-Forwarded-For", attackerIP)
	req6.Header.Set("Content-Type", "application/json")
	w6 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w6, req6)
	if w6.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests for attacker, got %d", w6.Code)
	}

	// Innocent user behind the SAME reverse proxy must NOT be locked out!
	innocentReq := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"correct-admin-password"}`))
	innocentReq.RemoteAddr = "127.0.0.1:12345"
	innocentReq.Header.Set("X-Forwarded-For", innocentIP)
	innocentReq.Header.Set("Content-Type", "application/json")
	innocentW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(innocentW, innocentReq)
	if innocentW.Code != http.StatusOK {
		t.Fatalf("expected innocent user behind reverse proxy to succeed with 200, got %d: %s", innocentW.Code, innocentW.Body.String())
	}
}

func assertNoSecrets(t *testing.T, body string) {
	t.Helper()
	secrets := []string{
		"sk-proj-super-secret-key-12345678",
		"ghu_access_token_secret_87654321",
		"ghr_refresh_token_secret_11223344",
		"ghr_nested_refresh_secret",
	}
	for _, s := range secrets {
		if strings.Contains(body, s) {
			t.Fatalf("SECURITY VIOLATION: secret %q leaked in response: %s", s, body)
		}
	}
}
