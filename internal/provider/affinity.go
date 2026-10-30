package provider

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Session affinity keeps one conversation pinned to the same upstream account so
// provider-side prompt KV caches stay warm. Plain round-robin spreads the
// consecutive turns of a conversation across accounts, forcing every request to
// re-warm (or fully miss) the upstream prefix cache and inflating token cost.
const (
	sessionAffinityTTL      = 30 * time.Minute
	sessionAffinityMaxEntry = 8192
)

type affinityEntry struct {
	connID string
	seen   time.Time
}

type sessionAffinity struct {
	mu      sync.Mutex
	entries map[string]affinityEntry
}

var credentialAffinity = &sessionAffinity{entries: make(map[string]affinityEntry)}

// get returns the bound connection for a session key, applying a sliding TTL:
// every hit refreshes the expiry while the conversation stays active.
func (a *sessionAffinity) get(key string, now time.Time) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	e, ok := a.entries[key]
	if !ok {
		return "", false
	}
	if now.Sub(e.seen) > sessionAffinityTTL {
		delete(a.entries, key)
		return "", false
	}
	e.seen = now
	a.entries[key] = e
	return e.connID, true
}

func (a *sessionAffinity) bind(key, connID string, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	// Rebinding an existing session never grows the table; skip the eviction sweep.
	if _, exists := a.entries[key]; !exists && len(a.entries) >= sessionAffinityMaxEntry {
		a.evictLocked(now)
	}
	a.entries[key] = affinityEntry{connID: connID, seen: now}
}

// evictLocked drops expired entries first, then the least recently seen ones
// until there is room again.
func (a *sessionAffinity) evictLocked(now time.Time) {
	for k, e := range a.entries {
		if now.Sub(e.seen) > sessionAffinityTTL {
			delete(a.entries, k)
		}
	}
	for len(a.entries) >= sessionAffinityMaxEntry {
		var oldestKey string
		var oldest time.Time
		for k, e := range a.entries {
			if oldestKey == "" || e.seen.Before(oldest) {
				oldestKey, oldest = k, e.seen
			}
		}
		delete(a.entries, oldestKey)
	}
}

// ResetSessionAffinity clears all bindings. Test isolation helper.
func ResetSessionAffinity() {
	credentialAffinity.mu.Lock()
	defer credentialAffinity.mu.Unlock()
	credentialAffinity.entries = make(map[string]affinityEntry)
}

// SessionKeyFromBodyMap derives an explicit client-provided session id from a
// request body in OpenAI or Anthropic shape, honoring ONLY declared fields:
//  1. prompt_cache_key / session_id / conversation_id / user (official cache-routing fields)
//  2. metadata.user_id (Anthropic; Claude Code embeds a per-session id there)
//
// Returns "" when none is present. Header sniffing and the content-fingerprint
// fallback live in the handler layer (requestSessionKey), which owns the full
// tenant scope; this function stays a pure body extractor.
func SessionKeyFromBodyMap(bodyMap map[string]any) string {
	for _, key := range []string{"prompt_cache_key", "session_id", "conversation_id", "user"} {
		if val, ok := bodyMap[key].(string); ok && val != "" {
			return BoundSessionKey("pk:" + val)
		}
	}
	if meta, ok := bodyMap["metadata"].(map[string]any); ok {
		if uid, ok := meta["user_id"].(string); ok && uid != "" {
			return BoundSessionKey("mu:" + uid)
		}
	}
	return ""
}

// sessionHeaders is the allowlist of generic agent client headers that carry
// session (not request) identity: X-Session-Id and X-Conversation-Id are the
// de-facto names, X-Grok-Conv-Id is xAI's conversation header. Request-level
// ids (x-request-id, x-client-request-id...) are deliberately excluded: they
// change every turn and would pin each turn to a fresh account — worse than
// no affinity at all.
var sessionHeaders = []string{"X-Session-Id", "X-Conversation-Id", "X-Grok-Conv-Id"}

