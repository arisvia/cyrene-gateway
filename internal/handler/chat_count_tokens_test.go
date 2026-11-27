package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestHandleCountTokens_MissingModel(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest("POST", "/v1/messages/count_tokens", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing model, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCountTokens_InvalidJSON(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/v1/messages/count_tokens", bytes.NewReader([]byte(`invalid-json`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCountTokens_Success(t *testing.T) {
	srv, _ := setupTestServer(t)

	payload := map[string]any{
		"model":  "claude-3-7-sonnet-20250219",
		"system": "You are an expert Go programmer.",
		"messages": []any{
			map[string]any{"role": "user", "content": "How do I implement an HTTP server in Go?"},
		},
		"tools": []any{
			map[string]any{
				"name":        "read_file",
				"description": "Reads a local file from disk",
				"input_schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string"},
					},
					"required": []any{"path"},
				},
			},
		},
	}

	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/v1/messages/count_tokens", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.InputTokens < 20 || resp.InputTokens > 150 {
		t.Fatalf("expected input_tokens around ~50-80, got %d", resp.InputTokens)
	}
}

func TestHandleCountTokens_UpstreamForwardURLAndModel(t *testing.T) {
	var capturedPath string
	var capturedModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		capturedModel, _ = body["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"input_tokens":321}`))
	}))
	defer upstream.Close()

	srv, database := setupTestServer(t)
	if err := database.CreateConnection(&model.ProviderConnection{
		ID:       "anth-count-conn",
		Provider: "anthropic",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-ant-test",
			BaseURL: upstream.URL + "/v1/messages",
		},
	}); err != nil {
		t.Fatal(err)
	}

	payload := map[string]any{
		"model": "anthropic/claude-sonnet-4",
		"messages": []any{
			map[string]any{"role": "user", "content": "count me"},
		},
	}
	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/v1/messages/count_tokens", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
	if capturedPath != "/v1/messages/count_tokens" {
		t.Fatalf("expected upstream path /v1/messages/count_tokens, got %q", capturedPath)
	}
	if capturedModel != "claude-sonnet-4" {
		t.Fatalf("expected stripped bare model claude-sonnet-4, got %q", capturedModel)
	}
	var resp struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if resp.InputTokens != 321 {
		t.Fatalf("expected upstream input_tokens=321, got %d", resp.InputTokens)
	}
}
