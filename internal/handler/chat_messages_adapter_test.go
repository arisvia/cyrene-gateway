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

func TestAnthropicAdapter_ForwardsSSEKeepaliveComment(t *testing.T) {
	rec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	adapter := newAnthropicResponseAdapter(rec, true, "m")

	adapter.Write([]byte(": ping - queue waiting\n\n"))

	if !strings.Contains(rec.body.String(), ": ping - queue waiting") {
		t.Fatalf("keepalive comment must reach the client, got %q", rec.body.String())
	}
	if rec.flushes == 0 {
		t.Error("keepalive must be flushed immediately")
	}
}

func TestMessagesErrorsUseAnthropicShapeAndDirectivesWork(t *testing.T) {
	// 1. Error responses must use {"type":"error","error":{"type":...,"message":...}}
	srv, database := setupTestServer(t)
	for _, tc := range []struct {
		name     string
		body     string
		wantCode int
	}{
		{"invalid_json", "{not json", http.StatusBadRequest},
		{"missing_model", `{"messages":[{"role":"user","content":"hi"}]}`, http.StatusBadRequest},
		{"no_creds", `{"model":"anthropic/claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}]}`, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(tc.body))
			srv.Handler.ServeHTTP(w, req)
			if w.Code != tc.wantCode {
				t.Fatalf("code=%d want %d body=%s", w.Code, tc.wantCode, w.Body.String())
			}
			var errResp map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
				t.Fatal(err)
			}
			errObj, _ := errResp["error"].(map[string]any)
			if errResp["type"] != "error" || errObj == nil || errObj["message"] == nil || errObj["type"] == nil {
				t.Fatalf("expected Anthropic error envelope, got %s", w.Body.String())
			}
		})
	}

	// 2. Native Anthropic passthrough applies (high) / @cg-effort:high without leaking reasoning_effort
	var capturedAnthropic map[string]any
	anthUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedAnthropic)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2}}`))
	}))
	defer anthUpstream.Close()

	_ = database.CreateConnection(&model.ProviderConnection{
		ID: "conn-anth-msg-dir", Provider: "anthropic", AuthType: "api-key", IsActive: true,
		Data: model.ConnectionData{APIKey: "sk-ant", BaseURL: anthUpstream.URL + "/v1/messages"},
	})
	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{
		"model": "anthropic/claude-3-7-sonnet(high)",
		"max_tokens": 1000,
		"temperature": 0.7,
		"messages": [{"role":"user","content":"Analyze @cg-effort:low"}]
	}`))
	srv.Handler.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("native messages directive failed: %d %s", w1.Code, w1.Body.String())
	}
	if _, leaked := capturedAnthropic["reasoning_effort"]; leaked {
		t.Fatalf("reasoning_effort leaked onto native Anthropic passthrough: %#v", capturedAnthropic)
	}
	if capturedAnthropic["model"] != "claude-3-7-sonnet" {
		t.Fatalf("expected stripped model claude-3-7-sonnet, got %v", capturedAnthropic["model"])
	}
	thinking, _ := capturedAnthropic["thinking"].(map[string]any)
	if thinking == nil || thinking["type"] != "enabled" || thinking["budget_tokens"] != float64(2048) {
		t.Fatalf("expected @cg-effort:low to inject budget_tokens=2048, got %#v", capturedAnthropic["thinking"])
	}
	if _, hasTemp := capturedAnthropic["temperature"]; hasTemp {
		t.Fatalf("temperature must be stripped when thinking is injected")
	}

	// 3. Adapter path passes reasoning_effort from suffix to OpenAI upstream
	var capturedOAI map[string]any
	oaiUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedOAI)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`))
	}))
	defer oaiUpstream.Close()

	_ = database.CreateConnection(&model.ProviderConnection{
		ID: "conn-oai-msg-dir", Provider: "openai", AuthType: "api-key", IsActive: true,
		Data: model.ConnectionData{APIKey: "sk-oai", BaseURL: oaiUpstream.URL + "/chat/completions"},
	})
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{
		"model": "openai/gpt-5.4(high)",
		"max_tokens": 100,
		"messages": [{"role":"user","content":"Hello"}]
	}`))
	srv.Handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("adapter messages suffix failed: %d %s", w2.Code, w2.Body.String())
	}
	if capturedOAI["reasoning_effort"] != "high" {
		t.Fatalf("expected reasoning_effort=high on OpenAI adapter path, got %v", capturedOAI["reasoning_effort"])
	}
}
