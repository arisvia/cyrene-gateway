package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestSessionTokenRequiresPassword(t *testing.T) {
	if token, err := CreateSessionToken(""); err == nil || token != "" {
		t.Fatal("session issuance without a configured password must fail")
	}
}

func TestSessionTokenRoundTrip(t *testing.T) {
	token, err := CreateSessionToken("test-password-hash")
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}
	if !VerifySessionToken(token, "test-password-hash") {
		t.Fatal("expected valid session token")
	}
}

func TestSessionTokenTampered(t *testing.T) {
	token, _ := CreateSessionToken("test-password-hash")
	tampered := token[:len(token)-2] + "xx"
	if VerifySessionToken(tampered, "test-password-hash") {
		t.Fatal("expected tampered token to fail")
	}
}

func TestSessionTokenInvalid(t *testing.T) {
	if VerifySessionToken("", "test-password-hash") {
		t.Fatal("expected empty token to fail")
	}
	if VerifySessionToken("garbage", "test-password-hash") {
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
	for _, tc := range []struct{ remote, want string }{
		{"192.168.1.1:12345", "192.168.1.1"},
		{"[::1]:8080", "::1"},
		{"[2606:4700:4700::1111]:443", "2606:4700:4700::1111"},
		{"[::ffff:203.0.113.5]:443", "::ffff:203.0.113.5"},
		{"2001:db8::1", "2001:db8::1"},
	} {
		if got := ClientIP(tc.remote); got != tc.want {
			t.Errorf("ClientIP(%q) = %q, want %q", tc.remote, got, tc.want)
		}
	}
}

func withIsolatedSecret(t *testing.T) {
	t.Helper()
	secretMu.Lock()
	savedSecret := append([]byte(nil), secret...)
	savedExplicit := isExplicit
	secret = nil
	isExplicit = false
	secretMu.Unlock()
	t.Cleanup(func() {
		secretMu.Lock()
		secret = savedSecret
		isExplicit = savedExplicit
		secretMu.Unlock()
	})
}

func TestInitSecretFile_NewSecretGenerated(t *testing.T) {
	withIsolatedSecret(t)

	dir := t.TempDir()
	if err := InitSecretFile(dir); err != nil {
		t.Fatalf("InitSecretFile failed: %v", err)
	}

	secretPath := filepath.Join(dir, "auth-secret")
	data, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatalf("failed to read created auth-secret: %v", err)
	}
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) < 32 {
		t.Fatalf("expected generated secret to be at least 32 chars, got %d", len(trimmed))
	}
	if GetSecret() != trimmed {
		t.Fatalf("in-memory secret %q does not match file secret %q", GetSecret(), trimmed)
	}
}

func TestInitSecretFile_ExistingValidSecret(t *testing.T) {
	withIsolatedSecret(t)

	dir := t.TempDir()
	secretPath := filepath.Join(dir, "auth-secret")
	preExisting := "this-is-a-valid-pre-existing-secret-key-123456"
	if err := os.WriteFile(secretPath, []byte(preExisting), 0o600); err != nil {
		t.Fatalf("failed to write test secret: %v", err)
	}

	if err := InitSecretFile(dir); err != nil {
		t.Fatalf("InitSecretFile failed: %v", err)
	}
	if GetSecret() != preExisting {
		t.Fatalf("expected in-memory secret to be %q, got %q", preExisting, GetSecret())
	}
}

func TestInitSecretFile_TooShortFileRejects(t *testing.T) {
	withIsolatedSecret(t)

	dir := t.TempDir()
	secretPath := filepath.Join(dir, "auth-secret")
	if err := os.WriteFile(secretPath, []byte("short-10b!"), 0o600); err != nil {
		t.Fatalf("failed to write short test secret: %v", err)
	}

	err := InitSecretFile(dir)
	if err == nil {
		t.Fatal("expected error for too short auth-secret file, got nil")
	}
	if !strings.Contains(err.Error(), "too short") {
		t.Fatalf("expected 'too short' error message, got: %v", err)
	}
}

func TestInitSecretFile_ExplicitShortSecretRejects(t *testing.T) {
	withIsolatedSecret(t)

	SetSecret("short-explicit")
	dir := t.TempDir()
	err := InitSecretFile(dir)
	if err == nil {
		t.Fatal("expected error for explicit short secret, got nil")
	}
	if !strings.Contains(err.Error(), "too short") {
		t.Fatalf("expected 'too short' error message, got: %v", err)
	}
}

func TestInitSecretFile_UnwriteableDirRejects(t *testing.T) {
	withIsolatedSecret(t)

	f, err := os.CreateTemp("", "cyrene-regular-file-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.Close()

	badDir := filepath.Join(f.Name(), "sub-dir")
	if err := InitSecretFile(badDir); err == nil {
		t.Fatal("expected error when data dir cannot be created under a regular file, got nil")
	}
}

func TestPersistSecret_RoundTrip(t *testing.T) {
	withIsolatedSecret(t)

	dir := t.TempDir()
	newSecret := "persisted-secret-value-that-exceeds-thirty-two-bytes-by-far"
	if err := PersistSecret(dir, newSecret); err != nil {
		t.Fatalf("PersistSecret failed: %v", err)
	}

	if GetSecret() != newSecret {
		t.Fatalf("expected in-memory secret to update to %q, got %q", newSecret, GetSecret())
	}
	fileData, err := os.ReadFile(filepath.Join(dir, "auth-secret"))
	if err != nil {
		t.Fatalf("failed to read persisted secret file: %v", err)
	}
	if strings.TrimSpace(string(fileData)) != newSecret {
		t.Fatalf("file content does not match persisted secret: %q", string(fileData))
	}
}
