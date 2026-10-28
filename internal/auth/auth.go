package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

// Secret management: load from env, file, or generate and persist.

var (
	secretMu   sync.RWMutex
	secret     []byte
	isExplicit bool
)

// init keeps env-only secret resolution working even before explicit Init
// (some tests rely on CYRENE_AUTH_SECRET). File persistence moves to
// InitSecretFile so -data-dir can control where the secret lives.
func init() {
	if env := os.Getenv("CYRENE_AUTH_SECRET"); env != "" {
		secret = []byte(env)
		isExplicit = true
	}
}

// InitSecretFile loads the HMAC secret from <dir>/auth-secret, generating and
// persisting one when absent. Must be called after flag parsing with the
// resolved data directory. A secret already set via env or SetSecret wins.
func InitSecretFile(dir string) error {
	secretMu.Lock()
	defer secretMu.Unlock()
	if isExplicit {
		if len(secret) < 32 {
			return fmt.Errorf("auth-secret is too short: must be at least 32 bytes")
		}
		return nil
	}
	if len(secret) >= 32 {
		return nil // secret already provided
	}
	path := filepath.Join(dir, "auth-secret")
	if data, err := os.ReadFile(path); err == nil {
		trimmed := strings.TrimSpace(string(data))
		if len(trimmed) < 32 {
			return fmt.Errorf("auth-secret is too short: must be at least 32 bytes")
		}
		secret = []byte(trimmed)
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read auth-secret: %w", err)
	}
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return fmt.Errorf("generate auth-secret: %w", err)
	}
	generated := hex.EncodeToString(b)
	if err := writeSecretFile(dir, generated); err != nil {
		return err
	}
	secret = []byte(generated)
	return nil
}

func writeSecretFile(dir, content string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create auth-secret directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".auth-secret-*")
	if err != nil {
		return fmt.Errorf("create temporary auth-secret: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temporary auth-secret: %w", err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write temporary auth-secret: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temporary auth-secret: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary auth-secret: %w", err)
	}
	path := filepath.Join(dir, "auth-secret")
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("install auth-secret: %w", err)
	}
	return nil
}

// SetSecret overrides the auth secret (used when -secret flag is provided).
func SetSecret(s string) {
	if s != "" {
		secretMu.Lock()
		secret = []byte(s)
		isExplicit = true
		secretMu.Unlock()
	}
}

// GetSecret returns the current in-memory HMAC auth secret as string.
func GetSecret() string {
	return string(getSecret())
}

// IsExplicitSecret reports whether the secret was set via CLI flag or env var.
func IsExplicitSecret() bool {
	secretMu.RLock()
	defer secretMu.RUnlock()
	return isExplicit
}

// PersistSecret writes the secret to <dir>/auth-secret first, then updates memory.
// It safely no-ops if the secret was explicitly configured via CLI flag or ENV.
func PersistSecret(dir, secretStr string) error {
	trimmed := strings.TrimSpace(secretStr)
	if len(trimmed) < 32 {
		return fmt.Errorf("secret too short: must be at least 32 bytes")
	}
	secretMu.Lock()
	defer secretMu.Unlock()
	if isExplicit {
		return nil
	}
	if dir != "" {
		if err := writeSecretFile(dir, trimmed); err != nil {
			return err
		}
	}
	secret = []byte(trimmed)
	return nil
}

func getSecret() []byte {
	secretMu.RLock()
	defer secretMu.RUnlock()
	return secret
}

func sign(data []byte) string {
	mac := hmac.New(sha256.New, getSecret())
	mac.Write(data)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func verify(data []byte, signature string) bool {
	mac := hmac.New(sha256.New, getSecret())
	mac.Write(data)
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// --- API Key Generation (HMAC-signed) ---

// GenerateAPIKey creates an HMAC-signed API key: cg-<random>.<signature>
func GenerateAPIKey() string {
	b := make([]byte, 24)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		panic(fmt.Errorf("generate API key: %w", err))
	}
	payload := hex.EncodeToString(b)
	sig := sign([]byte(payload))
	return "cg-" + payload + "." + sig
}

// VerifyAPIKeySignature checks the HMAC integrity of an API key.
func VerifyAPIKeySignature(key string) bool {
	if !strings.HasPrefix(key, "cg-") {
		return false
	}
	rest := key[3:]
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) != 2 {
		return false
	}
	return verify([]byte(parts[0]), parts[1])
}

