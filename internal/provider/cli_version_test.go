package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCLIVersionComparison(t *testing.T) {
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

func TestCLIVersionShape(t *testing.T) {
	for _, ok := range []string{"2.1.322", "0.5.14", "211.0.0"} {
		if !isNumericRelease(ok) {
			t.Errorf("%q must be a valid release", ok)
		}
	}
	for _, bad := range []string{"", "v2.1.322", "2.1.322-beta", "latest", "2.1..3", "2.1.322 "} {
		if isNumericRelease(bad) {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestApplyNpmTrackerVersion(t *testing.T) {
	tracker := npmReleaseTracker{name: "test", floor: "2.1.331"}

	if tracker.apply("") {
		t.Error("empty answer must report failure and re-arm the early retry")
	}
	if tracker.apply("nonsense") {
		t.Error("malformed answer must report failure")
	}
	if !tracker.apply("1.0.0") {
		t.Error("a below-floor release is a usable registry answer (floor veto, not failure)")
	}
	if v := tracker.live.Load(); v != nil {
		t.Fatalf("below-floor release must never become live: %q", *v)
	}

	if !tracker.apply("2.1.400") {
		t.Fatal("current release must be adopted")
	}
	if v := tracker.live.Load(); v == nil || *v != "2.1.400" {
		t.Fatalf("live = %v, want 2.1.400", v)
	}
	// A registry that goes quiet or lags must not walk the fingerprint back.
	if !tracker.apply("2.1.350") {
		t.Error("lagging registry answer still counts as usable")
	}
	if v := tracker.live.Load(); v == nil || *v != "2.1.400" {
		t.Fatalf("live regressed to %v", v)
	}
	if !tracker.apply("2.2.1") {
		t.Fatal("newer release must be adopted")
	}
	if v := tracker.live.Load(); v == nil || *v != "2.2.1" {
		t.Fatalf("live = %v, want 2.2.1", *v)
	}
}

func TestFetchNpmLatestVersion(t *testing.T) {
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
			if got := fetchNpmLatestVersion(srv.Client(), srv.URL); got != tc.want {
				t.Errorf("fetch = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCLIVersionPinsAndUserAgents(t *testing.T) {
	cases := []struct {
		name     string
		envKey   string
		pin      string
		getVer   func() string
		getUA    func() string
		wantUA   string
		hookName string
	}{
		{
			name: "claude", envKey: claudeCLIVersionPinEnv, pin: "9.9.1",
			getVer: ClaudeCodeVersion, getUA: ClaudeCodeUserAgent,
			wantUA: "claude-cli/9.9.1 (external, sdk-cli)", hookName: "claudeCLIUA",
		},
		{
			name: "grok-cli", envKey: grokCLIVersionPinEnv, pin: "9.9.2",
			getVer: GrokCLIVersion, getUA: GrokCLIUserAgent,
			wantUA: "grok-shell/9.9.2 (linux; x86_64)", hookName: "grokCliHeaders",
		},
		{
			name: "kimi", envKey: kimiCLIVersionPinEnv, pin: "9.9.3",
			getVer: KimiCLIVersion, getUA: KimiCLIUserAgent,
			wantUA: "KimiCLI/9.9.3", hookName: "kimiHeaders",
		},
		{
			name: "codebuddy-intl", envKey: codebuddyVersionPinEnv, pin: "9.9.4",
			getVer: CodebuddyVersion, getUA: CodebuddyIDEUA,
			wantUA: "IDE/9.9.4 CodeBuddy/9.9.4", hookName: "codebuddyIDEUA",
		},
		{
			name: "codebuddy-cn", envKey: codebuddyVersionPinEnv, pin: "9.9.4",
			getVer: CodebuddyVersion, getUA: CodebuddyCLIUA,
			wantUA: "CLI/9.9.4 CodeBuddy/9.9.4", hookName: "codebuddyCLIUA",
		},
		{
			name: "qoder", envKey: qoderCLIVersionPinEnv, pin: "9.9.5",
			getVer: QoderCLIVersion, getUA: QoderCLIUserAgent,
			wantUA: "qodercli/9.9.5",
		},
		{
			name: "opencode", envKey: opencodeVersionPinEnv, pin: "9.9.6",
			getVer: OpenCodeVersion, getUA: OpenCodeUserAgent,
			wantUA: "opencode/9.9.6", hookName: "opencodeHeaders",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.envKey, tc.pin)
			if got := tc.getVer(); got != tc.pin {
				t.Errorf("pinned version = %q, want %q", got, tc.pin)
			}
			if got := tc.getUA(); got != tc.wantUA {
				t.Errorf("UA = %q, want %q", got, tc.wantUA)
			}
			if tc.hookName != "" {
				h := http.Header{}
				authHooks[tc.hookName](h, Credentials{})
				if got := h.Get("User-Agent"); got != tc.wantUA {
					t.Errorf("hook %s UA = %q, want %q", tc.hookName, got, tc.wantUA)
				}
			}
		})
	}

	t.Run("copilot", func(t *testing.T) {
		t.Setenv(copilotChatVersionPinEnv, "0.99.0")
		t.Setenv(copilotVSCodePinEnv, "1.199.0")
		t.Setenv(copilotAPIVersionPinEnv, "2026-11-01")
		h := http.Header{}
		copilotHeadersHook(h, Credentials{})
		if got := h.Get("User-Agent"); got != "GitHubCopilotChat/0.99.0" {
			t.Errorf("User-Agent = %q", got)
		}
		if got := h.Get("Editor-Version"); got != "vscode/1.199.0" {
			t.Errorf("Editor-Version = %q", got)
		}
		if got := h.Get("Editor-Plugin-Version"); got != "copilot-chat/0.99.0" {
			t.Errorf("Editor-Plugin-Version = %q", got)
		}
		if got := h.Get("x-github-api-version"); got != "2026-11-01" {
			t.Errorf("x-github-api-version = %q", got)
		}
	})
}

func TestRegistryCarriesLiveVersionHooks(t *testing.T) {
	expected := map[string]string{
		"claude":       "claudeCLIUA",
		"codebuddy":    "codebuddyIDEUA",
		"codebuddy-cn": "codebuddyCLIUA",
		"github":       "copilotHeaders",
		"grok-cli":     "grokCliHeaders",
		"kimi":         "kimiHeaders",
		"opencode":     "opencodeHeaders",
		"opencode-go":  "opencodeHeaders",
	}
	for id, wantHook := range expected {
		p, ok := GetProvider(id)
		if !ok {
			t.Fatalf("%s provider missing from registry", id)
		}
		var found bool
		for _, h := range p.AuthHooks {
			if h == wantHook {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s registry entry must declare hook %q: %+v", id, wantHook, p.AuthHooks)
		}
		if _, ok := authHooks[wantHook]; !ok {
			t.Errorf("%q hook not registered in authHooks", wantHook)
		}
	}

	// CodeBuddy catalog fetch overrides IDE UA with live CLI UA without mutating registry map.
	cfg := ModelsFetchFor(Registry["codebuddy"])
	if got := cfg.Headers["User-Agent"]; !strings.HasPrefix(got, "CLI/") {
		t.Errorf("codebuddy ModelsFetchFor UA = %q, want CLI/...", got)
	}
}
