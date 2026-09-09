package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGeneratePKCE(t *testing.T) {
	pkce, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE failed: %v", err)
	}
	if pkce.CodeVerifier == "" {
		t.Error("code verifier should not be empty")
	}
	if pkce.CodeChallenge == "" {
		t.Error("code challenge should not be empty")
	}
	if pkce.State == "" {
		t.Error("state should not be empty")
	}
	if pkce.CodeVerifier == pkce.State {
		t.Error("verifier and state should be different")
	}
}

func TestGeneratePKCE_Unique(t *testing.T) {
	p1, _ := GeneratePKCE()
	p2, _ := GeneratePKCE()
	if p1.CodeVerifier == p2.CodeVerifier {
		t.Error("two PKCE generations should produce different verifiers")
	}
	if p1.State == p2.State {
		t.Error("two PKCE generations should produce different states")
	}
}

func TestGetProviderFlowType(t *testing.T) {
	tests := []struct {
		provider string
		expected OAuthFlowType
	}{
		{"github", FlowDeviceCode},
		{"kimi", FlowDeviceCode},
		{"grok-cli", FlowDeviceCode},
		{"codebuddy-cn", FlowDeviceCode},
		{"codebuddy-intl", FlowDeviceCode},
		{"claude", FlowAuthorizationCodePKCE},
		{"codex", FlowAuthorizationCodePKCE},
		{"nonexistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := GetProviderFlowType(tt.provider)
			if got != tt.expected {
				t.Errorf("GetProviderFlowType(%q) = %q, want %q", tt.provider, got, tt.expected)
			}
		})
	}
}

func TestBuildAuthorizeURL_Claude(t *testing.T) {
	pkce, _ := GeneratePKCE()
	url, err := BuildAuthorizeURL("claude", "http://localhost:8080/callback", pkce)
	if err != nil {
		t.Fatalf("BuildAuthorizeURL failed: %v", err)
	}
	if url == "" {
		t.Fatal("URL should not be empty")
	}
	if !contains(url, "claude.ai/oauth/authorize") {
		t.Error("URL should contain claude authorize endpoint")
	}
	if !contains(url, "code_challenge=") {
		t.Error("URL should contain code_challenge")
	}
	if !contains(url, "state=") {
		t.Error("URL should contain state")
	}
}
func TestBuildAuthorizeURL_Antigravity(t *testing.T) {
	pkce, _ := GeneratePKCE()
	url, err := BuildAuthorizeURL("antigravity", "http://localhost:20128/api/oauth/antigravity/callback", pkce)
	if err != nil {
		t.Fatalf("BuildAuthorizeURL failed: %v", err)
	}
	if !contains(url, "code_challenge=") {
		t.Error("Antigravity URL should contain code_challenge")
	}
	if !contains(url, "code_challenge_method=S256") {
		t.Error("Antigravity URL should contain code_challenge_method=S256")
	}
}

func TestBuildAuthorizeURL_Codex(t *testing.T) {
	pkce, _ := GeneratePKCE()
	url, err := BuildAuthorizeURL("codex", "http://localhost:8080/callback", pkce)
	if err != nil {
		t.Fatalf("BuildAuthorizeURL failed: %v", err)
	}
	if !contains(url, "auth.openai.com/oauth/authorize") {
		t.Error("URL should contain OpenAI authorize endpoint")
	}
	if !contains(url, "code_challenge_method=S256") {
		t.Error("URL should contain S256 challenge method")
	}
}

func TestBuildAuthorizeURL_UnknownProvider(t *testing.T) {
	pkce, _ := GeneratePKCE()
	_, err := BuildAuthorizeURL("nonexistent", "http://localhost:8080/callback", pkce)
	if err == nil {
		t.Error("should return error for unknown provider")
	}
}

func TestImportToken_PlainToken(t *testing.T) {
	result, err := ImportToken("codex", "sk-plain-token-123")
	if err != nil {
		t.Fatalf("ImportToken failed: %v", err)
	}
	if result.AccessToken != "sk-plain-token-123" {
		t.Error("access token should match input")
	}
	if result.Email != "" {
		t.Error("email should be empty for non-JWT token")
	}
}

func TestImportToken_JWT(t *testing.T) {
	// Minimal JWT with email claim (header.payload.signature)
	// payload: {"email":"test@example.com","exp":9999999999}
	payload := "eyJlbWFpbCI6InRlc3RAZXhhbXBsZS5jb20iLCJleHAiOjk5OTk5OTk5OTl9"
	token := "eyJhbGciOiJSUzI1NiJ9." + payload + ".sig"

	result, err := ImportToken("codex", token)
	if err != nil {
		t.Fatalf("ImportToken failed: %v", err)
	}
	if result.Email != "test@example.com" {
		t.Errorf("expected email test@example.com, got %q", result.Email)
	}
}

func TestImportToken_Empty(t *testing.T) {
	_, err := ImportToken("codex", "")
	if err == nil {
		t.Error("should return error for empty token")
	}
}

