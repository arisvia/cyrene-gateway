package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestHandleRegistryEnrichedCapabilities(t *testing.T) {
	srv, _ := setupTestServer(t)

	// 1. Test GET /api/registry (grouped by categories)
	req := httptest.NewRequest("GET", "/api/registry", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Categories []struct {
			Category  string `json:"category"`
			Count     int    `json:"count"`
			Providers []struct {
				ID           string   `json:"id"`
				Name         string   `json:"name"`
				Capabilities []string `json:"capabilities"`
			} `json:"providers"`
		} `json:"categories"`
		Total int `json:"total"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal registry response: %v", err)
	}

	if resp.Total == 0 || len(resp.Categories) == 0 {
		t.Fatalf("expected non-empty categories in registry response")
	}

	// Verify that 'media' category is present in the response
	foundMediaCategory := false
	for _, cat := range resp.Categories {
		if cat.Category == "media" {
			foundMediaCategory = true
			if len(cat.Providers) == 0 {
				t.Errorf("media category should contain synthesized pure media providers")
			}
			for _, p := range cat.Providers {
				if len(p.Capabilities) == 0 {
					t.Errorf("pure media provider %q has empty capabilities", p.ID)
				}
				for _, cap := range p.Capabilities {
					if cap == "llm" {
						t.Errorf("pure media provider %q should not have llm capability", p.ID)
					}
				}
			}
		}
	}
	if !foundMediaCategory {
		t.Errorf("expected 'media' category to be synthesized in /api/registry")
	}

	// Verify openai has both llm and media capabilities (image, tts, stt, embedding)
	var openaiCaps []string
	for _, cat := range resp.Categories {
		for _, p := range cat.Providers {
			if p.ID == "openai" {
				openaiCaps = p.Capabilities
				break
			}
		}
	}
	if len(openaiCaps) == 0 {
		t.Fatalf("openai provider not found in registry")
	}

	hasLLM := false
	hasImage := false
	hasEmbedding := false
	for _, c := range openaiCaps {
		if c == "llm" {
			hasLLM = true
		}
		if c == "image" {
			hasImage = true
		}
		if c == "embedding" {
			hasEmbedding = true
		}
	}
	if !hasLLM || !hasImage || !hasEmbedding {
		t.Errorf("openai missing expected capabilities: %v", openaiCaps)
	}

	// 2. Test GET /api/registry?category=media
	reqMedia := httptest.NewRequest("GET", "/api/registry?category=media", nil)
	wMedia := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wMedia, reqMedia)
	if wMedia.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wMedia.Code)
	}

	var mediaResp struct {
		Providers []struct {
			ID           string   `json:"id"`
			Capabilities []string `json:"capabilities"`
		} `json:"providers"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(wMedia.Body.Bytes(), &mediaResp); err != nil {
		t.Fatalf("failed to unmarshal media registry response: %v", err)
	}
	if mediaResp.Count == 0 {
		t.Errorf("expected providers in /api/registry?category=media")
	}
}

func TestHandleMediaProvidersConnectedFiltering(t *testing.T) {
	srv, database := setupTestServer(t)

	// Inactive / No connection initially
	req := httptest.NewRequest("GET", "/api/media-providers?connected=true", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var emptyResp struct {
		Providers []struct {
			Provider string `json:"provider"`
		} `json:"providers"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &emptyResp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if emptyResp.Count != 0 {
		t.Errorf("expected 0 connected providers, got %d", emptyResp.Count)
	}

	// Add an active connection for antigravity
	conn := &model.ProviderConnection{
		ID:       "test-antigravity-conn",
		Provider: "antigravity",
		AuthType: "oauth",
		IsActive: true,
		Data: model.ConnectionData{
			AccessToken: "test-token",
		},
	}
	if err := database.CreateConnection(conn); err != nil {
		t.Fatalf("failed to create connection: %v", err)
	}

	// Query again with connected=true
	req = httptest.NewRequest("GET", "/api/media-providers?connected=true&kind=image", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var connectedResp struct {
		Providers []struct {
			Provider          string `json:"provider"`
			HasConnection     bool   `json:"hasConnection"`
			ActiveConnections int    `json:"activeConnections"`
		} `json:"providers"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &connectedResp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if connectedResp.Count != 1 {
		t.Fatalf("expected exactly 1 connected provider, got %d", connectedResp.Count)
	}
	if connectedResp.Providers[0].Provider != "antigravity" {
		t.Errorf("expected antigravity, got %s", connectedResp.Providers[0].Provider)
	}
	if !connectedResp.Providers[0].HasConnection || connectedResp.Providers[0].ActiveConnections != 1 {
		t.Errorf("expected HasConnection=true and ActiveConnections=1")
	}

	// Query all providers (without connected=true) -> should return all registered image providers
	reqAll := httptest.NewRequest("GET", "/api/media-providers?kind=image", nil)
	wAll := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wAll, reqAll)

	var allResp struct {
		Providers []struct {
			Provider      string `json:"provider"`
			HasConnection bool   `json:"hasConnection"`
		} `json:"providers"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(wAll.Body.Bytes(), &allResp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if allResp.Count <= 1 {
		t.Errorf("expected multiple image providers in all listing, got %d", allResp.Count)
	}
}
