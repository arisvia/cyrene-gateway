package handler

import (
	"bytes"
	"encoding/json"
	"io"
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

func TestAggregateAntigravitySearchResponse(t *testing.T) {
	srv, _ := setupTestServer(t)

	// 1. Test success response with Google response.candidates envelope
	mockRespJSON := `{
		"response": {
			"candidates": [
				{
					"content": {
						"parts": [
							{"text": "Noah Lyles won the 2024 Olympic 100m gold."}
						]
					},
					"groundingMetadata": {
						"webSearchQueries": ["2024 olympics 100m gold winner"],
						"groundingChunks": [
							{
								"web": {
									"uri": "https://en.wikipedia.org/wiki/Athletics_at_the_2024_Summer_Olympics",
									"title": "Wikipedia"
								}
							}
						]
					}
				}
			]
		}
	}`

	httpResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader([]byte(mockRespJSON))),
	}

	w := httptest.NewRecorder()
	srv.aggregateAntigravitySearchResponse(w, httpResp, "who won 100m")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var out struct {
		Provider string `json:"provider"`
		Query    string `json:"query"`
		Count    int    `json:"count"`
		Summary  string `json:"summary"`
		Results  []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if out.Provider != "antigravity" || out.Query != "who won 100m" || out.Count != 1 {
		t.Errorf("unexpected output: %+v", out)
	}
	if out.Summary != "Noah Lyles won the 2024 Olympic 100m gold." {
		t.Errorf("unexpected summary: %q", out.Summary)
	}
	if len(out.Results) != 1 || out.Results[0].Title != "Wikipedia" {
		t.Errorf("unexpected results: %+v", out.Results)
	}

	// 2. Test error response propagation (e.g. 503 capacity exhausted)
	mockErrJSON := `{"error": {"code": 503, "message": "No capacity available for model gemini-2.5-flash", "status": "UNAVAILABLE"}}`
	errHttpResp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader([]byte(mockErrJSON))),
	}
	wErr := httptest.NewRecorder()
	srv.aggregateAntigravitySearchResponse(wErr, errHttpResp, "test query")
	if wErr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", wErr.Code)
	}
}

func TestHandleEmbeddingsProviderHintRouting(t *testing.T) {
	srv, database := setupTestServer(t)

	var receivedPath string
	var receivedBody map[string]any
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer mockUpstream.Close()

	// Setup an active openrouter connection pointing to mockUpstream with trailing /chat/completions
	conn := model.ProviderConnection{
		ID:       "test-openrouter-conn",
		Provider: "openrouter",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-test-key",
			BaseURL: mockUpstream.URL + "/chat/completions",
		},
	}
	database.CreateConnection(&conn)

	// Call handleEmbeddings with model="openai/text-embedding-3-small" and provider hint "openrouter"
	reqBody := `{"provider":"openrouter","model":"openai/text-embedding-3-small","input":"hello"}`
	req := httptest.NewRequest("POST", "/v1/embeddings", bytes.NewReader([]byte(reqBody)))
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if receivedPath != "/embeddings" {
		t.Errorf("expected trimmed path '/embeddings', got %q", receivedPath)
	}
	if receivedBody["model"] != "openai/text-embedding-3-small" {
		t.Errorf("expected preserved model 'openai/text-embedding-3-small', got %v", receivedBody["model"])
	}
	if _, leaked := receivedBody["provider"]; leaked {
		t.Errorf("provider hint leaked into upstream body")
	}
}
