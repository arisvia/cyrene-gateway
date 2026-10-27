package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

func setupTestDB(t *testing.T) (*db.DB, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "cyrene-mw-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	database, err := db.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("failed to init db: %v", err)
	}
	cleanup := func() {
		database.Close()
		os.RemoveAll(dir)
	}
	return database, cleanup
}

func TestAPIKeyAuth_OpenGatewayMode(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	// Settings: RequireAPIKey = false (default open mode)
	st, _ := database.GetSettings()
	st.RequireAPIKey = false
	_ = database.SaveSettings(st)

	// Create one valid active key with signature
	keyStr := auth.GenerateAPIKey()
	validKey := &model.APIKey{
		ID:        "key-open-1",
		Key:       keyStr,
		Name:      "Valid User",
		IsActive:  true,
		ExpiresAt: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
	}
	if err := database.CreateAPIKey(validKey); err != nil {
		t.Fatalf("failed to create key: %v", err)
	}

	var capturedKey *model.APIKey
	handler := APIKeyAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = auth.APIKeyFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Case 1: Non-v1 path passes through
	{
		capturedKey = nil
		req := httptest.NewRequest("GET", "/health", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for non-v1 path, got %d", rec.Code)
		}
	}

	// Case 2: Open gateway + no header -> 200 OK (anonymous)
	{
		capturedKey = nil
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for anonymous in open mode, got %d", rec.Code)
		}
		if capturedKey != nil {
			t.Errorf("expected nil capturedKey, got %v", capturedKey)
		}
	}

	// Case 3: Open gateway + garbage Bearer key -> 200 OK (pass-through as anonymous, NOT 401)
	{
		capturedKey = nil
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer invalid-garbage-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 pass-through for garbage key in open mode, got %d", rec.Code)
		}
		if capturedKey != nil {
			t.Errorf("expected nil capturedKey, got %v", capturedKey)
		}
	}

	// Case 4: Open gateway + valid Bearer key -> 200 OK with Key injected into Context
	{
		capturedKey = nil
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer "+validKey.Key)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for valid key, got %d", rec.Code)
		}
		if capturedKey == nil || capturedKey.ID != validKey.ID {
			t.Errorf("expected capturedKey ID %s, got %v", validKey.ID, capturedKey)
		}
	}
}

func TestAPIKeyAuth_StrictGatewayMode(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	// Settings: RequireAPIKey = true
	st, _ := database.GetSettings()
	st.RequireAPIKey = true
	_ = database.SaveSettings(st)

	keyStr := auth.GenerateAPIKey()
	validKey := &model.APIKey{
		ID:        "key-strict-1",
		Key:       keyStr,
		Name:      "Strict User",
		IsActive:  true,
		ExpiresAt: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
	}
	if err := database.CreateAPIKey(validKey); err != nil {
		t.Fatalf("failed to create key: %v", err)
	}

	var capturedKey *model.APIKey
	handler := APIKeyAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = auth.APIKeyFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Case 1: Strict mode + no header -> 401
	{
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for missing key in strict mode, got %d", rec.Code)
		}
	}

	// Case 2: Strict mode + invalid key -> 401
	{
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer cg-invalidkey123")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for invalid key in strict mode, got %d", rec.Code)
		}
	}

	// Case 3: Strict mode + valid key -> 200 with context
	{
		capturedKey = nil
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer "+validKey.Key)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for valid key in strict mode, got %d", rec.Code)
		}
		if capturedKey == nil || capturedKey.ID != validKey.ID {
			t.Errorf("expected capturedKey ID %s, got %v", validKey.ID, capturedKey)
		}
	}

	// Case 4: Key exists in DB but signature invalid (e.g. secret rotated) -> still passes
	{
		unsigKey := &model.APIKey{
			ID:        "key-unsig-1",
			Key:       "cg-oldkeypayload.invalidsignature",
			Name:      "Rotated Secret User",
			IsActive:  true,
			ExpiresAt: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
		}
		if err := database.CreateAPIKey(unsigKey); err != nil {
			t.Fatalf("failed to create key: %v", err)
		}
		capturedKey = nil
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer "+unsigKey.Key)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for DB key with mismatched signature, got %d", rec.Code)
		}
		if capturedKey == nil || capturedKey.ID != unsigKey.ID {
			t.Errorf("expected capturedKey ID %s, got %v", unsigKey.ID, capturedKey)
		}
	}

	// Case 5: Same-origin browser request to GET /v1/models without API key when login disabled
	{
		req := httptest.NewRequest("GET", "/v1/models", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		req.Host = "localhost:20128"
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for same-origin dashboard request, got %d", rec.Code)
		}
	}
}

