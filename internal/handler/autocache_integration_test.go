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

func TestHandleMessages_AutoPromptCache_Integration(t *testing.T) {
	srv, database := setupTestServer(t)

	// Enable AutoPromptCache in settings
	st, _ := database.GetSettings()
	st.AutoPromptCache = true
	database.SaveSettings(st)

	capturedBody := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody <- body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"msg_123","type":"message","role":"assistant","content":[{"type":"text","text":"cached hello"}]}`))
	}))
	t.Cleanup(upstream.Close)

	// Configure mock Anthropic provider connection pointing to upstream
	database.CreateConnection(&model.ProviderConnection{
		ID:       "conn-anthropic",
		Provider: "anthropic",
		AuthType: "api-key",
		Name:     "Anthropic Test",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "test-api-key",
			BaseURL: upstream.URL,
		},
	})

	// Make request with 2 user turns
	reqBody := map[string]any{
		"model":  "claude-3-7-sonnet-20250219",
		"system": "System instructions here",
		"tools": []any{
			map[string]any{
				"name":         "bash",
				"description":  "run command",
				"input_schema": map[string]any{"type": "object"},
			},
		},
		"messages": []any{
			map[string]any{"role": "user", "content": "turn 1 user"},
			map[string]any{"role": "assistant", "content": "turn 1 assistant"},
			map[string]any{"role": "user", "content": "turn 2 user (latest)"},
		},
	}
	raw, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	select {
	case captured := <-capturedBody:
		var sent map[string]any
		if err := json.Unmarshal(captured, &sent); err != nil {
			t.Fatalf("failed to parse upstream body: %v", err)
		}
		// Tools should have cache_control on the last tool
		tools, ok := sent["tools"].([]any)
		if !ok || len(tools) == 0 {
			t.Fatalf("expected tools forwarded upstream")
		}
		tool0 := tools[0].(map[string]any)
		if tool0["cache_control"] == nil {
			t.Errorf("expected auto-injected cache_control on tool definition")
		}
	default:
		t.Fatal("timed out waiting for upstream request")
	}
}
