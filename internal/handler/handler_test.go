package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/config"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

func setupTestServer(t *testing.T) (*Server, *db.DB) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	cfg := &config.Config{
		Host:                 "127.0.0.1",
		Port:                 0,
		DBPath:               ":memory:",
		DataDir:              t.TempDir(),
		AllowPrivateNetworks: true,
	}
	srv := NewServer(database, cfg)
	return srv, database
}

func TestHealthEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Fatalf("expected ok=true, got %v", resp["ok"])
	}
	if resp["service"] != "cyrene-gateway" {
		t.Fatalf("expected service=cyrene-gateway, got %v", resp["service"])
	}
}

func TestModelsEndpoint(t *testing.T) {
	srv, database := setupTestServer(t)

	// Create a connection
	conn := &model.ProviderConnection{
		ID:       "test-conn",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data:     model.ConnectionData{APIKey: "sk-test"},
	}
	database.CreateConnection(conn)

	req := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["object"] != "list" {
		t.Fatalf("expected object=list, got %v", resp["object"])
	}
}

func TestChatCompletionsMissingModel(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestChatCompletionsNoCredentials(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"model":"openai/gpt-4","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestChatCompletionsWithMockUpstream(t *testing.T) {
	// Create mock upstream
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)

		resp := map[string]any{
			"id":     "chatcmpl-test",
			"object": "chat.completion",
			"model":  reqBody["model"],
			"choices": []any{
				map[string]any{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "Hello! How can I help you?",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 8,
				"total_tokens":      18,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	// Create a connection pointing to mock upstream
	conn := &model.ProviderConnection{
		ID:       "test-conn",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-test",
			BaseURL: mockUpstream.URL,
		},
	}
	database.CreateConnection(conn)

	body := `{"model":"openai/gpt-4","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	choices, ok := resp["choices"].([]any)
	if !ok || len(choices) == 0 {
		t.Fatalf("expected choices, got %v", resp)
	}

	choice := choices[0].(map[string]any)
	message := choice["message"].(map[string]any)
	if message["content"] != "Hello! How can I help you?" {
		t.Fatalf("unexpected content: %v", message["content"])
	}
}

func TestChatCompletionsStreaming(t *testing.T) {
	// Create mock upstream that streams SSE
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher := w.(http.Flusher)
		chunks := []string{
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		}
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	conn := &model.ProviderConnection{
		ID:       "test-conn",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-test",
			BaseURL: mockUpstream.URL,
		},
	}
	database.CreateConnection(conn)

	body := `{"model":"openai/gpt-4","messages":[{"role":"user","content":"hello"}],"stream":true}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	responseBody := w.Body.String()
	if !strings.Contains(responseBody, "data: [DONE]") {
		t.Fatalf("expected [DONE] in stream, got: %s", responseBody)
	}
	if !strings.Contains(responseBody, "Hello") {
		t.Fatalf("expected 'Hello' in stream, got: %s", responseBody)
	}
}

func TestMessagesEndpoint(t *testing.T) {
	// Create mock Anthropic upstream
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") == "" {
			t.Error("expected x-api-key header")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Error("expected anthropic-version header")
		}

		resp := map[string]any{
			"id":   "msg_test",
			"type": "message",
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "text", "text": "Hello from Claude!"},
			},
			"model":       "claude-sonnet-4-20250514",
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  10,
				"output_tokens": 5,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	conn := &model.ProviderConnection{
		ID:       "test-conn",
		Provider: "anthropic",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-ant-test",
			BaseURL: mockUpstream.URL,
		},
	}
	database.CreateConnection(conn)

	body := `{"model":"anthropic/claude-sonnet-4-20250514","messages":[{"role":"user","content":"hello"}],"max_tokens":1024}`
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["id"] != "msg_test" {
		t.Fatalf("expected id=msg_test, got %v", resp["id"])
	}
}

