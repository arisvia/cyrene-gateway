package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// TestOAuthCallbackStateValidation enforces mandatory CSRF state checks on the OAuth callback.
func TestOAuthCallbackStateValidation(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Case 1: Missing state parameter must be rejected with 400
	req := httptest.NewRequest("GET", "/api/oauth/github/callback?code=mock_code", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing state: got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "missing state parameter") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}

	// Case 2: Invalid/expired state must be rejected with 400
	req = httptest.NewRequest("GET", "/api/oauth/github/callback?code=mock_code&state=nonexistent_state", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid state: got status %d, want %d", w.Code, http.StatusBadRequest)
	}

	// Case 3: State provider mismatch must be rejected with 400
	provider.StoreSession("mismatched_state", &provider.OAuthSession{
		Provider:  "google",
		CreatedAt: time.Now(),
		PKCE: &provider.PKCE{
			CodeVerifier: "verifier",
			State:        "mismatched_state",
		},
		RedirectURI: "http://localhost/callback",
	})
	defer provider.ClearSession("mismatched_state")

	req = httptest.NewRequest("GET", "/api/oauth/github/callback?code=mock_code&state=mismatched_state", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("mismatched state: got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "state provider mismatch") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}
