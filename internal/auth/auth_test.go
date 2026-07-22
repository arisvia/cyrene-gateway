package auth

import (
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
	for i := 0; i < 5; i++ {
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
	// Bearer token
	key := ExtractAPIKey("Bearer sk-test123", "")
	if key != "sk-test123" {
		t.Fatalf("expected sk-test123, got %s", key)
	}

	// x-api-key header
	key = ExtractAPIKey("", "sk-header")
	if key != "sk-header" {
		t.Fatalf("expected sk-header, got %s", key)
	}

	// Bearer takes priority
	key = ExtractAPIKey("Bearer sk-bearer", "sk-header")
	if key != "sk-bearer" {
		t.Fatalf("expected sk-bearer, got %s", key)
	}

	// Empty
	key = ExtractAPIKey("", "")
	if key != "" {
		t.Fatalf("expected empty, got %s", key)
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
