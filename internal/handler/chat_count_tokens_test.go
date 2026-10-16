package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