func TestDecodeJWTPayload(t *testing.T) {
	// {"email":"user@test.com"}
	b64 := "eyJlbWFpbCI6InVzZXJAdGVzdC5jb20ifQ"
	payload, err := decodeJWTPayload(b64)
	if err != nil {
		t.Fatalf("decodeJWTPayload failed: %v", err)
	}
	if payload["email"] != "user@test.com" {
		t.Errorf("expected email user@test.com, got %v", payload["email"])
	}
}

func TestSessionStore(t *testing.T) {
	pkce, _ := GeneratePKCE()
	session := &OAuthSession{
		Provider:    "claude",
		PKCE:        pkce,
		RedirectURI: "http://localhost:8080/callback",
		CreatedAt:   time.Now(),
	}

	StoreSession(pkce.State, session)

	got, ok := GetSession(pkce.State)
	if !ok {
		t.Fatal("session should be found")
	}
	if got.Provider != "claude" {
		t.Errorf("expected provider claude, got %q", got.Provider)
	}

	ClearSession(pkce.State)
	_, ok = GetSession(pkce.State)
	if ok {
		t.Error("session should be cleared")
	}
}

func TestDedupRefresh_NoToken(t *testing.T) {
	called := 0
	result, err := DedupRefresh("test", "", func() (*RefreshResult, error) {
		called++
		return &RefreshResult{AccessToken: "new-token"}, nil
	})
	if err != nil {
		t.Fatalf("DedupRefresh failed: %v", err)
	}
	if result.AccessToken != "new-token" {
		t.Error("expected new-token")
	}
	if called != 1 {
		t.Errorf("expected fn called once, got %d", called)
	}
}

func TestRequestDeviceCode_CodeBuddy(t *testing.T) {
	orig := Registry["codebuddy-cn"]
	defer func() { Registry["codebuddy-cn"] = orig }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Query().Get("platform") != "CLI" {
			t.Errorf("expected platform=CLI, got %s", r.URL.Query().Get("platform"))
		}
		if r.Header.Get("X-Domain") != "copilot.tencent.com" {
			t.Errorf("expected X-Domain copilot.tencent.com, got %s", r.Header.Get("X-Domain"))
		}
		if r.Header.Get("X-No-Authorization") != "true" {
			t.Errorf("expected X-No-Authorization true")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"msg":"ok","data":{"state":"cb-state-123","authUrl":"https://copilot.tencent.com/auth?state=cb-state-123"}}`))
	}))
	defer ts.Close()

	info := orig
	info.DeviceCodeURL = ts.URL
	Registry["codebuddy-cn"] = info

	resp, err := RequestDeviceCode("codebuddy-cn", ts.Client())
	if err != nil {
		t.Fatalf("RequestDeviceCode failed: %v", err)
	}
	if resp.DeviceCode != "cb-state-123" {
		t.Errorf("expected deviceCode cb-state-123, got %q", resp.DeviceCode)
	}
	if resp.VerificationURI != "https://copilot.tencent.com/auth?state=cb-state-123" {
		t.Errorf("unexpected verificationURI: %q", resp.VerificationURI)
	}
}

func TestPollDeviceCode_CodeBuddy(t *testing.T) {
	orig := Registry["codebuddy-cn"]
	defer func() { Registry["codebuddy-cn"] = orig }()

	var step atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Query().Get("state") != "cb-state-123" {
			t.Errorf("expected state cb-state-123, got %s", r.URL.Query().Get("state"))
		}
		if r.Header.Get("X-Domain") != "copilot.tencent.com" {
			t.Errorf("expected X-Domain copilot.tencent.com")
		}
		w.Header().Set("Content-Type", "application/json")
		switch step.Load() {
		case 0:
			// Pending
			w.Write([]byte(`{"code":11217,"msg":"RetryFetchToken"}`))
		case 1:
			// Success
			w.Write([]byte(`{"code":0,"msg":"ok","data":{"accessToken":"cb-acc-token","refreshToken":"cb-ref-token","expiresIn":7200}}`))
		default:
			// Error
			w.Write([]byte(`{"code":11218,"msg":"token expired or revoked"}`))
		}
	}))
	defer ts.Close()

	info := orig
	info.TokenURL = ts.URL
	Registry["codebuddy-cn"] = info

	// Step 0: Pending
	step.Store(0)
	res, err := PollDeviceCode("codebuddy-cn", "cb-state-123", "", nil, ts.Client())
	if err != nil {
		t.Fatalf("step 0 poll failed: %v", err)
	}
	if !res.Pending {
		t.Errorf("expected pending true, got false")
	}

	// Step 1: Success
	step.Store(1)
	res, err = PollDeviceCode("codebuddy-cn", "cb-state-123", "", nil, ts.Client())
	if err != nil {
		t.Fatalf("step 1 poll failed: %v", err)
	}
	if !res.Success || res.Tokens == nil {
		t.Fatalf("expected success with tokens, got %+v", res)
	}
	if res.Tokens.AccessToken != "cb-acc-token" || res.Tokens.RefreshToken != "cb-ref-token" {
		t.Errorf("unexpected tokens: %+v", res.Tokens)
	}
	if res.Tokens.ExpiresIn != 7200 {
		t.Errorf("expected expiresIn 7200, got %d", res.Tokens.ExpiresIn)
	}

	// Step 2: Error
	step.Store(2)
	res, err = PollDeviceCode("codebuddy-cn", "cb-state-123", "", nil, ts.Client())
	if err != nil {
		t.Fatalf("step 2 poll failed: %v", err)
	}
	if res.Error != "token expired or revoked" {
		t.Errorf("expected error 'token expired or revoked', got %q", res.Error)
	}
}

func TestRequestDeviceCode_CodeBuddyIntl(t *testing.T) {
	orig := Registry["codebuddy-intl"]
	defer func() { Registry["codebuddy-intl"] = orig }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("platform") != "ide" {
			t.Errorf("expected platform=ide, got %s", r.URL.Query().Get("platform"))
		}
		if r.Header.Get("X-Domain") != "www.codebuddy.ai" {
			t.Errorf("expected X-Domain www.codebuddy.ai, got %s", r.Header.Get("X-Domain"))
		}
		if r.Header.Get("User-Agent") != "IDE/2.108.1 CodeBuddy/2.108.1" {
			t.Errorf("expected IDE User-Agent, got %s", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"msg":"ok","data":{"state":"cb-intl-state","authUrl":"https://www.codebuddy.ai/auth?state=cb-intl-state"}}`))
	}))
	defer ts.Close()

	info := orig
	info.DeviceCodeURL = ts.URL
	Registry["codebuddy-intl"] = info

	resp, err := RequestDeviceCode("codebuddy-intl", ts.Client())
	if err != nil {
		t.Fatalf("RequestDeviceCode failed: %v", err)
	}
	if resp.DeviceCode != "cb-intl-state" {
		t.Errorf("expected deviceCode cb-intl-state, got %q", resp.DeviceCode)
	}
	if resp.VerificationURI != "https://www.codebuddy.ai/auth?state=cb-intl-state" {
		t.Errorf("unexpected verificationURI: %q", resp.VerificationURI)
	}
}

