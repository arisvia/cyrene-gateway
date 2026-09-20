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

func TestMessagesToOpenAIAdapter_Stream_ToolCalls(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		// Chunk 1: Text message start
		w.Write([]byte("data: {\"id\":\"chatcmpl-tc-stream\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Calculating...\"},\"finish_reason\":null}]}\n\n"))
		flusher.Flush()

		// Chunk 2: Tool call initiation
		w.Write([]byte("data: {\"id\":\"chatcmpl-tc-stream\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_calc_42\",\"type\":\"function\",\"function\":{\"name\":\"calc\",\"arguments\":\"\"}}]},\"finish_reason\":null}]}\n\n"))
		flusher.Flush()

		// Chunk 3: Tool call arguments delta
		w.Write([]byte("data: {\"id\":\"chatcmpl-tc-stream\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"num\\\": 42}\"}}]},\"finish_reason\":null}]}\n\n"))
		flusher.Flush()

		// Chunk 4: Finish reason
		w.Write([]byte("data: {\"id\":\"chatcmpl-tc-stream\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"))
		flusher.Flush()

		// Chunk 5: [DONE]
		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	database.CreateConnection(&model.ProviderConnection{
		ID:        "conn-mock-tc-stream",
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
		"model": "openai/mock-tc-model",
		"messages": [
			{"role": "user", "content": "What is 6 times 7?"}
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
		t.Errorf("expected message_start, got:\n%s", res)
	}
	if !strings.Contains(res, "Calculating...") {
		t.Errorf("expected text 'Calculating...', got:\n%s", res)
	}
	if !strings.Contains(res, "event: content_block_start") || !strings.Contains(res, "tool_use") || !strings.Contains(res, "call_calc_42") || !strings.Contains(res, "calc") {
		t.Errorf("expected tool_use block start, got:\n%s", res)
	}
	if !strings.Contains(res, "event: content_block_delta") || !strings.Contains(res, "input_json_delta") || !strings.Contains(res, "42") {
		t.Errorf("expected input_json_delta, got:\n%s", res)
	}
	if !strings.Contains(res, "event: content_block_stop") {
		t.Errorf("expected content_block_stop, got:\n%s", res)
	}
	if !strings.Contains(res, `"stop_reason":"tool_use"`) {
		t.Errorf("expected stop_reason tool_use, got:\n%s", res)
	}
	if !strings.Contains(res, "event: message_stop") {
		t.Errorf("expected message_stop, got:\n%s", res)
	}
}
