package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Secret management: load from env, file, or generate and persist.

var secret []byte

func init() {
	secret = loadSecret()
}

func loadSecret() []byte {
	if env := os.Getenv("CYRENE_AUTH_SECRET"); env != "" {
		return []byte(env)
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".cyrene-gateway")
	path := filepath.Join(dir, "auth-secret")
	if data, err := os.ReadFile(path); err == nil && len(data) >= 32 {
		return []byte(strings.TrimSpace(string(data)))
	}
	b := make([]byte, 32)
	rand.Read(b)
	generated := hex.EncodeToString(b)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(path, []byte(generated), 0o600)
	return []byte(generated)
}

// SetSecret overrides the auth secret (used when -secret flag is provided).
func SetSecret(s string) {
	if s != "" {
		secret = []byte(s)
	}
}

func sign(data []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func verify(data []byte, signature string) bool {
	mac := hmac.New(sha256.New, secret)
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

// --- Password Hashing (HMAC-based, no bcrypt dependency) ---

// HashPassword creates an HMAC-SHA256 hash of the password.
func HashPassword(password string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("password:" + password))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyPassword checks a password against a stored hash.
func VerifyPassword(password, hash string) bool {
	return hmac.Equal([]byte(HashPassword(password)), []byte(hash))
}

// --- Login Rate Limiter ---

const (
	maxFailsBeforeLock = 5
	failWindowMS       = 60 * 60 * 1000 // 1h
)

var lockStepsMS = []int64{30_000, 120_000, 600_000, 1_800_000} // 30s, 2m, 10m, 30m

type lockEntry struct {
	fails      int
	lockUntil  int64
	lockLevel  int
	lastFailAt int64
}

var attempts = make(map[string]*lockEntry)

func nowMS() int64 { return time.Now().UnixMilli() }

func getEntry(ip string) *lockEntry {
	e, ok := attempts[ip]
	if !ok {
		return nil
	}
	if e.lastFailAt > 0 && nowMS()-e.lastFailAt > failWindowMS && (e.lockUntil == 0 || nowMS() >= e.lockUntil) {
		delete(attempts, ip)
		return nil
	}
	return e
}

// CheckLock returns whether the IP is locked out and seconds remaining.
func CheckLock(ip string) (locked bool, retryAfterSec int) {
	e := getEntry(ip)
	if e == nil || e.lockUntil == 0 {
		return false, 0
	}
	remaining := e.lockUntil - nowMS()
	if remaining <= 0 {
		return false, 0
	}
	return true, int((remaining + 999) / 1000)
}

// RecordFail records a failed login attempt.
func RecordFail(ip string) {
	e := getEntry(ip)
	if e == nil {
		e = &lockEntry{}
	}
	e.fails++
	e.lastFailAt = nowMS()
	if e.fails >= maxFailsBeforeLock {
		step := lockStepsMS[min(e.lockLevel, len(lockStepsMS)-1)]
		e.lockUntil = nowMS() + step
		e.lockLevel++
		e.fails = 0
	}
	attempts[ip] = e
}

// RecordSuccess clears login attempts for an IP.
func RecordSuccess(ip string) {
	delete(attempts, ip)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ExtractAPIKey extracts the API key from request headers.
func ExtractAPIKey(authHeader, xAPIKey string) string {
	if strings.HasPrefix(authHeader, "Bearer ") {
		return authHeader[7:]
	}
	if xAPIKey != "" {
		return xAPIKey
	}
	return ""
}

// ClientIP extracts client IP from remote address.
func ClientIP(remoteAddr string) string {
	if idx := strings.LastIndex(remoteAddr, ":"); idx != -1 {
		return remoteAddr[:idx]
	}
	return remoteAddr
}

// FormatRetryAfter formats milliseconds into human-readable duration.
func FormatRetryAfter(ms int64) string {
	sec := ms / 1000
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	if sec < 3600 {
		return fmt.Sprintf("%dm%ds", sec/60, sec%60)
	}
	return fmt.Sprintf("%dh%dm", sec/3600, (sec%3600)/60)
}
