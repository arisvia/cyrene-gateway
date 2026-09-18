package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestMessagesToOpenAIAdapter_NonStream(t *testing.T) {
	// Mock upstream OpenAI server
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != "mock-model" {
			t.Errorf("unexpected model: %v", req["model"])
		}

		resp := map[string]any{
			"id":     "chatcmpl-mock123",
			"object": "chat.completion",
			"model":  "openai/mock-model",
			"choices": []any{
				map[string]any{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "Hello through universal adapter!",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     15,
				"completion_tokens": 5,
				"total_tokens":      20,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	// Configure provider connection for openai
	database.CreateConnection(&model.ProviderConnection{
		ID:        "conn-mock-openai-1",
		Provider:  "openai",
		AuthType:  "api-key",
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Data: model.ConnectionData{
			APIKey:  "sk-mock",
			BaseURL: mockUpstream.URL + "/chat/completions",
		},
	})

	body := `{
		"model": "openai/mock-model",
		"system": "You are helpful",
		"messages": [
			{"role": "user", "content": "Hi"}
		],
		"max_tokens": 100
	}`

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var claudeResp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &claudeResp); err != nil {
		t.Fatalf("failed to unmarshal Anthropic response: %v", err)
	}

	if claudeResp["type"] != "message" {
		t.Errorf("expected type 'message', got %v", claudeResp["type"])
	}
	if claudeResp["role"] != "assistant" {
		t.Errorf("expected role 'assistant', got %v", claudeResp["role"])
	}
	if claudeResp["stop_reason"] != "end_turn" {
		t.Errorf("expected stop_reason 'end_turn', got %v", claudeResp["stop_reason"])
	}

	content, ok := claudeResp["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("missing content: %v", claudeResp)
	}
	block := content[0].(map[string]any)
	if block["text"] != "Hello through universal adapter!" {
		t.Errorf("unexpected content text: %v", block["text"])
	}
}

func TestMessagesToOpenAIAdapter_Stream(t *testing.T) {
	// Mock upstream OpenAI streaming server
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		w.Write([]byte("data: {\"id\":\"chatcmpl-stream\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Streamed \"},\"finish_reason\":null}]}\n\n"))
		flusher.Flush()
		w.Write([]byte("data: {\"id\":\"chatcmpl-stream\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Universal!\"},\"finish_reason\":\"stop\"}]}\n\n"))
		flusher.Flush()
		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	database.CreateConnection(&model.ProviderConnection{
		ID:        "conn-mock-openai-stream",
		Provider:  "openai",
		AuthType:  "api-key",
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Data: model.ConnectionData{
			APIKey:  "sk-mock",
			BaseURL: mockUpstream.URL + "/chat/completions",
		},
	})

	body := `{
		"model": "openai/mock-stream-model",
		"messages": [
			{"role": "user", "content": "Hi"}
		],
		"stream": true
	}`

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	res := w.Body.String()
	if !strings.Contains(res, "event: message_start") {
		t.Errorf("expected message_start event, got:\n%s", res)
	}
	if !strings.Contains(res, "event: content_block_start") {
		t.Errorf("expected content_block_start event, got:\n%s", res)
	}
	if !strings.Contains(res, "event: content_block_delta") || !strings.Contains(res, "Streamed ") || !strings.Contains(res, "Universal!") {
		t.Errorf("expected content_block_delta events with content, got:\n%s", res)
	}
	if !strings.Contains(res, "event: message_stop") {
		t.Errorf("expected message_stop event, got:\n%s", res)
	}
}

func TestMessagesToCombo_Stream(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		w.Write([]byte("data: {\"id\":\"chatcmpl-combo\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Combo Stream!\"},\"finish_reason\":\"stop\"}]}\n\n"))
		flusher.Flush()
		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	database.CreateConnection(&model.ProviderConnection{
		ID:        "conn-combo-sub",
		Provider:  "openai",
		AuthType:  "api-key",
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Data: model.ConnectionData{
			APIKey:  "sk-mock",
			BaseURL: mockUpstream.URL + "/chat/completions",
		},
	})

	// Create a combo named "my-combo"
	database.CreateCombo(&model.Combo{
		ID:        "combo-1",
		Name:      "my-combo",
		Models:    []string{"openai/sub-model"},
		Kind:      "fallback",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	body := `{
		"model": "my-combo",
		"messages": [
			{"role": "user", "content": "Hi"}
		],
		"stream": true
	}`

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	res := w.Body.String()
	if !strings.Contains(res, "event: message_start") || !strings.Contains(res, "Combo Stream!") || !strings.Contains(res, "event: message_stop") {
		t.Fatalf("unexpected combo stream response: %s", res)
	}
}
