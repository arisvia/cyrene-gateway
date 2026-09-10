package provider

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
)

func init() {
	authHooks["opencodeHeaders"] = opencodeHeadersHook
}

func deriveOpencodeSessionID(seed string) string {
	if seed == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		return fmt.Sprintf("ses_%x", b)
	}
	h := sha256.Sum256([]byte("opencode:session:" + seed))
	return fmt.Sprintf("ses_%x", h[:16])
}

func generateOpencodeRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("msg_%x", b)
}

// opencodeHeadersHook injects the client tracking and routing headers required by OpenCode.
// It preserves downstream client session IDs when present, and otherwise derives a stable
// session ID per account credential to ensure optimal upstream worker routing and KV-cache hits.
func opencodeHeadersHook(h http.Header, c Credentials) {
	if h.Get("User-Agent") == "" {
		h.Set("User-Agent", "opencode")
	}
	if h.Get("x-opencode-client") == "" {
		h.Set("x-opencode-client", "desktop")
	}
	if h.Get("x-opencode-session") == "" {
		sessionID := ""
		if c.ProviderSpecificData != nil {
			if s, ok := c.ProviderSpecificData["sessionId"].(string); ok {
				sessionID = strings.TrimSpace(s)
			}
		}
		if sessionID == "" {
			sessionID = deriveOpencodeSessionID(c.token())
		}
		h.Set("x-opencode-session", sessionID)
	}
	if h.Get("x-opencode-request") == "" {
		h.Set("x-opencode-request", generateOpencodeRequestID())
	}
	if h.Get("x-opencode-project") == "" {
		h.Set("x-opencode-project", "global")
	}
}
