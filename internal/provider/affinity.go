package provider

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
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
	if len(a.entries) >= sessionAffinityMaxEntry {
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

// SessionKeyFromRawBody derives a conversation identifier for affinity routing
// from a request body in OpenAI or Anthropic shape, honoring ONLY explicit
// client-provided session fields:
//  1. prompt_cache_key / session_id / conversation_id (official cache-routing fields)
//  2. metadata.user_id (Anthropic; Claude Code embeds a per-session id there)
//
// Returns "" when none is present, which disables affinity for the request.
// Deliberately NO content-hash fallback: two unrelated users opening with the
// same sentence must not be pinned to one account (load skew + cross-tenant
// cache coupling). Clients that want affinity send an id.
func SessionKeyFromRawBody(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	return SessionKeyFromBodyMap(m)
}

// SessionKeyFromBodyMap is SessionKeyFromRawBody for an already parsed body.
func SessionKeyFromBodyMap(bodyMap map[string]any) string {
	for _, key := range []string{"prompt_cache_key", "session_id", "conversation_id"} {
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

// FirstUserText returns the text of the first user message, as a plain string
// or concatenated text blocks (Anthropic content arrays). Used for upstream
// conversation tagging (CodeBuddy X-Conversation-ID), where conflating similar
// openings is harmless — NOT for routing affinity.
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
