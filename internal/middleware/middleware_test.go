package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCORS_Preflight_AllowHeaders locks the preflight Allow-Headers list for the
// browser-facing SDK surfaces. Google's @google/genai SDK sends x-goog-api-key and
// Anthropic's SDK sends anthropic-version / anthropic-dangerous-direct-browser-access;
// without these in Access-Control-Allow-Headers the browser blocks the request before
// APIKeyAuth ever runs.
func TestCORS_Preflight_AllowHeaders(t *testing.T) {
	handler := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("preflight OPTIONS must be short-circuited, not forwarded")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/v1beta/models/gemini-2.5-flash:generateContent", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 preflight, got %d", rec.Code)
	}

	allow := rec.Header().Get("Access-Control-Allow-Headers")
	lower := strings.ToLower(allow)
	for _, h := range []string{
		"x-goog-api-key",
		"anthropic-version",
		"anthropic-beta",
		"anthropic-dangerous-direct-browser-access",
		"x-api-key", // existing entry (CORS comparison is case-insensitive)
		"authorization",
	} {
		if !strings.Contains(lower, h) {
			t.Errorf("Access-Control-Allow-Headers missing %q; got %q", h, allow)
		}
	}
}

// TestSecurityHeaders_CSP_Scope locks where the CSP is emitted. Applying it to every
// response broke the user-configurable remote wallpaper (the appearance panel fetches
// an arbitrary cross-origin image and renders it) and the OAuth callback page. The CSP
// must therefore ride on the dashboard document only, while the hardening headers stay
// global.
func TestSecurityHeaders_CSP_Scope(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, tc := range []struct {
		path    string
		wantCSP bool
	}{
		{"/", true},
		{"/index.html", true},
		{"/assets/index-abc123.js", false},
		{"/api/settings", false},
		{"/api/system/backup", false},
		{"/api/usage/stream", false},
		{"/api/oauth/anthropic/callback", false},
		{"/v1/chat/completions", false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			// Hardening headers are global.
			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}
			if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
				t.Errorf("X-Frame-Options = %q, want DENY", got)
			}

			csp := rec.Header().Get("Content-Security-Policy")
			if tc.wantCSP && csp == "" {
				t.Errorf("expected CSP on dashboard document %s", tc.path)
			}
			if !tc.wantCSP && csp != "" {
				t.Errorf("unexpected CSP on %s: %q", tc.path, csp)
			}
		})
	}
}

// TestSecurityHeaders_CSP_AllowsRemoteWallpaper locks the two directives the remote
// wallpaper depends on. Dropping https: from img-src/connect-src silently breaks
// "appearance panel -> fetch cross-origin image -> render as background", while
// script-src must stay strict (the theme bootstrap is an external classic script).
func TestSecurityHeaders_CSP_AllowsRemoteWallpaper(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{
		"img-src 'self' data: blob: https:",
		"connect-src 'self' https:",
		"script-src 'self';", // must NOT contain 'unsafe-inline'
	} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP missing %q; got %q", want, csp)
		}
	}
	if strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
		t.Errorf("CSP must keep script-src strict (external theme-init.js); got %q", csp)
	}
}