func TestMessagesOutputConfigFormatStrippedForNonAnthropic(t *testing.T) {
	var receivedBody map[string]any
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		resp := map[string]any{
			"id":   "msg_test_compat",
			"type": "message",
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "text", "text": "Hello from MiniMax!"},
			},
			"model":       "MiniMax-Text-01",
			"stop_reason": "end_turn",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)
	conn := &model.ProviderConnection{
		Provider: "minimax",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-minimax-test",
			BaseURL: mockUpstream.URL,
		},
	}
	database.CreateConnection(conn)

	// Request with output_config containing both format (JSON schema) and effort (thinking)
	body := `{
		"model":"minimax/MiniMax-Text-01",
		"messages":[{"role":"user","content":"hello"}],
		"output_config":{
			"format":{"type":"json_schema","schema":{"type":"object"}},
			"effort":"high"
		},
		"max_tokens":1024
	}`
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	oc, ok := receivedBody["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("expected output_config to be preserved, got %+v", receivedBody)
	}
	if _, hasFormat := oc["format"]; hasFormat {
		t.Errorf("expected format to be stripped for non-Anthropic provider, but it was present: %+v", oc)
	}
	if oc["effort"] != "high" {
		t.Errorf("expected effort='high' to be retained, got %v", oc["effort"])
	}
}

func TestMessagesOutputConfigFormatRetainedForOfficialClaude(t *testing.T) {
	var receivedBody map[string]any
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		resp := map[string]any{
			"id":   "msg_test_official",
			"type": "message",
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "text", "text": "Hello from official Claude!"},
			},
			"model":       "claude-sonnet-4-20250514",
			"stop_reason": "end_turn",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)
	conn := &model.ProviderConnection{
		Provider: "claude",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-ant-test",
			BaseURL: mockUpstream.URL,
		},
	}
	database.CreateConnection(conn)

	body := `{
		"model":"claude/claude-sonnet-4-20250514",
		"messages":[{"role":"user","content":"hello"}],
		"output_config":{
			"format":{"type":"json_schema","schema":{"type":"object"}},
			"effort":"high"
		},
		"max_tokens":1024
	}`
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	oc, ok := receivedBody["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("expected output_config in official Claude request, got %+v", receivedBody)
	}
	if _, hasFormat := oc["format"]; !hasFormat {
		t.Errorf("expected format to be retained for official Claude provider, but it was stripped: %+v", oc)
	}
	if oc["effort"] != "high" {
		t.Errorf("expected effort='high', got %v", oc["effort"])
	}
}

