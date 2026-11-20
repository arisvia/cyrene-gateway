package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClaudeCLIVersionComparison(t *testing.T) {
	cases := []struct {
		a, b string
		less bool
	}{
		{"2.1.321", "2.1.322", true},
		{"2.1.322", "2.1.322", false},
		{"2.1.322", "2.2.0", true},
		{"2.9.0", "2.10.0", true},
		{"2.1.322", "2.1", false},
		{"2.1", "2.1.1", true},
		{"3.0.0", "2.99.99", false},
	}
	for _, tc := range cases {
		if got := versionLess(tc.a, tc.b); got != tc.less {
			t.Errorf("versionLess(%q,%q)=%t want %t", tc.a, tc.b, got, tc.less)
		}
	}
}

func TestClaudeCLIVersionShape(t *testing.T) {
	for _, ok := range []string{"2.1.322", "0.5.14", "211.0.0"} {
		if !isClaudeCLIVersion(ok) {
			t.Errorf("%q must be a valid release", ok)
		}
	}
	for _, bad := range []string{"", "v2.1.322", "2.1.322-beta", "latest", "2.1..3", "2.1.322 "} {
		if isClaudeCLIVersion(bad) {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestApplyClaudeCLIVersion(t *testing.T) {
	old := claudeCLILive.Load()
	t.Cleanup(func() { claudeCLILive.Store(old) })
	claudeCLILive.Store(nil)

	if applyClaudeCLIVersion("") {
		t.Error("empty answer must report failure and re-arm the early retry")
	}
	if applyClaudeCLIVersion("nonsense") {
		t.Error("malformed answer must report failure")
	}
	if !applyClaudeCLIVersion("1.0.0") {
		t.Error("a below-floor release is a usable registry answer (floor veto, not failure)")
	}
	if v := claudeCLILive.Load(); v != nil {
		t.Fatalf("below-floor release must never become live: %q", *v)
	}

	if !applyClaudeCLIVersion("2.1.400") {
		t.Fatal("current release must be adopted")
	}
	v := claudeCLILive.Load()
	if v == nil || *v != "2.1.400" {
		t.Fatalf("live = %v, want 2.1.400", v)
	}
	// A registry that goes quiet or lags must not walk the fingerprint back.
	if !applyClaudeCLIVersion("2.1.350") {
		t.Error("lagging registry answer still counts as usable")
	}
	if v := claudeCLILive.Load(); v == nil || *v != "2.1.400" {
		t.Fatalf("live regressed to %v", v)
	}
	if !applyClaudeCLIVersion("2.2.1") {
		t.Fatal("newer release must be adopted")
	}
	if v := claudeCLILive.Load(); v == nil || *v != "2.2.1" {
		t.Fatalf("live = %v, want 2.2.1", *v)
	}
}

func TestFetchClaudeCLIVersion(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   any
		want   string
	}{{
		name: "registry document", status: 200, body: map[string]any{"name": "@anthropic-ai/claude-code", "dist-tags": map[string]string{"latest": "2.1.400"}, "version": "2.1.400"}, want: "2.1.400",
	}, {
		name: "trims", status: 200, body: map[string]any{"version": " 2.1.401 "}, want: "2.1.401",
	}, {
		name: "error page", status: 500, body: "nope", want: "",
	}, {
		name: "no version field", status: 200, body: map[string]any{"name": "x"}, want: "",
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				if s, ok := tc.body.(string); ok {
					_, _ = w.Write([]byte(s))
					return
				}
				_ = json.NewEncoder(w).Encode(tc.body)
			}))
			defer srv.Close()
			if got := fetchClaudeCLIVersion(srv.Client(), srv.URL); got != tc.want {
				t.Errorf("fetch = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClaudeCodeVersionPinAndUserAgent(t *testing.T) {
	t.Setenv(claudeCLIVersionPinEnv, "9.9.9")
	if got := ClaudeCodeVersion(); got != "9.9.9" {
		t.Errorf("pinned version = %q, want 9.9.9", got)
	}
	h := http.Header{}
	claudeCLIUAHook(h, Credentials{})
	if got := h.Get("User-Agent"); got != "claude-cli/9.9.9 (external, sdk-cli)" {
		t.Errorf("hooked UA = %q, want the pinned fingerprint", got)
	}
}

func TestClaudeRegistryCarriesLiveVersionHook(t *testing.T) {
	// The whole point of the hook: the static registry map alone would freeze
	// whatever the fingerprint was at package init.
	p, ok := GetProvider("claude")
	if !ok {
		t.Fatal("claude provider missing from registry")
	}
	var found bool
	for _, h := range p.AuthHooks {
		if h == "claudeCLIUA" {
			found = true
		}
	}
	if !found {
		t.Fatalf("claude registry entry must declare the claudeCLIUA hook: %+v", p.AuthHooks)
	}
	if _, ok := authHooks["claudeCLIUA"]; !ok {
		t.Fatal("claudeCLIUA hook not registered")
	}
}
