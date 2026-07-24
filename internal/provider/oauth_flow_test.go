package provider

import (
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
		{"qwen", FlowDeviceCode},
		{"kimi", FlowDeviceCode},
		{"grok-cli", FlowDeviceCode},
		{"claude", FlowAuthorizationCodePKCE},
		{"codex", FlowAuthorizationCodePKCE},
		{"gemini-cli", FlowAuthorizationCodePKCE},
		{"cline", FlowAuthorizationCode},
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