func TestProxyNonStreamingUnwrapsSuccessEnvelope(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return Cline-style wrapped envelope
		wrapped := map[string]any{
			"success": true,
			"data": map[string]any{
				"id":      "chatcmpl-cline-123",
				"object":  "chat.completion",
				"created": 1234567890,
				"model":   "gpt-4o",
				"choices": []any{
					map[string]any{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "Unwrapped hello!",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     15,
					"completion_tokens": 10,
					"total_tokens":      25,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(wrapped)
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)
	conn := &model.ProviderConnection{
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-openai-test",
			BaseURL: mockUpstream.URL,
		},
	}
	database.CreateConnection(conn)

	body := `{"model":"openai/gpt-4o","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["id"] != "chatcmpl-cline-123" {
		t.Errorf("expected unwrapped id chatcmpl-cline-123, got %v", resp["id"])
	}
	choices, ok := resp["choices"].([]any)
	if !ok || len(choices) == 0 {
		t.Fatalf("expected choices array, got %v", resp["choices"])
	}
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "Unwrapped hello!" {
		t.Errorf("expected 'Unwrapped hello!', got %v", msg["content"])
	}
	usageObj, ok := resp["usage"].(map[string]any)
	if !ok {
		t.Fatalf("expected usage object in unwrapped response, got %+v", resp)
	}
	if promptTokens, _ := usageObj["prompt_tokens"].(float64); promptTokens != 15 {
		t.Errorf("expected prompt_tokens=15 extracted before translation, got %v", promptTokens)
	}
}

func TestCORSHeaders(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("expected CORS header Access-Control-Allow-Origin: *")
	}
}

func TestOptionsPreflight(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("OPTIONS", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS, got %d", w.Code)
	}
}

func TestDisabledModel(t *testing.T) {
	srv, database := setupTestServer(t)

	// Disable a model
	database.KVSet("disabledModels", "openai/gpt-4", "true")

	body := `{"model":"openai/gpt-4","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disabled model, got %d", w.Code)
	}
}

func TestDisabledModelExcludedFromV1Models(t *testing.T) {
	srv, database := setupTestServer(t)

	conn := &model.ProviderConnection{
		ID:       "test-openai-conn",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data:     model.ConnectionData{APIKey: "sk-test"},
	}
	database.CreateConnection(conn)

	// Disable a model
	database.KVSet("disabledModels", "openai/gpt-4o", "true")

	req := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	for _, m := range resp.Data {
		if m.ID == "openai/gpt-4o" {
			t.Fatalf("disabled model openai/gpt-4o should NOT be present in /v1/models")
		}
		if strings.HasSuffix(m.ID, "/*") {
			t.Fatalf("wildcard model %s should NEVER appear in /v1/models", m.ID)
		}
	}
}

func TestSetDisabledModelsAPI(t *testing.T) {
	srv, database := setupTestServer(t)

	// 1. Batch disable via models array
	body := `{"models":["openai/gpt-4o","openai/gpt-4o-mini"],"disabled":true}`
	req := httptest.NewRequest("POST", "/api/models/disabled", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for batch disable, got %d", w.Code)
	}

	list, err := database.KVList("disabledModels")
	if err != nil {
		t.Fatalf("KVList failed: %v", err)
	}
	if list["openai/gpt-4o"] != "true" || list["openai/gpt-4o-mini"] != "true" {
		t.Fatalf("expected both models disabled, got %+v", list)
	}

	// 2. Single enable via model string fallback
	body = `{"model":"openai/gpt-4o","disabled":false}`
	req = httptest.NewRequest("POST", "/api/models/disabled", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for single enable, got %d", w.Code)
	}

	list, err = database.KVList("disabledModels")
	if err != nil {
		t.Fatalf("KVList failed: %v", err)
	}
	if _, ok := list["openai/gpt-4o"]; ok {
		t.Fatalf("openai/gpt-4o should be enabled, but still in disabledModels")
	}
	if list["openai/gpt-4o-mini"] != "true" {
		t.Fatalf("openai/gpt-4o-mini should still be disabled")
	}

	// 3. Empty payload
	body = `{"models":[],"disabled":true}`
	req = httptest.NewRequest("POST", "/api/models/disabled", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty slice, got %d", w.Code)
	}
}

func TestOpenCodeHeadersInjection(t *testing.T) {
	var capturedHeaders http.Header
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","choices":[{"message":{"role":"assistant","content":"hello"}}]}`))
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	conn := &model.ProviderConnection{
		ID:       "test-opencode-conn",
		Provider: "opencode",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "oc-secret-key",
			BaseURL: mockUpstream.URL,
		},
	}
	database.CreateConnection(conn)

	// Free model should route with Bearer public and canonical headers
	body := `{"model":"opencode/big-pickle","messages":[{"role":"user","content":"hi"}]}`
	reqChat := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	reqChat.Header.Set("Content-Type", "application/json")
	wChat := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wChat, reqChat)

	if wChat.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", wChat.Code, wChat.Body.String())
	}

	if got := capturedHeaders.Get("Authorization"); got != "Bearer public" {
		t.Errorf("expected Bearer public for free model, got %q", got)
	}
	if got := capturedHeaders.Get("User-Agent"); got != "opencode/1.18.31" {
		t.Errorf("expected User-Agent opencode/1.18.31, got %q", got)
	}
	if got := capturedHeaders.Get("x-opencode-client"); got != "desktop" {
		t.Errorf("expected x-opencode-client desktop, got %q", got)
	}
	if got := capturedHeaders.Get("x-opencode-session"); !strings.HasPrefix(got, "ses_") || len(got) != 30 {
		t.Errorf("expected x-opencode-session 30-char ses_..., got %q", got)
	}
	if got := capturedHeaders.Get("x-opencode-request"); !strings.HasPrefix(got, "msg_") || len(got) != 30 {
		t.Errorf("expected x-opencode-request 30-char msg_..., got %q", got)
	}
	if got := capturedHeaders.Get("x-opencode-project"); got != "global" {
		t.Errorf("expected x-opencode-project global, got %q", got)
	}

	// Paid model should route with the configured API key
	bodyPaid := `{"model":"opencode/deepseek-v4-pro","messages":[{"role":"user","content":"hi"}]}`
	reqPaid := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(bodyPaid))
	reqPaid.Header.Set("Content-Type", "application/json")
	wPaid := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wPaid, reqPaid)
	if wPaid.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for paid model, got %d: %s", wPaid.Code, wPaid.Body.String())
	}
	if got := capturedHeaders.Get("Authorization"); got != "Bearer oc-secret-key" {
		t.Errorf("expected Bearer oc-secret-key for paid model, got %q", got)
	}
}
func TestOpenCodeTestConnection(t *testing.T) {
	srv, _ := setupTestServer(t)

	// 1. Missing API key should be rejected immediately without calling upstream
	reqNoKey := httptest.NewRequest("POST", "/api/providers/test-credentials", strings.NewReader(`{"provider":"opencode","apiKey":""}`))
	reqNoKey.Header.Set("Content-Type", "application/json")
	wNoKey := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wNoKey, reqNoKey)
	if wNoKey.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 wrapper, got %d", wNoKey.Code)
	}
	var resNoKey map[string]any
	json.Unmarshal(wNoKey.Body.Bytes(), &resNoKey)
	if resNoKey["ok"] == true {
		t.Fatalf("expected ok: false for missing OpenCode key, got ok: true")
	}

	// 2. Upstream 401 should fail the connection test
	mock401 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"type":"error","error":{"type":"AuthError","message":"Invalid API key."}}`))
	}))
	defer mock401.Close()

	req401 := httptest.NewRequest("POST", "/api/providers/test-credentials", strings.NewReader(fmt.Sprintf(`{"provider":"opencode","apiKey":"invalid-key","baseUrl":%q}`, mock401.URL)))
	req401.Header.Set("Content-Type", "application/json")
	w401 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w401, req401)
	var res401 map[string]any
	json.Unmarshal(w401.Body.Bytes(), &res401)
	if res401["ok"] == true {
		t.Fatalf("expected ok: false for 401 response, got ok: true")
	}
	if res401["error"] != "Invalid API key." {
		t.Errorf("expected error 'Invalid API key.', got %v", res401["error"])
	}

	// 3. Upstream 200 should pass the connection test
	mock200 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer mock200.Close()

	req200 := httptest.NewRequest("POST", "/api/providers/test-credentials", strings.NewReader(fmt.Sprintf(`{"provider":"opencode-go","apiKey":"valid-go-key","baseUrl":%q}`, mock200.URL)))
	req200.Header.Set("Content-Type", "application/json")
	w200 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w200, req200)
	var res200 map[string]any
	json.Unmarshal(w200.Body.Bytes(), &res200)
	if res200["ok"] != true {
		t.Fatalf("expected ok: true for 200 response, got: %v", res200)
	}

	// 4. Upstream 429 (quota exhausted) should still prove auth passed
	mock429 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded"}}`))
	}))
	defer mock429.Close()

	req429 := httptest.NewRequest("POST", "/api/providers/test-credentials", strings.NewReader(fmt.Sprintf(`{"provider":"opencode","apiKey":"valid-zen-key","baseUrl":%q}`, mock429.URL)))
	req429.Header.Set("Content-Type", "application/json")
	w429 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w429, req429)
	var res429 map[string]any
	json.Unmarshal(w429.Body.Bytes(), &res429)
	if res429["ok"] != true {
		t.Fatalf("expected ok: true for 429 response, got: %v", res429)
	}
}

