package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestSessionKeyFromBodyMap(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantPrefix string
		wantEmpty  bool
	}{
		{"prompt_cache_key wins", `{"prompt_cache_key":"pc-1","session_id":"s-2","messages":[{"role":"user","content":"x"}]}`, "pk:pc-1", false},
		{"session_id", `{"session_id":"s-2","messages":[{"role":"user","content":"x"}]}`, "pk:s-2", false},
		{"conversation_id", `{"conversation_id":"c-3","messages":[{"role":"user","content":"x"}]}`, "pk:c-3", false},
		{"openai user field", `{"user":"u-9","messages":[{"role":"user","content":"x"}]}`, "pk:u-9", false},
		{"anthropic metadata user id", `{"messages":[{"role":"user","content":"x"}],"metadata":{"user_id":"{\"session_id\":\"abc\"}"}}`, "mu:", false},
		{"first user string is NOT an explicit key", `{"messages":[{"role":"system","content":"s"},{"role":"user","content":"hello world"}]}`, "", true},
		{"first user blocks is NOT an explicit key", `{"messages":[{"role":"user","content":[{"type":"text","text":"a"},{"type":"image_url","image_url":{"url":"u"}}]}]}`, "", true},
		{"no user message", `{"model":"gpt-4o","messages":[{"role":"assistant","content":"x"}]}`, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var m map[string]any
			if err := json.Unmarshal([]byte(tc.body), &m); err != nil {
				t.Fatal(err)
			}
			got := SessionKeyFromBodyMap(m)
			if tc.wantEmpty {
				if got != "" {
					t.Fatalf("expected empty key, got %q", got)
				}
				return
			}
			if !strings.HasPrefix(got, tc.wantPrefix) {
				t.Fatalf("key %q missing prefix %q", got, tc.wantPrefix)
			}
			if again := SessionKeyFromBodyMap(m); again != got {
				t.Fatalf("key must be deterministic: %q vs %q", got, again)
			}
		})
	}
}

func TestSessionKeyFromHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("X-Session-Id", "sess_01JABC")
	if got := SessionKeyFromHeaders(h); got != "hd:x-session-id|sess_01JABC" {
		t.Fatalf("allowlisted session header must be keyed, got %q", got)
	}

	// Request-scoped ids change every turn and must never feed affinity.
	h = http.Header{}
	h.Set("X-Client-Request-Id", "req-1234")
	h.Set("X-Request-Id", "req-5678")
	if got := SessionKeyFromHeaders(h); got != "" {
		t.Fatalf("request ids must be ignored, got %q", got)
	}

	// Junk values (spaces, control chars, out of length bounds) are rejected so
	// the routing table cannot be keyed on arbitrary payloads.
	for _, bad := range []string{"", "ab", "has spaces in it", strings.Repeat("x", 200), "line\nbreak"} {
		h = http.Header{}
		h.Set("X-Conversation-Id", bad)
		if got := SessionKeyFromHeaders(h); got != "" {
			t.Fatalf("value %q must be rejected, got %q", bad, got)
		}
	}
}

