package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/auth"
)

func TestHandleEndpoints_CurrentDomainAndDeduplication(t *testing.T) {
	srv, _ := setupTestServer(t)

	// 1. Authenticated request via public domain with HTTPS proxy header
	token, err := auth.CreateSessionToken()
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}
	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	req.Host = "internal-server:8080"
	req.Header.Set("X-Forwarded-Host", "ai.mycompany.com")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Endpoints []struct {
			Label string `json:"label"`
			URL   string `json:"url"`
			Type  string `json:"type"`
		} `json:"endpoints"`
		RequireAuth bool `json:"requireAuth"`
		Port        int  `json:"port"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Endpoints) == 0 {
		t.Fatal("expected non-empty endpoints")
	}

	// First entry must be the current domain
	first := resp.Endpoints[0]
	if first.Type != "current" {
		t.Errorf("expected first endpoint to be 'current', got %s", first.Type)
	}
	if first.URL != "https://ai.mycompany.com" {
		t.Errorf("expected URL https://ai.mycompany.com, got %s", first.URL)
	}

	// 2. Request via localhost - should deduplicate with localhost entry
	req2 := httptest.NewRequest("GET", "/api/endpoints", nil)
	req2.Host = "localhost:20128"
	w2 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w2, req2)

	var resp2 struct {
		Endpoints []struct {
			Label string `json:"label"`
			URL   string `json:"url"`
			Type  string `json:"type"`
		} `json:"endpoints"`
	}
	json.Unmarshal(w2.Body.Bytes(), &resp2)

	// Check for URL duplicates
	seen := make(map[string]int)
	for _, ep := range resp2.Endpoints {
		seen[ep.URL]++
		if seen[ep.URL] > 1 {
			t.Errorf("duplicate endpoint URL found: %s", ep.URL)
		}
	}
}
