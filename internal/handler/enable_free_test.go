package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// countNoAuth returns the number of NoAuth providers in the registry.
func countNoAuth() int {
	n := 0
	for _, info := range provider.Registry {
		if info.NoAuth {
			n++
		}
	}
	return n
}

func TestEnableFreeProviders(t *testing.T) {
	srv, database := setupTestServer(t)

	total := countNoAuth()
	if total == 0 {
		t.Fatal("expected at least one NoAuth provider in the registry")
	}

	// Enable all free providers (empty body).
	req := httptest.NewRequest("POST", "/api/providers/enable-free", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Enabled []string `json:"enabled"`
		Skipped []string `json:"skipped"`
		Count   int      `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Count != total {
		t.Fatalf("expected %d enabled, got %d (%v)", total, resp.Count, resp.Enabled)
	}

	// Connections should now exist for every NoAuth provider, all authType none.
	conns, _ := database.ListConnections()
	if len(conns) != total {
		t.Fatalf("expected %d connections, got %d", total, len(conns))
	}
	for _, c := range conns {
		if c.AuthType != "none" {
			t.Fatalf("expected authType=none for %s, got %s", c.Provider, c.AuthType)
		}
		if !c.IsActive {
			t.Fatalf("expected enabled connection %s to be active", c.Provider)
		}
	}

	// Second call should be idempotent: nothing new enabled, all skipped.
	req = httptest.NewRequest("POST", "/api/providers/enable-free", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on second call, got %d", w.Code)
	}
	var resp2 struct {
		Enabled []string `json:"enabled"`
		Skipped []string `json:"skipped"`
		Count   int      `json:"count"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp2)
	if resp2.Count != 0 {
		t.Fatalf("expected 0 enabled on second call, got %d (%v)", resp2.Count, resp2.Enabled)
	}
	if len(resp2.Skipped) != total {
		t.Fatalf("expected %d skipped on second call, got %d", total, len(resp2.Skipped))
	}
}

func TestEnableFreeProvidersSelective(t *testing.T) {
	srv, database := setupTestServer(t)

	// Find a NoAuth provider to target.
	var target string
	for id, info := range provider.Registry {
		if info.NoAuth {
			target = id
			break
		}
	}
	if target == "" {
		t.Fatal("no NoAuth provider found in registry")
	}

	body := `{"providers":["` + target + `"]}`
	req := httptest.NewRequest("POST", "/api/providers/enable-free", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Enabled []string `json:"enabled"`
		Count   int      `json:"count"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Count != 1 || len(resp.Enabled) != 1 || resp.Enabled[0] != target {
		t.Fatalf("expected exactly [%s] enabled, got %v", target, resp.Enabled)
	}

	conns, _ := database.ListConnections()
	if len(conns) != 1 || conns[0].Provider != target {
		t.Fatalf("expected single connection for %s, got %v", target, conns)
	}
}

func TestEnableFreeProvidersSkipsExisting(t *testing.T) {
	srv, database := setupTestServer(t)

	// Pre-create a connection for a NoAuth provider with a custom name.
	var target string
	for id, info := range provider.Registry {
		if info.NoAuth {
			target = id
			break
		}
	}
	if target == "" {
		t.Fatal("no NoAuth provider found in registry")
	}
	database.CreateConnection(&model.ProviderConnection{
		ID:       "pre-existing",
		Provider: target,
		AuthType: "none",
		Name:     "custom",
		IsActive: true,
	})

	req := httptest.NewRequest("POST", "/api/providers/enable-free", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// The pre-existing connection must be preserved (still named "custom").
	conn, err := database.GetConnection("pre-existing")
	if err != nil {
		t.Fatalf("pre-existing connection lost: %v", err)
	}
	if conn.Name != "custom" {
		t.Fatalf("pre-existing connection was modified, name=%s", conn.Name)
	}
}