// TestAPIKeyAuth_GeminiSurface locks the Gemini inbound surface: official Google
// SDK clients send x-goog-api-key and REST callers use ?key=. Both must clear
// APIKeyAuth on /v1beta/* (the middleware 401s before handleGeminiGenerateContent
// ever runs) and attribute the resolved key into context.
func TestAPIKeyAuth_GeminiSurface(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	st, _ := database.GetSettings()
	st.RequireAPIKey = true
	_ = database.SaveSettings(st)

	validKey := &model.APIKey{
		ID:        "key-gemini-1",
		Key:       auth.GenerateAPIKey(),
		Name:      "Gemini User",
		IsActive:  true,
		ExpiresAt: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
	}
	if err := database.CreateAPIKey(validKey); err != nil {
		t.Fatalf("failed to create key: %v", err)
	}

	var capturedKey *model.APIKey
	handler := APIKeyAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = auth.APIKeyFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		name string
		req  *http.Request
	}{
		{
			name: "x-goog-api-key header",
			req: func() *http.Request {
				r := httptest.NewRequest("POST", "/v1beta/models/gemini-2.5-flash:generateContent", nil)
				r.Header.Set("x-goog-api-key", validKey.Key)
				return r
			}(),
		},
		{
			name: "rest ?key= query",
			req:  httptest.NewRequest("POST", "/v1beta/models/gemini-2.5-flash:generateContent?key="+validKey.Key, nil),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capturedKey = nil
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, tc.req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 for %s, got %d", tc.name, rec.Code)
			}
			if capturedKey == nil || capturedKey.ID != validKey.ID {
				t.Fatalf("expected context key %s for %s, got %v", validKey.ID, tc.name, capturedKey)
			}
		})
	}

	// Missing credential on the Gemini surface still 401s in strict mode.
	t.Run("missing key 401", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1beta/models/gemini-2.5-flash:generateContent", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for missing gemini key, got %d", rec.Code)
		}
	})
}