// ExtractAPIKey pulls the caller credential from the request, supporting every
// inbound protocol surface in one shared site (AGENTS.md §7):
//   - OpenAI / generic: "Authorization: Bearer <key>" (or a bare Authorization value)
//   - Anthropic Messages: "x-api-key: <key>"
//   - Google Gemini SDK: "x-goog-api-key: <key>"
//   - Google Gemini REST: "?key=<key>" query parameter
//
// Precedence follows the order above; the first non-empty source wins.
func ExtractAPIKey(r *http.Request) string {
	if r == nil {
		return ""
	}
	if h := strings.TrimSpace(r.Header.Get("Authorization")); h != "" {
		if len(h) >= 7 && strings.EqualFold(h[:7], "bearer ") {
			return strings.TrimSpace(h[7:])
		}
		return h
	}
	if k := strings.TrimSpace(r.Header.Get("x-api-key")); k != "" {
		return k
	}
	if k := strings.TrimSpace(r.Header.Get("x-goog-api-key")); k != "" {
		return k
	}
	return strings.TrimSpace(r.URL.Query().Get("key"))
}

// ClientIP extracts the IP address from a host:port string.
func ClientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// --- Dashboard Session Token ---

type sessionClaims struct {
	Authenticated bool  `json:"auth"`
	ExpiresAt     int64 `json:"exp"`
}

// CreateSessionToken binds dashboard sessions to the current password hash.
// Password changes revoke sessions without rotating the API key signing secret.
func CreateSessionToken(passwordHash string) (string, error) {
	if passwordHash == "" {
		return "", fmt.Errorf("password not initialized")
	}
	claims := sessionClaims{
		Authenticated: true,
		ExpiresAt:     time.Now().Add(24 * time.Hour).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	sig := sign([]byte("session:" + passwordHash + "\x00" + encoded))
	return encoded + "." + sig, nil
}

// VerifySessionToken validates a session against the current password and expiry.
// Legacy unbound sessions are intentionally rejected.
func VerifySessionToken(token, passwordHash string) bool {
	if passwordHash == "" {
		return false
	}
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	if !verify([]byte("session:"+passwordHash+"\x00"+parts[0]), parts[1]) {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	var claims sessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return false
	}
	return claims.Authenticated && time.Now().Unix() < claims.ExpiresAt
}

// --- Password Hashing (Argon2id) ---

const (
	argonMemory      = 64 * 1024 // 64 MB
	argonIterations  = 3
	argonParallelism = 2
	argonSaltLen     = 16
	argonKeyLen      = 32
)

// GenerateRandomPassword creates a cryptographically secure random password for initial setup.
// Format: cg-admin-<16 random hex characters>
func GenerateRandomPassword() string {
	b := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		panic(fmt.Errorf("generate initial admin password: %w", err))
	}
	return "cg-admin-" + hex.EncodeToString(b)
}

// HashPassword creates a secure Argon2id hash of the password.
func HashPassword(password string) string {
	salt := make([]byte, argonSaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		panic(fmt.Errorf("generate password salt: %w", err))
	}
	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash))
}

// VerifyPassword checks a password against a stored Argon2id hash. Legacy
// HMAC-shaped hashes are no longer accepted (clean cutover; stored hashes are
// re-hashed on password change).
func VerifyPassword(password, storedHash string) bool {
	if !strings.HasPrefix(storedHash, "$argon2id$") {
		return false
	}
	return verifyArgon2id(password, storedHash)
}

