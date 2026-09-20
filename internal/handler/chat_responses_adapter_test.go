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

func TestResponses_StringInput_NonStream(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != "mock-model" {
			t.Errorf("unexpected model: %v", req["model"])
		}

		msgs, ok := req["messages"].([]any)
		if !ok || len(msgs) != 2 {
			t.Errorf("expected 2 messages (developer + user), got: %v", msgs)
		}

		resp := map[string]any{
			"id":     "chatcmpl-responses-1",
			"object": "chat.completion",
			"model":  "openai/mock-model",
			"choices": []any{
				map[string]any{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "Responses API succeeded!",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 4,
				"total_tokens":      14,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	database.CreateConnection(&model.ProviderConnection{
		ID:        "conn-mock-openai-resp",
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
		"instructions": "Be concise",
		"input": "Say hello",
		"stream": false
	}`

	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal responses payload: %v", err)
	}

	if res["object"] != "response" {
		t.Errorf("expected object 'response', got %v", res["object"])
	}
	if res["status"] != "completed" {
		t.Errorf("expected status 'completed', got %v", res["status"])
	}
	if res["output_text"] != "Responses API succeeded!" {
		t.Errorf("expected output_text 'Responses API succeeded!', got %v", res["output_text"])
	}
}

func TestResponses_ArrayInput_Stream(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		w.Write([]byte("data: {\"id\":\"chatcmpl-resp-stream\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Streamed \"},\"finish_reason\":null}]}\n\n"))
		flusher.Flush()
		w.Write([]byte("data: {\"id\":\"chatcmpl-resp-stream\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Response!\"},\"finish_reason\":\"stop\"}]}\n\n"))
		flusher.Flush()
		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	database.CreateConnection(&model.ProviderConnection{
		ID:        "conn-mock-openai-resp-stream",
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
		"input": [
			{"role": "user", "content": "Hello"}
		],
		"stream": true
	}`

	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	res := w.Body.String()
	if !strings.Contains(res, "event: response.created") {
		t.Errorf("expected response.created event, got:\n%s", res)
	}
	if !strings.Contains(res, "event: response.output_item.added") {
		t.Errorf("expected response.output_item.added event, got:\n%s", res)
	}
	if !strings.Contains(res, "event: response.content_part.added") {
		t.Errorf("expected response.content_part.added event, got:\n%s", res)
	}
	if !strings.Contains(res, "event: response.output_text.delta") || !strings.Contains(res, "Streamed ") || !strings.Contains(res, "Response!") {
		t.Errorf("expected response.output_text.delta events, got:\n%s", res)
	}
	if !strings.Contains(res, "event: response.done") {
		t.Errorf("expected response.done event, got:\n%s", res)
	}
}

func TestResponses_Combo_Stream(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		w.Write([]byte("data: {\"id\":\"chatcmpl-combo-resp\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Combo Responses Stream!\"},\"finish_reason\":\"stop\"}]}\n\n"))
		flusher.Flush()
		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	database.CreateConnection(&model.ProviderConnection{
		ID:        "conn-combo-resp",
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

	database.CreateCombo(&model.Combo{
		ID:        "combo-resp-1",
		Name:      "responses-combo",
		Models:    []string{"openai/sub-model"},
		Kind:      "fallback",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	body := `{
		"model": "responses-combo",
		"input": "Stream this combo please",
		"stream": true
	}`

	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	res := w.Body.String()
	if !strings.Contains(res, "event: response.created") || !strings.Contains(res, "Combo Responses Stream!") || !strings.Contains(res, "event: response.done") {
		t.Fatalf("unexpected combo stream response: %s", res)
	}
}

func TestResponses_MultiTurn_ToolCalls_Stream(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var chatReq map[string]any
		json.NewDecoder(r.Body).Decode(&chatReq)

		// Verify the upstream received proper tool messages from Responses API input
		msgs, ok := chatReq["messages"].([]any)
		if !ok || len(msgs) != 2 {
			t.Errorf("expected 2 messages (assistant tool_call + tool output), got: %v", msgs)
		} else {
			m0 := msgs[0].(map[string]any)
			if m0["role"] != "assistant" || m0["tool_calls"] == nil {
				t.Errorf("expected assistant message with tool_calls for m0, got: %v", m0)
			}
			m1 := msgs[1].(map[string]any)
			if m1["role"] != "tool" || m1["tool_call_id"] != "call_prior" || m1["content"] != "42" {
				t.Errorf("expected tool message with tool_call_id for m1, got: %v", m1)
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		// Stream a new tool call
		w.Write([]byte("data: {\"id\":\"chatcmpl-turn2\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_next\",\"type\":\"function\",\"function\":{\"name\":\"finish_task\",\"arguments\":\"\"}}]},\"finish_reason\":null}]}\n\n"))
		flusher.Flush()
		w.Write([]byte("data: {\"id\":\"chatcmpl-turn2\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"status\\\":\\\"ok\\\"}\"}}]},\"finish_reason\":null}]}\n\n"))
		flusher.Flush()
		w.Write([]byte("data: {\"id\":\"chatcmpl-turn2\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"))
		flusher.Flush()
		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	database.CreateConnection(&model.ProviderConnection{
		ID:        "conn-resp-turn2",
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
		"model": "openai/mock-turn2",
		"input": [
			{"type": "function_call", "call_id": "call_prior", "name": "calc", "arguments": "{\"x\":42}"},
			{"type": "function_call_output", "call_id": "call_prior", "output": "42"}
		],
		"stream": true
	}`

	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	res := w.Body.String()
	if !strings.Contains(res, "event: response.created") {
		t.Errorf("expected response.created, got:\n%s", res)
	}
	if !strings.Contains(res, "event: response.output_item.added") || !strings.Contains(res, "finish_task") || !strings.Contains(res, "call_next") {
		t.Errorf("expected output_item.added with finish_task, got:\n%s", res)
	}
	if !strings.Contains(res, "event: response.function_call_arguments.delta") || !strings.Contains(res, "status") {
		t.Errorf("expected function_call_arguments.delta with status, got:\n%s", res)
	}
	if !strings.Contains(res, "event: response.function_call_arguments.done") {
		t.Errorf("expected function_call_arguments.done, got:\n%s", res)
	}
	if !strings.Contains(res, "event: response.output_item.done") {
		t.Errorf("expected output_item.done, got:\n%s", res)
	}
	if !strings.Contains(res, "event: response.done") {
		t.Errorf("expected response.done, got:\n%s", res)
	}
}