func TestDashboardServesHTML(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html content type, got %s", ct)
	}
	if !strings.Contains(w.Body.String(), "Cyrene") {
		t.Fatal("expected dashboard HTML content")
	}
}

func TestDashboardAssetMissIs404NotSPA(t *testing.T) {
	srv, _ := setupTestServer(t)

	// A hashed asset path that does not exist in the embedded dist
	// must return 404 — never the SPA index.html fallback, which
	// previously made a missing JS bundle indistinguishable from a
	// real one (browser got text/html with 200 and rendered a blank page).
	req := httptest.NewRequest("GET", "/assets/index-DOESNOTEXIST.js", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing asset, got %d with content-type %s", w.Code, w.Header().Get("Content-Type"))
	}
	if ct := w.Header().Get("Content-Type"); strings.Contains(ct, "text/html") {
		t.Fatalf("expected non-HTML error response for missing asset, got %s", ct)
	}
}

func TestAuthStatusNoLoginRequired(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/auth/status", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["requireLogin"] != false {
		t.Fatalf("expected requireLogin=false, got %v", resp["requireLogin"])
	}
	if resp["authenticated"] != true {
		t.Fatalf("expected authenticated=true when login not required, got %v", resp["authenticated"])
	}
}

func TestLoginWithDefaultPassword(t *testing.T) {
	srv, database := setupTestServer(t)

	// Set a password first (no weak default 123456)
	settings, _ := database.GetSettings()
	settings.PasswordHash = auth.HashPassword("secure-password-123")
	database.SaveSettings(settings)

	body := `{"password":"secure-password-123"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Check that auth_token cookie is set
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "auth_token" && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected auth_token cookie to be set")
	}
}

func TestLoginWithWrongPassword(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"password":"wrongpass"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestLogout(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Check that auth_token cookie is cleared
	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "auth_token" && c.MaxAge != -1 {
			t.Fatal("expected auth_token cookie to be cleared")
		}
	}
}

func TestAPIKeyAuthEnforcement(t *testing.T) {
	srv, database := setupTestServer(t)

	// Enable requireApiKey
	settings, _ := database.GetSettings()
	settings.RequireAPIKey = true
	database.SaveSettings(settings)

	// Request without API key should be rejected
	body := `{"model":"openai/gpt-4","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without API key, got %d", w.Code)
	}
}

