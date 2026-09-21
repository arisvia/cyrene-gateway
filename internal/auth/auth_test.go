package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateAndVerifyAPIKey(t *testing.T) {
	key := GenerateAPIKey()
	if key == "" {
		t.Fatal("expected non-empty key")
	}
	if len(key) < 10 {
		t.Fatalf("key too short: %s", key)
	}
	if key[:3] != "cg-" {
		t.Fatalf("expected cg- prefix, got %s", key[:3])
	}
	if !VerifyAPIKeySignature(key) {
		t.Fatal("expected valid signature")
	}
}

func TestGenerateRandomPassword(t *testing.T) {
	pw1 := GenerateRandomPassword()
	pw2 := GenerateRandomPassword()
	if pw1 == "" || pw2 == "" {
		t.Fatal("expected non-empty random password")
	}
	if pw1 == pw2 {
		t.Fatal("expected random passwords to be unique")
	}
	if len(pw1) < 12 {
		t.Fatalf("expected password length >= 12, got %d (%s)", len(pw1), pw1)
	}
}

func TestVerifyAPIKeyTampered(t *testing.T) {
	key := GenerateAPIKey()
	// Tamper with the key
	tampered := key[:len(key)-2] + "xx"
	if VerifyAPIKeySignature(tampered) {
		t.Fatal("expected tampered key to fail verification")
	}
}

func TestVerifyAPIKeyInvalidFormat(t *testing.T) {
	if VerifyAPIKeySignature("sk-invalid") {
		t.Fatal("expected non-cg key to fail")
	}
	if VerifyAPIKeySignature("cg-nodotsignature") {
		t.Fatal("expected key without dot to fail")
	}
	if VerifyAPIKeySignature("") {
		t.Fatal("expected empty key to fail")
	}
}

func TestSessionTokenRoundTrip(t *testing.T) {
	token, err := CreateSessionToken()
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}
	if !VerifySessionToken(token) {
		t.Fatal("expected valid session token")
	}
}

func TestSessionTokenTampered(t *testing.T) {
	token, _ := CreateSessionToken()
	tampered := token[:len(token)-2] + "xx"
	if VerifySessionToken(tampered) {
		t.Fatal("expected tampered token to fail")
	}
}

func TestSessionTokenInvalid(t *testing.T) {
	if VerifySessionToken("") {
		t.Fatal("expected empty token to fail")
	}
	if VerifySessionToken("garbage") {
		t.Fatal("expected garbage token to fail")
	}
}

func TestPasswordHashAndVerify(t *testing.T) {
	hash := HashPassword("mypassword")
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if !VerifyPassword("mypassword", hash) {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword("wrongpassword", hash) {
		t.Fatal("expected wrong password to fail")
	}
}

func TestLoginRateLimiter(t *testing.T) {
	ip := "192.168.1.100"

	// Should not be locked initially
	locked, _ := CheckLock(ip)
	if locked {
		t.Fatal("should not be locked initially")
	}

	// Record 5 failures to trigger lock
	for range 5 {
		RecordFail(ip)
	}

	locked, retryAfter := CheckLock(ip)
	if !locked {
		t.Fatal("should be locked after 5 failures")
	}
	if retryAfter <= 0 {
		t.Fatal("expected positive retryAfter")
	}

	// Success clears the lock
	RecordSuccess(ip)
	locked, _ = CheckLock(ip)
	if locked {
		t.Fatal("should not be locked after success")
	}
}

func TestExtractAPIKey(t *testing.T) {
	req := func(target string, headers map[string]string) *http.Request {
		r := httptest.NewRequest("POST", target, nil)
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		return r
	}

	cases := []struct {
		name string
		req  *http.Request
		want string
	}{
		{"bearer token", req("/v1/chat/completions", map[string]string{"Authorization": "Bearer sk-test123"}), "sk-test123"},
		{"bare authorization", req("/v1/chat/completions", map[string]string{"Authorization": "sk-raw"}), "sk-raw"},
		{"anthropic x-api-key", req("/v1/messages", map[string]string{"x-api-key": "sk-ant"}), "sk-ant"},
		{"gemini x-goog-api-key", req("/v1beta/models/gemini:generateContent", map[string]string{"x-goog-api-key": "AIza-sdk"}), "AIza-sdk"},
		{"gemini rest ?key=", req("/v1beta/models/gemini:generateContent?key=AIza-query", nil), "AIza-query"},
		{"bearer wins over x-api-key", req("/v1/messages", map[string]string{"Authorization": "Bearer sk-bearer", "x-api-key": "sk-ant"}), "sk-bearer"},
		{"x-api-key wins over goog", req("/v1/messages", map[string]string{"x-api-key": "sk-ant", "x-goog-api-key": "AIza-sdk"}), "sk-ant"},
		{"x-goog-api-key wins over ?key=", req("/v1beta/models/gemini?key=AIza-query", map[string]string{"x-goog-api-key": "AIza-sdk"}), "AIza-sdk"},
		{"empty", req("/v1/chat/completions", nil), ""},
		{"nil request", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractAPIKey(tc.req); got != tc.want {
				t.Fatalf("ExtractAPIKey(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	if ip := ClientIP("192.168.1.1:12345"); ip != "192.168.1.1" {
		t.Fatalf("expected 192.168.1.1, got %s", ip)
	}
	if ip := ClientIP("[::1]:8080"); ip != "[::1]" {
		t.Fatalf("expected [::1], got %s", ip)
	}
}
