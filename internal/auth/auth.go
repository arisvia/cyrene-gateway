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
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

// Secret management: load from env, file, or generate and persist.

var (
	secretMu sync.RWMutex
	secret   []byte
)

// init keeps env-only secret resolution working even before explicit Init
// (some tests rely on CYRENE_AUTH_SECRET). File persistence moves to
// InitSecretFile so -data-dir can control where the secret lives.
func init() {
	secret = []byte(os.Getenv("CYRENE_AUTH_SECRET"))
}

// InitSecretFile loads the HMAC secret from <dir>/auth-secret, generating and
// persisting one when absent. Must be called after flag parsing with the
// resolved data directory. A secret already set via env or SetSecret wins.
func InitSecretFile(dir string) {
	secretMu.Lock()
	defer secretMu.Unlock()
	if len(secret) >= 32 {
		return // env or explicit SetSecret already provided a secret
	}
	path := filepath.Join(dir, "auth-secret")
	if data, err := os.ReadFile(path); err == nil && len(data) >= 32 {
		secret = []byte(strings.TrimSpace(string(data)))
		return
	}
	b := make([]byte, 32)
	rand.Read(b)
	generated := hex.EncodeToString(b)
	os.MkdirAll(dir, 0o700)
	os.WriteFile(path, []byte(generated), 0o600)
	secret = []byte(generated)
}

// loadSecret is retained for backward compatibility of tests only.
func loadSecret() []byte {
	if env := os.Getenv("CYRENE_AUTH_SECRET"); env != "" {
		return []byte(env)
	}
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".cyrene-gateway", "auth-secret")
	if data, err := os.ReadFile(path); err == nil && len(data) >= 32 {
		return []byte(strings.TrimSpace(string(data)))
	}
	return nil
}

// SetSecret overrides the auth secret (used when -secret flag is provided).
func SetSecret(s string) {
	if s != "" {
		secretMu.Lock()
		secret = []byte(s)
		secretMu.Unlock()
	}
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
	rand.Read(b)
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

// ExtractAPIKey extracts the API key from Authorization header or x-api-key header.
func ExtractAPIKey(authHeader, xApiKeyHeader string) string {
	if authHeader != "" {
		if after, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
			return after
		}
		return authHeader
	}
	return xApiKeyHeader
}

// ClientIP extracts the IP address from a host:port string.
func ClientIP(remoteAddr string) string {
	if strings.HasPrefix(remoteAddr, "[") && strings.Contains(remoteAddr, "]:") {
		idx := strings.LastIndex(remoteAddr, "]:")
		return remoteAddr[:idx+1]
	}
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

// CreateSessionToken creates an HMAC-signed session token for dashboard auth.
func CreateSessionToken() (string, error) {
	claims := sessionClaims{
		Authenticated: true,
		ExpiresAt:     time.Now().Add(24 * time.Hour).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	sig := sign([]byte(encoded))
	return encoded + "." + sig, nil
}

// VerifySessionToken validates a session token and checks expiry.
func VerifySessionToken(token string) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	if !verify([]byte(parts[0]), parts[1]) {
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

// --- Password Hashing (Argon2id + legacy HMAC fallback) ---

const (
	argonMemory      = 64 * 1024 // 64 MB
	argonIterations  = 3
	argonParallelism = 2
	argonSaltLen     = 16
	argonKeyLen      = 32
)

// HashPassword creates a secure Argon2id hash of the password.
func HashPassword(password string) string {
	salt := make([]byte, argonSaltLen)
	rand.Read(salt)
	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash))
}

// VerifyPassword checks a password against a stored hash (supports Argon2id and legacy HMAC migration).
func VerifyPassword(password, storedHash string) bool {
	if strings.HasPrefix(storedHash, "$argon2id$") {
		return verifyArgon2id(password, storedHash)
	}
	// Fallback to legacy HMAC hash verification
	mac := hmac.New(sha256.New, getSecret())
	mac.Write([]byte("password:" + password))
	legacyHash := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(legacyHash), []byte(storedHash))
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
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
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