func TestDashboardAuth_NetworkEvidence(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	settings, err := database.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.RequireLogin = false
	if err := database.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	handler := DashboardAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, tc := range []struct {
		name, remote, host string
		headers            http.Header
		want               int
	}{
		{"local IPv4", "127.0.0.1:1234", "localhost:20128", nil, 200},
		{"local IPv6", "[::1]:1234", "[::1]:20128", nil, 200},
		{"LAN", "192.168.1.10:1234", "gateway.lan", nil, 200},
		{"LAN IPv6", "[fd00::2]:1234", "[fd00::1]:20128", nil, 200},
		{"overlay", "100.64.0.5:1234", "my-server", nil, 200},
		{"local proxy LAN chain", "127.0.0.1:1234", "gateway.local", http.Header{"X-Forwarded-For": {"192.168.1.10, 10.0.0.2"}, "X-Forwarded-Host": {"gateway.lan"}}, 200},
		{"WAN IPv4", "203.0.113.5:1234", "localhost", nil, 401},
		{"WAN IPv6", "[2606:4700:4700::1111]:1234", "localhost", nil, 401},
		{"WAN mapped IPv6", "[::ffff:203.0.113.5]:1234", "localhost", nil, 401},
		{"missing peer", "", "localhost", nil, 401},
		{"hostname peer", "localhost:1234", "localhost", nil, 401},
		{"invalid peer", "not-an-ip:1234", "localhost", nil, 401},
		{"unspecified peer", "[::]:1234", "localhost", nil, 401},
		{"test peer is not LAN", "192.0.2.1:1234", "localhost", nil, 401},
		{"WAN cannot spoof LAN", "203.0.113.5:1234", "localhost", http.Header{"X-Forwarded-For": {"127.0.0.1"}, "Cf-Connecting-Ip": {"127.0.0.1"}}, 401},
		{"XFF private prefix", "127.0.0.1:1234", "localhost", http.Header{"X-Forwarded-For": {"127.0.0.1, 203.0.113.5, 10.0.0.2"}}, 401},
		{"XFF multiple lines", "10.0.0.2:1234", "localhost", http.Header{"X-Forwarded-For": {"127.0.0.1", "203.0.113.5, 10.0.0.1"}}, 401},
		{"CF cannot mask XFF", "127.0.0.1:1234", "localhost", http.Header{"X-Forwarded-For": {"203.0.113.5"}, "Cf-Connecting-Ip": {"127.0.0.1"}}, 401},
		{"CF cannot mask real IP", "127.0.0.1:1234", "localhost", http.Header{"X-Real-Ip": {"203.0.113.5"}, "Cf-Connecting-Ip": {"127.0.0.1"}}, 401},
		{"XFF cannot mask CF", "127.0.0.1:1234", "localhost", http.Header{"X-Forwarded-For": {"127.0.0.1"}, "Cf-Connecting-Ip": {"203.0.113.5"}}, 401},
		{"invalid XFF", "127.0.0.1:1234", "localhost", http.Header{"X-Forwarded-For": {"unknown, 10.0.0.2"}}, 401},
		{"invalid CF", "127.0.0.1:1234", "localhost", http.Header{"Cf-Connecting-Ip": {"unknown"}}, 401},
		{"public host cannot be replaced", "127.0.0.1:1234", "gateway.example.net", http.Header{"X-Forwarded-Host": {"localhost"}}, 401},
		{"example host is public", "127.0.0.1:1234", "example.com", nil, 401},
		{"forwarded public host", "127.0.0.1:1234", "localhost", http.Header{"X-Forwarded-Host": {"gateway.example.net"}}, 401},
		{"forwarded host chain", "127.0.0.1:1234", "localhost", http.Header{"X-Forwarded-Host": {"gateway.example.net, gateway.lan"}}, 401},
		{"forwarded host multiple lines", "127.0.0.1:1234", "localhost", http.Header{"X-Forwarded-Host": {"localhost", "gateway.example.net"}}, 401},
		{"missing host", "127.0.0.1:1234", "", nil, 401},
		{"invalid host", "127.0.0.1:1234", "[not-ip]", nil, 401},
		{"public origin", "127.0.0.1:1234", "localhost", http.Header{"Origin": {"https://evil.example.net"}}, 401},
		{"opaque origin", "127.0.0.1:1234", "localhost", http.Header{"Origin": {"null"}}, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
			req.RemoteAddr, req.Host = tc.remote, tc.host
			for name, values := range tc.headers {
				for _, value := range values {
					req.Header.Add(name, value)
				}
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("got status %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestDashboardAuth_SecurityAndRebinding(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	// Settings: RequireLogin = false (default local-first mode)
	st, _ := database.GetSettings()
	st.RequireLogin = false
	_ = database.SaveSettings(st)

	handler := DashboardAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))

	// 1. Trusted loopback request -> 200 OK
	{
		req := httptest.NewRequest("GET", "/api/providers", nil)
		req.RemoteAddr = "127.0.0.1:54321"
		req.Host = "localhost:20128"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for trusted loopback, got %d", rec.Code)
		}
	}

	// 2. DNS rebinding attack (Host header evil.com on loopback IP) -> 401 Unauthorized
	{
		req := httptest.NewRequest("GET", "/api/providers", nil)
		req.RemoteAddr = "127.0.0.1:54321"
		req.Host = "evil.com:20128"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for DNS rebinding attack with Host evil.com, got %d", rec.Code)
		}
	}

	// 3. Cross-origin attack (Origin header evil.com on loopback IP) -> 401 Unauthorized
	{
		req := httptest.NewRequest("GET", "/api/keys", nil)
		req.RemoteAddr = "127.0.0.1:54321"
		req.Host = "localhost:20128"
		req.Header.Set("Origin", "http://evil.com")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for cross-origin request from evil.com, got %d", rec.Code)
		}
	}

	// 4. Remote non-loopback IP -> 401 Unauthorized
	{
		req := httptest.NewRequest("GET", "/api/settings", nil)
		req.RemoteAddr = "203.0.113.195:43210"
		req.Host = "localhost:20128"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for remote non-loopback IP, got %d", rec.Code)
		}
	}

	// 5. Public path (/api/health) -> 200 OK even from remote IP with evil Host
	{
		req := httptest.NewRequest("GET", "/api/health", nil)
		req.RemoteAddr = "203.0.113.195:43210"
		req.Host = "evil.com:20128"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for public /api/health, got %d", rec.Code)
		}
	}

	// 6. Valid session token -> 200 OK for remote IP
	{
		st.PasswordHash = auth.HashPassword("test-password")
		if err := database.SaveSettings(st); err != nil {
			t.Fatal(err)
		}
		token, _ := auth.CreateSessionToken(st.PasswordHash)
		req := httptest.NewRequest("GET", "/api/providers", nil)
		req.RemoteAddr = "203.0.113.195:43210"
		req.Host = "mygateway.example.com"
		req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for authenticated remote request, got %d", rec.Code)
		}
	}
}

func TestAPIKeyAuth_FailClosedOnDatabaseError(t *testing.T) {
	database, cleanup := setupTestDB(t)
	// Close the DB immediately so any query inside APIKeyAuth fails
	cleanup()

	nextCalled := false
	handler := APIKeyAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	// 1. /v1/* route must fail-closed with 503 Service Unavailable, NOT 200
	reqV1 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	recV1 := httptest.NewRecorder()
	handler.ServeHTTP(recV1, reqV1)

	if recV1.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable on database error, got %d", recV1.Code)
	}
	if nextCalled {
		t.Fatalf("next handler must not be called when database settings fails")
	}

	// 2. Non-v1 route is not intercepted by APIKeyAuth
	reqNonV1 := httptest.NewRequest("GET", "/health", nil)
	recNonV1 := httptest.NewRecorder()
	handler.ServeHTTP(recNonV1, reqNonV1)
	if recNonV1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for non-v1 path, got %d", recNonV1.Code)
	}
	if !nextCalled {
		t.Fatalf("expected next handler to be called for non-v1 path")
	}
}
