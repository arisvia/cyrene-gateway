package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

// Password hashing uses Argon2id with a per-password salt (P0-3). The session
// signing secret is never used for password derivation. Hash format:
//
//	argon2id$v=19$m=<mem>,t=<time>,p=<threads>$<salt-b64>$<hash-b64>
//
// There is no default password and no legacy HMAC fallback: the project is
// pre-production, so only the Argon2id scheme is supported.

const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16

	// MinPasswordLength is enforced for dashboard passwords.
	MinPasswordLength = 8
)

// HashPassword creates an Argon2id hash with a fresh random salt.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate salt: %w", err)
	}
	sum := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum),
	), nil
}

// VerifyPassword checks a password against an Argon2id hash in constant time.
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" || parts[1] != "v=19" {
		return false
	}
	params := strings.Split(parts[2], ",")
	if len(params) != 3 {
		return false
	}
	if !strings.HasPrefix(params[0], "m=") || !strings.HasPrefix(params[1], "t=") || !strings.HasPrefix(params[2], "p=") {
		return false
	}
	mem64, err := strconv.ParseUint(strings.TrimPrefix(params[0], "m="), 10, 32)
	if err != nil || mem64 == 0 {
		return false
	}
	iterations64, err := strconv.ParseUint(strings.TrimPrefix(params[1], "t="), 10, 32)
	if err != nil || iterations64 == 0 {
		return false
	}
	threads64, err := strconv.ParseUint(strings.TrimPrefix(params[2], "p="), 10, 8)
	if err != nil || threads64 == 0 {
		return false
	}
	mem := uint32(mem64)
	iterations := uint32(iterations64)
	threads := uint8(threads64)
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, mem, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// IsArgon2Hash reports whether a stored hash uses the Argon2id scheme.
func IsArgon2Hash(hash string) bool {
	return strings.HasPrefix(hash, "argon2id$")
}

// --- Login Rate Limiter ---
//
// The limiter is a mutex-protected, capacity-bounded instance (P0-2). It is
// owned by the Server so tests and multi-instance deployments do not share
// mutable global state.

const (
	maxFailsBeforeLock = 5
	failWindowMS       = 60 * 60 * 1000 // 1h
	defaultMaxEntries  = 10_000
)

var lockStepsMS = []int64{30_000, 120_000, 600_000, 1_800_000} // 30s, 2m, 10m, 30m

type lockEntry struct {
	fails      int
	lockUntil  int64
	lockLevel  int
	lastFailAt int64
}

// LoginLimiter tracks failed dashboard logins per client address.
type LoginLimiter struct {
	mu         sync.Mutex
	entries    map[string]*lockEntry
	maxEntries int
	nowMS      func() int64 // injectable clock for tests
}

// NewLoginLimiter creates a bounded, concurrency-safe login limiter.
func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{
		entries:    make(map[string]*lockEntry),
		maxEntries: defaultMaxEntries,
		nowMS:      func() int64 { return time.Now().UnixMilli() },
	}
}

// getLocked returns the live entry for ip, dropping it when fully expired.
// Caller must hold the mutex.
func (l *LoginLimiter) getLocked(ip string) *lockEntry {
	e, ok := l.entries[ip]
	if !ok {
		return nil
	}
	now := l.nowMS()
	if e.lastFailAt > 0 && now-e.lastFailAt > failWindowMS && (e.lockUntil == 0 || now >= e.lockUntil) {
		delete(l.entries, ip)
		return nil
	}
	return e
}

// evictLocked frees capacity by removing expired entries first, then the
// oldest entries by lastFailAt. Caller must hold the mutex.
func (l *LoginLimiter) evictLocked() {
	if len(l.entries) < l.maxEntries {
		return
	}
	now := l.nowMS()
	for ip, e := range l.entries {
		if e.lastFailAt > 0 && now-e.lastFailAt > failWindowMS && (e.lockUntil == 0 || now >= e.lockUntil) {
			delete(l.entries, ip)
		}
	}
	for len(l.entries) >= l.maxEntries {
		var oldestIP string
		oldestAt := int64(math.MaxInt64)
		for ip, e := range l.entries {
			if e.lastFailAt < oldestAt {
				oldestAt = e.lastFailAt
				oldestIP = ip
			}
		}
		delete(l.entries, oldestIP)
	}
}

// CheckLock returns whether the address is locked out and seconds remaining.
func (l *LoginLimiter) CheckLock(ip string) (locked bool, retryAfterSec int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.getLocked(ip)
	if e == nil || e.lockUntil == 0 {
		return false, 0
	}
	remaining := e.lockUntil - l.nowMS()
	if remaining <= 0 {
		return false, 0
	}
	return true, int((remaining + 999) / 1000)
}

// RecordFail records a failed login attempt, escalating lockouts on repeats.
func (l *LoginLimiter) RecordFail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.getLocked(ip)
	if e == nil {
		l.evictLocked()
		e = &lockEntry{}
		l.entries[ip] = e
	}
	now := l.nowMS()
	e.fails++
	e.lastFailAt = now
	if e.fails >= maxFailsBeforeLock {
		step := lockStepsMS[min(e.lockLevel, len(lockStepsMS)-1)]
		e.lockUntil = now + step
		e.lockLevel++
		e.fails = 0
	}
}

// RecordSuccess clears login attempts for an address.
func (l *LoginLimiter) RecordSuccess(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, ip)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- Misc helpers ---

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

// MaskSecret returns a non-reversible hint for a secret: masked prefix plus
// the last 4 characters. Empty secrets return "".
func MaskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "••••"
	}
	return "••••" + s[len(s)-4:]
}
