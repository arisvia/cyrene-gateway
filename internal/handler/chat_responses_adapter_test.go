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
	if !strings.Contains(res, "event: response.completed") {
		t.Errorf("expected response.completed event, got:\n%s", res)
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
	if !strings.Contains(res, "event: response.created") || !strings.Contains(res, "Combo Responses Stream!") || !strings.Contains(res, "event: response.completed") {
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
	if !strings.Contains(res, "event: response.completed") {
		t.Errorf("expected response.completed, got:\n%s", res)
	}
}

func TestResponsesAdapterTerminalStatus(t *testing.T) {
	for _, tc := range []struct {
		name, chunk, status, detail string
	}{
		{"completed", `{"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`, "completed", ""},
		{"tools", `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, "completed", "call_1"},
		{"length", `{"choices":[{"delta":{"content":"partial"},"finish_reason":"length"}]}`, "incomplete", "max_output_tokens"},
		{"filtered", `{"choices":[{"delta":{"content":"partial"},"finish_reason":"content_filter"}]}`, "incomplete", "content_filter"},
		{"failed", `{"error":{"type":"upstream_error","code":"server_error","message":"boom","param":"input","extra":"preserved"}}`, "failed", "boom"},
	} {
		for _, unterminated := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/done", true: "/eof"}[unterminated], func(t *testing.T) {
				w := httptest.NewRecorder()
				a := newResponsesResponseAdapter(w, true, "audit")
				payload := "data: " + tc.chunk
				if !unterminated {
					payload += "\r\n\r\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2,\"total_tokens\":7}}\n\ndata: [DONE]\n\n"
				}
				for _, part := range []string{payload[:9], payload[9:]} {
					if _, err := a.Write([]byte(part)); err != nil {
						t.Fatal(err)
					}
				}
				if !unterminated && tc.status == "failed" && !strings.Contains(w.Body.String(), "event: response.failed\n") {
					t.Fatal("upstream error was not emitted immediately")
				}
				a.Finish()
				result := w.Body.String()
				if strings.Count(result, "event: response."+tc.status+"\n") != 1 || !strings.Contains(result, `"status":"`+tc.status+`"`) {
					t.Fatalf("incorrect terminal event/status: %s", result)
				}
				if tc.status != "completed" && strings.Contains(result, "event: response.completed\n") {
					t.Fatalf("failure/truncation became success: %s", result)
				}
				if tc.detail != "" && !strings.Contains(result, tc.detail) {
					t.Fatalf("terminal detail lost: %s", result)
				}
				if tc.status == "failed" && (!strings.Contains(result, `"error":`) || !strings.Contains(result, `"extra":"preserved"`)) {
					t.Fatalf("upstream error fields lost: %s", result)
				}
				if !unterminated && tc.status != "failed" && !strings.Contains(result, `"total_tokens":7`) {
					t.Fatalf("trailing usage lost: %s", result)
				}
				a.Write([]byte("data: " + tc.chunk + "\n\n"))
				a.Finish()
				if w.Body.String() != result {
					t.Fatal("adapter emitted data after terminal event")
				}
			})
		}
	}
}

func TestResponsesAdapterNonStreamStatus(t *testing.T) {
	for _, tc := range []struct{ name, body, status, detail string }{
		{"length", `{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`, "incomplete", "max_output_tokens"},
		{"filter", `{"choices":[{"message":{"content":"partial"},"finish_reason":"content_filter"}]}`, "incomplete", "content_filter"},
		{"error", `{"error":{"code":"server_error","message":"boom","extra":"preserved"}}`, "failed", "preserved"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			a := newResponsesResponseAdapter(w, false, "audit")
			a.Write([]byte(tc.body))
			a.Finish()
			var result map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result["status"] != tc.status || !strings.Contains(w.Body.String(), tc.detail) {
				t.Fatalf("incorrect non-stream status/detail: %s", w.Body.String())
			}
			before := w.Body.String()
			a.Finish()
			if before != w.Body.String() {
				t.Fatal("non-stream response emitted twice")
			}
		})
	}
	for _, stream := range []bool{false, true} {
		w := httptest.NewRecorder()
		a := newResponsesResponseAdapter(w, stream, "audit")
		a.WriteHeader(http.StatusBadGateway)
		body := `{"error":{"code":"server_error","message":"boom"}}`
		a.Write([]byte(body))
		a.Finish()
		if w.Code != http.StatusBadGateway || w.Body.String() != body {
			t.Fatalf("HTTP failure was changed: %d %s", w.Code, w.Body.String())
		}
	}
}
