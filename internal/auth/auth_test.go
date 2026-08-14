package auth

import (
	"fmt"
	"os"
	"sync"
	"testing"
)

func setupSecret(t *testing.T) {
	t.Helper()
	if err := InitSecretManager(t.TempDir(), ""); err != nil {
		t.Fatalf("init secret: %v", err)
	}
}

func TestGenerateAndVerifyAPIKey(t *testing.T) {
	setupSecret(t)
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
	setupSecret(t)
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
	setupSecret(t)
	token, err := CreateSessionToken()
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}
	if !VerifySessionToken(token) {
		t.Fatal("expected valid session token")
	}
}

func TestSessionTokenTampered(t *testing.T) {
	setupSecret(t)
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
	hash, err := HashPassword("mypassword")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if !IsArgon2Hash(hash) {
		t.Fatalf("expected argon2id hash, got %q", hash)
	}
	if !VerifyPassword("mypassword", hash) {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword("wrongpassword", hash) {
		t.Fatal("expected wrong password to fail")
	}
}

func TestPasswordHashUsesUniqueSalts(t *testing.T) {
	h1, _ := HashPassword("samepassword")
	h2, _ := HashPassword("samepassword")
	if h1 == h2 {
		t.Fatal("expected unique salts per hash")
	}
	if !VerifyPassword("samepassword", h1) || !VerifyPassword("samepassword", h2) {
		t.Fatal("both hashes must verify")
	}
}

func TestVerifyPasswordRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "hmac-sha256-deadbeef", "argon2id$v=19$m=0,t=0,p=0$xx$yy", "argon2id$v=19$garbage$a$b"} {
		if VerifyPassword("anything", bad) {
			t.Fatalf("expected malformed hash %q to fail", bad)
		}
	}
}

func TestLoginLimiter(t *testing.T) {
	l := NewLoginLimiter()
	ip := "192.168.1.100"

	// Should not be locked initially
	locked, _ := l.CheckLock(ip)
	if locked {
		t.Fatal("should not be locked initially")
	}

	// Record 5 failures to trigger lock
	for i := 0; i < 5; i++ {
		l.RecordFail(ip)
	}

	locked, retryAfter := l.CheckLock(ip)
	if !locked {
		t.Fatal("should be locked after 5 failures")
	}
	if retryAfter <= 0 {
		t.Fatal("expected positive retryAfter")
	}

	// Success clears the lock
	l.RecordSuccess(ip)
	locked, _ = l.CheckLock(ip)
	if locked {
		t.Fatal("should not be locked after success")
	}
}

func TestLoginLimiterEscalation(t *testing.T) {
	l := NewLoginLimiter()
	now := int64(1_000_000)
	l.nowMS = func() int64 { return now }
	ip := "10.0.0.1"

	for round, wantStep := range lockStepsMS {
		for i := 0; i < maxFailsBeforeLock; i++ {
			l.RecordFail(ip)
		}
		locked, retry := l.CheckLock(ip)
		if !locked {
			t.Fatalf("round %d: expected lock", round)
		}
		if got := int64(retry) * 1000; got > wantStep+1000 || got < wantStep-1000 {
			t.Fatalf("round %d: retryAfter=%dms, want ~%dms", round, got, wantStep)
		}
		// Advance past the lockout so the next round can accumulate.
		now += wantStep + 1
	}

	// Lockout escalates and caps at the last step.
	for i := 0; i < maxFailsBeforeLock; i++ {
		l.RecordFail(ip)
	}
	_, retry := l.CheckLock(ip)
	if got := int64(retry) * 1000; got > lockStepsMS[len(lockStepsMS)-1]+1000 {
		t.Fatalf("lockout exceeded cap: %dms", got)
	}
}

func TestLoginLimiterExpiry(t *testing.T) {
	l := NewLoginLimiter()
	now := int64(2_000_000)
	l.nowMS = func() int64 { return now }
	ip := "10.0.0.2"

	for i := 0; i < maxFailsBeforeLock-1; i++ {
		l.RecordFail(ip)
	}
	// Advance past the fail window and any (absent) lock: entry must expire.
	now += failWindowMS + 1
	if locked, _ := l.CheckLock(ip); locked {
		t.Fatal("entry should expire after the fail window")
	}
	for i := 0; i < maxFailsBeforeLock-1; i++ {
		l.RecordFail(ip)
	}
	if locked, _ := l.CheckLock(ip); locked {
		t.Fatal("counter should reset after expiry")
	}
}

func TestLoginLimiterCapacityBound(t *testing.T) {
	l := NewLoginLimiter()
	l.maxEntries = 8

	for i := 0; i < 100; i++ {
		l.RecordFail(fmt.Sprintf("10.1.0.%d", i))
	}
	l.mu.Lock()
	n := len(l.entries)
	l.mu.Unlock()
	if n > 8 {
		t.Fatalf("expected capacity-bounded entries, got %d", n)
	}
}

func TestLoginLimiterConcurrent(t *testing.T) {
	l := NewLoginLimiter()
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ip := fmt.Sprintf("10.2.0.%d", i%16)
			l.RecordFail(ip)
			l.CheckLock(ip)
			if i%7 == 0 {
				l.RecordSuccess(ip)
			}
		}(i)
	}
	wg.Wait()
}

func TestMaskSecret(t *testing.T) {
	if MaskSecret("") != "" {
		t.Fatal("expected empty mask for empty secret")
	}
	if got := MaskSecret("sk-1234567890abcd"); got != "••••abcd" {
		t.Fatalf("unexpected mask: %q", got)
	}
	if got := MaskSecret("abcd"); got != "••••" {
		t.Fatalf("unexpected short mask: %q", got)
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

func TestInitSecretManager(t *testing.T) {
	dir := t.TempDir()
	if err := InitSecretManager(dir, ""); err != nil {
		t.Fatalf("init: %v", err)
	}
	if !HasSecret() {
		t.Fatal("expected secret initialized")
	}
	key1 := GenerateAPIKey()

	// Re-init from the same dir must load the same secret (keys stay valid).
	if err := InitSecretManager(dir, ""); err != nil {
		t.Fatalf("re-init: %v", err)
	}
	if !VerifyAPIKeySignature(key1) {
		t.Fatal("secret changed across re-init from the same data dir")
	}

	// Explicit override takes precedence.
	if err := InitSecretManager(dir, "explicit-override-secret-0123456789"); err != nil {
		t.Fatalf("override init: %v", err)
	}
	if VerifyAPIKeySignature(key1) {
		t.Fatal("override secret must not validate keys from the file secret")
	}

	// Missing dataDir without override is an error.
	if err := InitSecretManager("", ""); err == nil {
		t.Fatal("expected error without dataDir")
	}
}

func TestInitSecretManagerRejectsShortFile(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(t, dir+"/auth-secret", "short"); err != nil {
		t.Fatal(err)
	}
	if err := InitSecretManager(dir, ""); err == nil {
		t.Fatal("expected error for short secret file")
	}
}

func writeFile(t *testing.T, path, content string) error {
	t.Helper()
	return os.WriteFile(path, []byte(content), 0o600)
}