func TestAPIKeyAuthWithValidKey(t *testing.T) {
	srv, database := setupTestServer(t)

	// Enable requireApiKey
	settings, _ := database.GetSettings()
	settings.RequireAPIKey = true
	database.SaveSettings(settings)

	// Create a valid signed API key
	apiKeyStr := auth.GenerateAPIKey()
	key := &model.APIKey{
		ID:       "test-key-id",
		Key:      apiKeyStr,
		Name:     "test",
		IsActive: true,
	}
	database.CreateAPIKey(key)

	// Request with valid API key should pass through (will get 503 for no credentials, not 401)
	body := `{"model":"openai/gpt-4","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKeyStr)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("expected non-401 with valid API key, got %d", w.Code)
	}
}

func TestDashboardAuthEnforcement(t *testing.T) {
	srv, database := setupTestServer(t)

	// Enable requireLogin
	settings, _ := database.GetSettings()
	settings.RequireLogin = true
	database.SaveSettings(settings)

	// Request to protected API without session should be rejected
	req := httptest.NewRequest("GET", "/api/providers", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", w.Code)
	}

	// Public paths should still work
	req = httptest.NewRequest("GET", "/api/health", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for public path, got %d", w.Code)
	}
}

func TestSettingsPasswordAndRequireLoginGuard(t *testing.T) {
	srv, database := setupTestServer(t)

	// 1. Initial state: hasPassword should be false
	req := httptest.NewRequest("GET", "/api/settings", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var sResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &sResp)
	if sResp["hasPassword"] != false {
		t.Fatalf("expected hasPassword=false, got %v", sResp["hasPassword"])
	}

	// 2. Attempt to enable requireLogin via PUT without password: must be rejected with 400
	body := `{"requireLogin":true}`
	req = httptest.NewRequest("PUT", "/api/settings", strings.NewReader(body))
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when enabling requireLogin without password, got %d", w.Code)
	}

	// 3. Attempt to enable requireLogin via PATCH without password: must be rejected with 400
	req = httptest.NewRequest("PATCH", "/api/settings", strings.NewReader(body))
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when enabling requireLogin via PATCH without password, got %d", w.Code)
	}

	// 4. Set password via /api/auth/password
	pwBody := `{"password":"admin-password-123"}`
	req = httptest.NewRequest("POST", "/api/auth/password", strings.NewReader(pwBody))
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 setting password, got %d", w.Code)
	}

	// Verify hasPassword is now true
	req = httptest.NewRequest("GET", "/api/settings", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	json.Unmarshal(w.Body.Bytes(), &sResp)
	if sResp["hasPassword"] != true {
		t.Fatalf("expected hasPassword=true, got %v", sResp["hasPassword"])
	}

	// 5. Now enabling requireLogin via PATCH should succeed
	req = httptest.NewRequest("PATCH", "/api/settings", strings.NewReader(body))
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 enabling requireLogin after password set, got %d", w.Code)
	}
	loginBody := `{"password":"admin-password-123"}`
	req = httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(loginBody))
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from login, got %d", w.Code)
	}
	cookie := w.Result().Cookies()[0]

	// 7. Verify that saving settings with passwordHash="" does NOT wipe existing password
	patchEmptyPw := `{"apiKeyRpm":10,"passwordHash":""}`
	req = httptest.NewRequest("PATCH", "/api/settings", strings.NewReader(patchEmptyPw))
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from patch, got %d", w.Code)
	}
	st, _ := database.GetSettings()
	if st.PasswordHash == "" {
		t.Fatal("passwordHash was wiped out by patch!")
	}
}

func TestCreateAPIKeyEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"name":"my-key"}`
	req := httptest.NewRequest("POST", "/api/keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	key, ok := resp["key"].(string)
	if !ok || !strings.HasPrefix(key, "cg-") {
		t.Fatalf("expected cg- prefixed key, got %v", resp["key"])
	}
	if !strings.Contains(key, ".") {
		t.Fatalf("expected HMAC-signed key with dot separator, got %s", key)
	}
}

func TestHealthEndpointEnhanced(t *testing.T) {
	srv, database := setupTestServer(t)

	// Create a connection to verify connection count
	conn := &model.ProviderConnection{
		ID:       "health-test-conn",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data:     model.ConnectionData{APIKey: "sk-test"},
	}
	database.CreateConnection(conn)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["db"] != "ok" {
		t.Fatalf("expected db=ok, got %v", resp["db"])
	}
	if _, ok := resp["uptimeSeconds"]; !ok {
		t.Fatal("expected uptimeSeconds field in health response")
	}
	if resp["connections"] != float64(1) {
		t.Fatalf("expected connections=1, got %v", resp["connections"])
	}
	if resp["activeConnections"] != float64(1) {
		t.Fatalf("expected activeConnections=1, got %v", resp["activeConnections"])
	}
}

func TestProxyPoolCRUD(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Create
	body := `{"name":"test-proxy","proxyUrl":"http://127.0.0.1:7890","type":"http","strictProxy":true}`
	req := httptest.NewRequest("POST", "/api/proxy-pools", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var createResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &createResp)
	pool := createResp["proxyPool"].(map[string]any)
	poolID := pool["id"].(string)
	if poolID == "" {
		t.Fatal("expected non-empty pool id")
	}
	poolData := pool["data"].(map[string]any)
	if poolData["name"] != "test-proxy" {
		t.Fatalf("expected name=test-proxy, got %v", poolData["name"])
	}
	if poolData["proxyUrl"] != "http://127.0.0.1:7890" {
		t.Fatalf("expected proxyUrl, got %v", poolData["proxyUrl"])
	}

	// List
	req = httptest.NewRequest("GET", "/api/proxy-pools", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var listResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &listResp)
	pools := listResp["proxyPools"].([]any)
	if len(pools) != 1 {
		t.Fatalf("expected 1 pool, got %d", len(pools))
	}

	// Get by ID
	req = httptest.NewRequest("GET", "/api/proxy-pools/"+poolID, nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Update
	body = `{"name":"updated-proxy","isActive":false}`
	req = httptest.NewRequest("PUT", "/api/proxy-pools/"+poolID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var updateResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &updateResp)
	updatedPool := updateResp["proxyPool"].(map[string]any)
	updatedData := updatedPool["data"].(map[string]any)
	if updatedData["name"] != "updated-proxy" {
		t.Fatalf("expected name=updated-proxy, got %v", updatedData["name"])
	}
	if updatedPool["isActive"] != false {
		t.Fatalf("expected isActive=false, got %v", updatedPool["isActive"])
	}

	// Delete
	req = httptest.NewRequest("DELETE", "/api/proxy-pools/"+poolID, nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify deleted
	req = httptest.NewRequest("GET", "/api/proxy-pools/"+poolID, nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", w.Code)
	}
}

func TestProxyPoolCreateValidation(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Missing name
	body := `{"proxyUrl":"http://127.0.0.1:7890"}`
	req := httptest.NewRequest("POST", "/api/proxy-pools", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing name, got %d", w.Code)
	}

	// Missing proxyUrl
	body = `{"name":"test"}`
	req = httptest.NewRequest("POST", "/api/proxy-pools", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing proxyUrl, got %d", w.Code)
	}
}

func TestProxyPoolDeleteConflict(t *testing.T) {
	srv, database := setupTestServer(t)

	// Create a proxy pool
	body := `{"name":"bound-proxy","proxyUrl":"http://127.0.0.1:7890"}`
	req := httptest.NewRequest("POST", "/api/proxy-pools", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	var createResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &createResp)
	pool := createResp["proxyPool"].(map[string]any)
	poolID := pool["id"].(string)

	// Create a connection bound to this pool
	conn := &model.ProviderConnection{
		ID:       "bound-conn",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:               "sk-test",
			ProviderSpecificData: map[string]any{"proxyPoolId": poolID},
		},
	}
	database.CreateConnection(conn)

	// Attempt delete should return 409
	req = httptest.NewRequest("DELETE", "/api/proxy-pools/"+poolID, nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for in-use pool, got %d: %s", w.Code, w.Body.String())
	}
}