// SessionKeyFromHeaders scans the allowlisted session headers. Values must be
// 4-128 chars of [0-9A-Za-z._:@-]; anything else (spaces, control bytes,
// oversized blobs, PII sentences) is ignored so the routing table can only
// ever be keyed on sane opaque tokens.
func SessionKeyFromHeaders(h http.Header) string {
	for _, name := range sessionHeaders {
		if v := h.Get(name); validSessionToken(v) {
			return BoundSessionKey("hd:" + strings.ToLower(name) + "|" + v)
		}
	}
	return ""
}

func validSessionToken(v string) bool {
	if len(v) < 4 || len(v) > 128 {
		return false
	}
	return strings.IndexFunc(v, func(r rune) bool {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			return false
		case r == '.' || r == '_' || r == ':' || r == '@' || r == '-':
			return false
		}
		return true
	}) < 0
}

// SessionKeyFromContentFallback derives a synthetic session id from the
// conversation's immutable prefix — system prompt + first user message — hashed
// under a tenant scope. Most agent clients (pi, opencode, VS Code BYOK, aider)
// send no session field or header at all, so without this tier the KV-cache
// pinning never reaches them.
//
// Scoping is the safeguard against the original objection to content hashing:
// the caller must pass a per-tenant scope (API key id, or client address in
// open mode), so two unrelated tenants opening with the same sentence can
// never collide into one pin. Within one tenant, conflating two conversations
// that open with byte-identical prompts is acceptable: they share the same
// prefix anyway, and mis-pinning only costs balance, never correctness.
// The fingerprint never leaves this process — only its SHA-1 goes into the
// affinity table.
func SessionKeyFromContentFallback(bodyMap map[string]any, scope string) string {
	sys := sessionSystemText(bodyMap)
	user := FirstUserText(bodyMap)
	if sys == "" && user == "" {
		return ""
	}
	h := sha1.Sum([]byte(scope + "\x00" + sys + "\x00" + user))
	return "c:" + hex.EncodeToString(h[:])
}

// sessionSystemText returns the conversation's system/instructions text in
// either wire shape: Anthropic top-level `system` (string or content blocks),
// OpenAI `instructions`, or the leading system/developer message.
func sessionSystemText(bodyMap map[string]any) string {
	if s := joinTextBlocks(bodyMap["system"]); s != "" {
		return s
	}
	if ins, ok := bodyMap["instructions"].(string); ok {
		return ins
	}
	msgs, _ := bodyMap["messages"].([]any)
	for _, m := range msgs {
		msgMap, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if role, _ := msgMap["role"].(string); role == "system" || role == "developer" {
			if t := joinTextBlocks(msgMap["content"]); t != "" {
				return t
			}
		}
	}
	return ""
}

// joinTextBlocks renders a content field that is either a plain string or an
// array of {text:...} blocks (Gemini/Claude/OpenAI multi-part styles).
func joinTextBlocks(v any) string {
	switch c := v.(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, b := range c {
			if bMap, ok := b.(map[string]any); ok {
				if text, ok := bMap["text"].(string); ok && text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// FirstUserText returns the text of the first user message, as a plain string
// or concatenated text blocks (Anthropic content arrays). Used for upstream
// conversation tagging (CodeBuddy X-Conversation-ID) and as part of the
// content-fingerprint affinity fallback.
func FirstUserText(bodyMap map[string]any) string {
	msgs, ok := bodyMap["messages"].([]any)
	if !ok {
		return ""
	}
	for _, m := range msgs {
		msgMap, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if role, _ := msgMap["role"].(string); role != "user" {
			continue
		}
		switch c := msgMap["content"].(type) {
		case string:
			if c != "" {
				return c
			}
		case []any:
			var parts []string
			for _, b := range c {
				if bMap, ok := b.(map[string]any); ok {
					if text, ok := bMap["text"].(string); ok && text != "" {
						parts = append(parts, text)
					}
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, "\n")
			}
		}
	}
	return ""
}

// BoundSessionKey caps oversized client identifiers so the affinity table cannot
// be bloated through arbitrarily long keys.
func BoundSessionKey(k string) string {
	if len(k) <= 160 {
		return k
	}
	h := sha1.Sum([]byte(k))
	return k[:128] + ":" + hex.EncodeToString(h[:4])
}