func verifyArgon2id(password, storedHash string) bool {
	parts := strings.Split(storedHash, "$")
	if len(parts) < 6 {
		return false
	}
	var version int
	var memory uint32
	var iterations uint32
	var parallelism uint8
	_, err := fmt.Sscanf(parts[2], "v=%d", &version)
	if err != nil {
		return false
	}
	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism)
	if err != nil {
		return false
	}
	// Never allocate directly from untrusted/restored hash parameters.
	if version != argon2.Version || memory < 8*1024 || memory > 256*1024 || iterations < 1 || iterations > 10 || parallelism < 1 || parallelism > 16 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(salt) < 16 || len(salt) > 64 || len(expectedHash) < 16 || len(expectedHash) > 64 {
		return false
	}
	calculatedHash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(expectedHash, calculatedHash) == 1
}

// --- Concurrent-Safe Login Rate Limiter ---

const (
	maxFailsBeforeLock = 5
	failWindowMS       = 60 * 60 * 1000 // 1h
	maxLimiterEntries  = 10000          // Memory upper-bound
)

var lockStepsMS = []int64{30_000, 120_000, 600_000, 1_800_000} // 30s, 2m, 10m, 30m

type lockEntry struct {
	fails      int
	lockUntil  int64
	lockLevel  int
	lastFailAt int64
}

type LoginLimiter struct {
	mu       sync.Mutex
	attempts map[string]*lockEntry
}

var globalLimiter = &LoginLimiter{
	attempts: make(map[string]*lockEntry),
}

func nowMS() int64 { return time.Now().UnixMilli() }

func (l *LoginLimiter) cleanupLocked() {
	now := nowMS()
	for ip, e := range l.attempts {
		if e.lastFailAt > 0 && now-e.lastFailAt > failWindowMS && (e.lockUntil == 0 || now >= e.lockUntil) {
			delete(l.attempts, ip)
		}
	}
	// If still too large, prune arbitrarily
	if len(l.attempts) > maxLimiterEntries {
		for ip := range l.attempts {
			delete(l.attempts, ip)
			if len(l.attempts) <= maxLimiterEntries/2 {
				break
			}
		}
	}
}

// CheckLock returns whether the IP is locked out and seconds remaining.
func CheckLock(ip string) (locked bool, retryAfterSec int) {
	globalLimiter.mu.Lock()
	defer globalLimiter.mu.Unlock()

	e, ok := globalLimiter.attempts[ip]
	if !ok {
		return false, 0
	}
	now := nowMS()
	if e.lastFailAt > 0 && now-e.lastFailAt > failWindowMS && (e.lockUntil == 0 || now >= e.lockUntil) {
		delete(globalLimiter.attempts, ip)
		return false, 0
	}
	if e.lockUntil == 0 {
		return false, 0
	}
	remaining := e.lockUntil - now
	if remaining <= 0 {
		return false, 0
	}
	return true, int((remaining + 999) / 1000)
}

// RecordFail records a failed login attempt safely.
func RecordFail(ip string) {
	globalLimiter.mu.Lock()
	defer globalLimiter.mu.Unlock()

	if len(globalLimiter.attempts) >= maxLimiterEntries {
		globalLimiter.cleanupLocked()
	}

	e, ok := globalLimiter.attempts[ip]
	if !ok {
		e = &lockEntry{}
		globalLimiter.attempts[ip] = e
	}
	e.fails++
	e.lastFailAt = nowMS()
	if e.fails >= maxFailsBeforeLock {
		step := lockStepsMS[min(e.lockLevel, len(lockStepsMS)-1)]
		e.lockUntil = nowMS() + step
		e.lockLevel++
		e.fails = 0
	}
}

// RecordSuccess clears login attempts for an IP safely.
func RecordSuccess(ip string) {
	globalLimiter.mu.Lock()
	defer globalLimiter.mu.Unlock()
	delete(globalLimiter.attempts, ip)
}