func TestRequestDeviceCode_CodeBuddy_Negative(t *testing.T) {
	orig := Registry["codebuddy-cn"]
	defer func() { Registry["codebuddy-cn"] = orig }()

	// Server returning 500
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`server error`))
	}))
	defer ts.Close()

	info := orig
	info.DeviceCodeURL = ts.URL
	Registry["codebuddy-cn"] = info

	_, err := RequestDeviceCode("codebuddy-cn", ts.Client())
	if err == nil {
		t.Error("expected error on 500 status, got nil")
	}
}
func TestDeviceCodeResponse_JSONCamelCase(t *testing.T) {
	d := &DeviceCodeResponse{
		DeviceCode:              "dev-123",
		UserCode:                "usr-456",
		VerificationURI:         "https://example.com/verify",
		VerificationURIComplete: "https://example.com/verify?code=usr-456",
		ExpiresIn:               300,
		Interval:                5,
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"deviceCode":"dev-123"`) {
		t.Errorf("expected camelCase deviceCode, got %s", s)
	}
	if !strings.Contains(s, `"verificationUri":"https://example.com/verify"`) {
		t.Errorf("expected camelCase verificationUri, got %s", s)
	}
	if !strings.Contains(s, `"verificationUriComplete":"https://example.com/verify?code=usr-456"`) {
		t.Errorf("expected camelCase verificationUriComplete, got %s", s)
	}
	if !strings.Contains(s, `"expiresIn":300`) {
		t.Errorf("expected camelCase expiresIn, got %s", s)
	}
	if strings.Contains(s, `"device_code"`) || strings.Contains(s, `"verification_uri"`) {
		t.Errorf("unexpected snake_case keys in %s", s)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMapTokenResponse_MalformedIDToken(t *testing.T) {
	for _, pid := range []string{"codex", "grok-cli", "gemini", "antigravity"} {
		raw := map[string]any{
			"access_token": "at_123",
			"id_token":     "not-a-valid-jwt-without-dots",
		}
		res := mapTokenResponse(pid, raw)
		if res == nil || res.AccessToken != "at_123" {
			t.Errorf("expected at_123 for %s, got %+v", pid, res)
		}
	}
}

func TestSessionStore_ReaperAndExpiry(t *testing.T) {
	stateActive := "state_active"
	stateExpired := "state_expired"

	StoreSession(stateActive, &OAuthSession{
		CreatedAt: time.Now(),
		Provider:  "codex",
	})
	StoreSession(stateExpired, &OAuthSession{
		CreatedAt: time.Now().Add(-15 * time.Minute),
		Provider:  "codex",
	})

	if _, ok := GetSession(stateExpired); ok {
		t.Error("expected expired session to return false")
	}

	sess, ok := GetSession(stateActive)
	if !ok || sess == nil || sess.Provider != "codex" {
		t.Errorf("expected active session, got %+v", sess)
	}

	ClearSession(stateActive)
	if _, ok := GetSession(stateActive); ok {
		t.Error("expected cleared session to be gone")
	}
}