func TestSessionKeyFromContentFallback(t *testing.T) {
	parse := func(s string) map[string]any {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	convA := `{"messages":[{"role":"system","content":"sys"},{"role":"user","content":"first turn"}]}`
	convAExtended := `{"messages":[{"role":"system","content":"sys"},{"role":"user","content":"first turn"},{"role":"assistant","content":"a"},{"role":"user","content":"second turn"}]}`
	convB := `{"messages":[{"role":"system","content":"sys"},{"role":"user","content":"other opener"}]}`

	keyA := SessionKeyFromContentFallback(parse(convA), "k:key1")
	if keyA == "" || !strings.HasPrefix(keyA, "c:") {
		t.Fatalf("fingerprint must be a c: key, got %q", keyA)
	}
	if grew := SessionKeyFromContentFallback(parse(convAExtended), "k:key1"); grew != keyA {
		t.Fatalf("later turns of the same conversation must reproduce the key: %q vs %q", keyA, grew)
	}
	if other := SessionKeyFromContentFallback(parse(convB), "k:key1"); other == keyA {
		t.Fatal("different openers must not share a fingerprint")
	}
	if crossKey := SessionKeyFromContentFallback(parse(convA), "k:key2"); crossKey == keyA {
		t.Fatal("same content under a different API key must not share a fingerprint")
	}
	// Anthropic shape: top-level system string + block content array.
	anth := `{"system":"You are...","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`
	if k := SessionKeyFromContentFallback(parse(anth), "ip:1.2.3.4"); !strings.HasPrefix(k, "c:") {
		t.Fatalf("anthropic shape must fingerprint, got %q", k)
	}
	if k := SessionKeyFromContentFallback(parse(`{"model":"gpt-4o","messages":[{"role":"assistant","content":"x"}]}`), "k:key1"); k != "" {
		t.Fatalf("content-free body must produce no fallback, got %q", k)
	}
}

func TestBoundSessionKeyCapsOversizedIDs(t *testing.T) {
	key := BoundSessionKey("pk:" + strings.Repeat("x", 5000))
	if len(key) > 160 {
		t.Fatalf("key length %d exceeds cap", len(key))
	}
}

func stickyPool(providerName string) []model.ProviderConnection {
	return []model.ProviderConnection{
		{ID: providerName + "-a", Provider: providerName, AuthType: "api-key", IsActive: true, Data: model.ConnectionData{APIKey: "ka"}},
		{ID: providerName + "-b", Provider: providerName, AuthType: "api-key", IsActive: true, Data: model.ConnectionData{APIKey: "kb"}},
	}
}

func TestStickyKeepsConversationOnOneAccount(t *testing.T) {
	conns := stickyPool("affinity-pin")
	first := SelectCredentialSticky(conns, "", nil, nil, "sess-pin")
	if first == nil {
		t.Fatal("expected a connection")
	}
	for i := 1; i < 5; i++ {
		got := SelectCredentialSticky(conns, "", nil, nil, "sess-pin")
		if got == nil || got.ID != first.ID {
			t.Fatalf("turn %d left account %s (got %v)", i, first.ID, got)
		}
	}
	// Control: without a session key the same pool still round-robins.
	a := SelectCredentialSticky(conns, "", nil, nil, "")
	b := SelectCredentialSticky(conns, "", nil, nil, "")
	if a.ID == b.ID {
		t.Fatal("keyless selection must keep alternating")
	}
}

func TestStickyRebindsWhenBoundAccountCoolsDown(t *testing.T) {
	conns := stickyPool("affinity-cool")
	bound := SelectCredentialSticky(conns, "", nil, nil, "sess-cool")
	for i := range conns {
		if conns[i].ID == bound.ID {
			conns[i].Data.RateLimitedUntil = time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
		}
	}
	got := SelectCredentialSticky(conns, "", nil, nil, "sess-cool")
	if got == nil || got.ID == bound.ID {
		t.Fatalf("cooled-down account must not stay pinned, got %v", got)
	}
	again := SelectCredentialSticky(conns, "", nil, nil, "sess-cool")
	if again.ID != got.ID {
		t.Fatalf("conversation must rebind to the surviving account: %s vs %s", again.ID, got.ID)
	}
}

func TestStickyRespectsExclusionsAndModelLocks(t *testing.T) {
	conns := stickyPool("affinity-excl")
	bound := SelectCredentialSticky(conns, "", nil, nil, "sess-excl")
	got := SelectCredentialSticky(conns, "", map[string]bool{bound.ID: true}, nil, "sess-excl")
	if got == nil || got.ID == bound.ID {
		t.Fatalf("excluded account must not be pinned, got %v", got)
	}

	lockKey := modelLockPrefix + "gpt-test"
	for i := range conns {
		if conns[i].ID == got.ID {
			if conns[i].Data.ProviderSpecificData == nil {
				conns[i].Data.ProviderSpecificData = map[string]any{}
			}
			conns[i].Data.ProviderSpecificData[lockKey] = time.Now().Add(time.Minute).UTC().Format(time.RFC3339)
		}
	}
	next := SelectCredentialSticky(conns, "gpt-test", nil, nil, "sess-excl")
	if next == nil || next.ID == got.ID {
		t.Fatalf("model-locked account must not be pinned, got %v", next)
	}
}

func TestStickySkipsQuotaExhaustedBinding(t *testing.T) {
	providerName := "affinity-quota"
	conns := stickyPool(providerName)
	for i := range conns {
		conns[i].Data.QuotaLimit = 10
		conns[i].Data.QuotaPeriod = "daily"
	}
	bound := SelectCredentialSticky(conns, "", nil, func(string, string) int { return 0 }, "sess-quota")
	quota := func(id string, _ string) int {
		if id == bound.ID {
			return 10
		}
		return 0
	}
	got := SelectCredentialSticky(conns, "", nil, quota, "sess-quota")
	if got == nil || got.ID == bound.ID {
		t.Fatalf("over-quota account must not stay pinned, got %v", got)
	}
}

func TestStickyReturnsToHigherPriorityTierWhenItRecovers(t *testing.T) {
	conns := []model.ProviderConnection{
		{ID: "tier-pri", Provider: "affinity-tier", AuthType: "api-key", IsActive: true, Priority: 0, Data: model.ConnectionData{APIKey: "k1"}},
		{ID: "tier-bak", Provider: "affinity-tier", AuthType: "api-key", IsActive: true, Priority: 5, Data: model.ConnectionData{APIKey: "k2"}},
	}
	// Primary tier is cooling: the conversation lands on the backup tier.
	conns[0].Data.RateLimitedUntil = time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
	got := SelectCredentialSticky(conns, "", nil, nil, "sess-tier")
	if got == nil || got.ID != "tier-bak" {
		t.Fatalf("expected backup tier while primary cools, got %v", got)
	}
	// Primary recovers: the active session must migrate back — priority is
	// never traded away for cache warmth.
	conns[0].Data.RateLimitedUntil = ""
	got = SelectCredentialSticky(conns, "", nil, nil, "sess-tier")
	if got == nil || got.ID != "tier-pri" {
		t.Fatalf("recovered higher tier must reclaim the session, got %v", got)
	}
	again := SelectCredentialSticky(conns, "", nil, nil, "sess-tier")
	if again.ID != "tier-pri" {
		t.Fatalf("session must stay on the reclaimed primary account, got %s", again.ID)
	}
}

func TestAffinitySlidingTTL(t *testing.T) {
	a := &sessionAffinity{entries: make(map[string]affinityEntry)}
	t0 := time.Now()
	a.bind("ttl|s", "conn-x", t0)
	if _, ok := a.get("ttl|s", t0.Add(20*time.Minute)); !ok {
		t.Fatal("entry within TTL must survive")
	}
	// The previous get refreshed seen to t0+20m, so t0+40m is still alive.
	if _, ok := a.get("ttl|s", t0.Add(40*time.Minute)); !ok {
		t.Fatal("active conversation must not expire (sliding TTL)")
	}
	if _, ok := a.get("ttl|s", t0.Add(40*time.Minute+sessionAffinityTTL+time.Minute)); ok {
		t.Fatal("idle entry past TTL must expire")
	}
}

func TestAffinityEvictsWhenFull(t *testing.T) {
	a := &sessionAffinity{entries: make(map[string]affinityEntry)}
	t0 := time.Now()
	for i := range sessionAffinityMaxEntry + 1 {
		a.bind(fmt.Sprintf("evict|%d", i), fmt.Sprintf("conn-%d", i), t0.Add(time.Duration(i)*time.Millisecond))
	}
	if len(a.entries) > sessionAffinityMaxEntry {
		t.Fatalf("table exceeded cap: %d", len(a.entries))
	}
	if _, ok := a.get("evict|0", t0.Add(time.Duration(sessionAffinityMaxEntry)*time.Millisecond)); ok {
		t.Fatal("least recently seen entry must have been evicted")
	}
}
