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
