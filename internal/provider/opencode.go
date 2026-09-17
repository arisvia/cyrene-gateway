package provider

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultOpenCodeUA = "opencode/1.18.31"
	base62Chars       = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

var (
	openCodeSessionRegex = regexp.MustCompile(`^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`)
	openCodeVersionRegex = regexp.MustCompile(`(?i)opencode/(\d+)\.(\d+)`)
)

func init() {
	authHooks["opencodeHeaders"] = opencodeHeadersHook
}

func hasValidOpenCodeVersion(ua string) bool {
	m := openCodeVersionRegex.FindStringSubmatch(ua)
	if len(m) < 3 {
		return false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	return major > 1 || (major == 1 && minor >= 17)
}

// GenerateCanonicalSessionID generates a canonical descending OpenCode session ID.
// Format: ses_ + 12 hex timestamp digits + 14 Base62 chars (30 chars total).
func GenerateCanonicalSessionID() string {
	now := time.Now().UnixMilli()
	current := uint64(now)*0x1000 + 1
	inv := ^current
	timeBytes := make([]byte, 6)
	for i := range 6 {
		timeBytes[i] = byte(inv >> (40 - 8*i))
	}
	timeHex := hex.EncodeToString(timeBytes)
	randBytes := make([]byte, 14)
	_, _ = rand.Read(randBytes)
	var randPart strings.Builder
	for i := range 14 {
		randPart.WriteByte(base62Chars[randBytes[i]%62])
	}
	return "ses_" + timeHex + randPart.String()
}

// GenerateCanonicalRequestID generates a canonical OpenCode request ID.
// Format: msg_ + 12 hex timestamp digits + 14 Base62 chars (30 chars total).
func GenerateCanonicalRequestID() string {
	now := time.Now().UnixMilli()
	current := uint64(now)*0x1000 + 1
	timeBytes := make([]byte, 6)
	for i := range 6 {
		timeBytes[i] = byte(current >> (40 - 8*i))
	}
	timeHex := hex.EncodeToString(timeBytes)
	randBytes := make([]byte, 14)
	_, _ = rand.Read(randBytes)
	var randPart strings.Builder
	randPart.Grow(14)
	for i := range 14 {
		randPart.WriteByte(base62Chars[randBytes[i]%62])
	}
	return "msg_" + timeHex + randPart.String()
}

// TranslateSessionID maps an arbitrary session ID into OpenCode canonical format.
func TranslateSessionID(sessionID string) string {
	trimmed := strings.TrimSpace(sessionID)
	if openCodeSessionRegex.MatchString(trimmed) {
		return trimmed
	}
	h := sha256.Sum256([]byte("opencode\x00session\x00" + trimmed))
	timeHex := hex.EncodeToString(h[:6])
	var randPart strings.Builder
	randPart.Grow(14)
	for i := 6; i < 20; i++ {
		randPart.WriteByte(base62Chars[h[i]%62])
	}
	return "ses_" + timeHex + randPart.String()
}

// IsOpenCodeFreeModel checks whether a model qualifies for the unauthenticated free tier.
func IsOpenCodeFreeModel(modelName string) bool {
	lower := strings.ToLower(strings.TrimSpace(modelName))
	if idx := strings.LastIndex(lower, "/"); idx >= 0 {
		lower = lower[idx+1:]
	}
	return strings.HasSuffix(lower, "-free") ||
		lower == "big-pickle" ||
		lower == "union-alpha" ||
		strings.Contains(lower, "-contributor-free")
}

// opencodeHeadersHook injects the client tracking and fingerprint headers required by OpenCode.
// Upstream enforces versioned User-Agent (>= 1.17.0) and canonical ses_ / msg_ format on free tiers.
func opencodeHeadersHook(h http.Header, c Credentials) {
	if !hasValidOpenCodeVersion(h.Get("User-Agent")) {
		h.Set("User-Agent", DefaultOpenCodeUA)
	}
	if h.Get("x-opencode-client") == "" {
		h.Set("x-opencode-client", "desktop")
	}

	sessionID := strings.TrimSpace(h.Get("x-opencode-session"))
	if sessionID != "" {
		h.Set("x-opencode-session", TranslateSessionID(sessionID))
	} else {
		if c.ProviderSpecificData != nil {
			if s, ok := c.ProviderSpecificData["sessionId"].(string); ok && s != "" {
				sessionID = TranslateSessionID(s)
			}
		}
		if sessionID == "" {
			if token := c.Token(); token != "" && token != "public" {
				sessionID = TranslateSessionID(token)
			} else {
				sessionID = GenerateCanonicalSessionID()
			}
		}
		h.Set("x-opencode-session", sessionID)
	}

	if h.Get("x-opencode-request") == "" {
		h.Set("x-opencode-request", GenerateCanonicalRequestID())
	}
	if h.Get("x-opencode-project") == "" {
		h.Set("x-opencode-project", "global")
	}
}
