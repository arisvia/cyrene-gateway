package handler

// E2E test suite (Phase 35): scripted chat completion for representative
// providers from every category, verifying the full request path through
// model resolution → transport → auth injection → upstream proxy → response.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// mockOpenAIUpstream returns a test server that validates auth headers and
// returns a standard OpenAI chat completion response.
func mockOpenAIUpstream(t *testing.T, wantAuth func(r *http.Request) error) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wantAuth != nil {
			if err := wantAuth(r); err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
		}
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)
		resp := map[string]any{
			"id":     "chatcmpl-e2e",
			"object": "chat.completion",
			"model":  reqBody["model"],
			"choices": []any{
				map[string]any{
					"index":         0,
					"message":       map[string]any{"role": "assistant", "content": "E2E OK"},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 3, "total_tokens": 8},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

// e2eCase defines a single E2E provider verification.
type e2eCase struct {
	WantAuth func(r *http.Request) error
	Provider string
	Model    string
	AuthType string
	APIKey   string
	Token    string
}

func TestE2EChatCompletion(t *testing.T) {
	cases := []e2eCase{
		// --- apikey category ---
		{
			Provider: "openai", Model: "openai/gpt-4o",
			AuthType: "api-key", APIKey: "sk-test-openai",
			WantAuth: func(r *http.Request) error {
				if got := r.Header.Get("Authorization"); got != "Bearer sk-test-openai" {
					return fmt.Errorf("openai: want Bearer sk-test-openai, got %q", got)
				}
				return nil
			},
		},
		{
			Provider: "deepseek", Model: "deepseek/deepseek-v4-pro",
			AuthType: "api-key", APIKey: "sk-ds-test",
			WantAuth: func(r *http.Request) error {
				if got := r.Header.Get("Authorization"); got != "Bearer sk-ds-test" {
					return fmt.Errorf("deepseek: want Bearer, got %q", got)
				}
				return nil
			},
		},
		{
			Provider: "openrouter", Model: "openrouter/anthropic/claude-sonnet-4-20250514",
			AuthType: "api-key", APIKey: "sk-or-test",
			WantAuth: func(r *http.Request) error {
				// conn.Data.BaseURL override skips registry transport headers (by design).
				// Registry headers (HTTP-Referer, X-Title) are tested in transport_test.go.
				if got := r.Header.Get("Authorization"); got != "Bearer sk-or-test" {
					return fmt.Errorf("openrouter: want Bearer, got %q", got)
				}
				return nil
			},
		},
		{
			Provider: "cerebras", Model: "cerebras/llama-3.3-70b",
			AuthType: "api-key", APIKey: "csk-test",
			WantAuth: func(r *http.Request) error {
				if got := r.Header.Get("Authorization"); got != "Bearer csk-test" {
					return fmt.Errorf("cerebras: want Bearer, got %q", got)
				}
				return nil
			},
		},
		{
			Provider: "groq", Model: "groq/llama-3.3-70b-versatile",
			AuthType: "api-key", APIKey: "gsk-test",
			WantAuth: func(r *http.Request) error {
				if got := r.Header.Get("Authorization"); got != "Bearer gsk-test" {
					return fmt.Errorf("groq: want Bearer, got %q", got)
				}
				return nil
			},
		},
		// --- freeTier category (apikey auth) ---
		{
			Provider: "nvidia", Model: "nvidia/deepseek-ai/deepseek-v4-pro",
			AuthType: "api-key", APIKey: "nvapi-test",
			WantAuth: func(r *http.Request) error {
				if got := r.Header.Get("Authorization"); got != "Bearer nvapi-test" {
					return fmt.Errorf("nvidia: want Bearer, got %q", got)
				}
				return nil
			},
		},
		// --- freeTier category (gemini format) ---
		// Gemini uses a different wire format; tested separately below.
		// --- free category (NoAuth, zero-config) ---
		{
			Provider: "opencode", Model: "opencode/big-pickle",
			AuthType: "none",
			WantAuth: func(r *http.Request) error {
				if got := r.Header.Get("Authorization"); got != "Bearer public" {
					return fmt.Errorf("opencode: want Bearer public, got %q", got)
				}
				if got := r.Header.Get("x-opencode-client"); got != "desktop" {
					return fmt.Errorf("opencode: want x-opencode-client=desktop, got %q", got)
				}
				return nil
			},
		},
		// --- apikey category (anthropic format) ---
		// Note: anthropic format translates the request body, so the mock must
		// return an anthropic-format response. Tested separately below.
		// --- freeTier (apikey, custom headers) ---
		// --- oauth category (access token) ---
		{
			Provider: "github", Model: "github/gpt-5.4",
			AuthType: "oauth", Token: "gho-test-token",
			WantAuth: func(r *http.Request) error {
				if got := r.Header.Get("Authorization"); got != "Bearer gho-test-token" {
					return fmt.Errorf("github: want Bearer gho-test-token, got %q", got)
				}
				return nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.Provider, func(t *testing.T) {
			upstream := mockOpenAIUpstream(t, tc.WantAuth)
			defer upstream.Close()

			srv, database := setupTestServer(t)

			conn := &model.ProviderConnection{
				ID:       "e2e-" + tc.Provider,
				Provider: tc.Provider,
				AuthType: tc.AuthType,
				IsActive: true,
				Data: model.ConnectionData{
					APIKey:      tc.APIKey,
					AccessToken: tc.Token,
					BaseURL:     upstream.URL,
				},
			}
			if err := database.CreateConnection(conn); err != nil {
				t.Fatalf("failed to create connection: %v", err)
			}

			body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"e2e test"}]}`, tc.Model)
			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}

			var resp map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			choices, ok := resp["choices"].([]any)
			if !ok || len(choices) == 0 {
				t.Fatalf("expected choices in response, got %v", resp)
			}
			msg := choices[0].(map[string]any)["message"].(map[string]any)
			if msg["content"] != "E2E OK" {
				t.Fatalf("unexpected content: %v", msg["content"])
			}
		})
	}
}

// TestE2ENoAuthAutoProvision verifies that NoAuth providers (free category)
// work out-of-the-box without any pre-existing connection: the gateway
// auto-provisions a connection and sends "Bearer public".
func TestE2ENoAuthAutoProvision(t *testing.T) {
	upstream := mockOpenAIUpstream(t, func(r *http.Request) error {
		if got := r.Header.Get("Authorization"); got != "Bearer public" {
			return fmt.Errorf("want Bearer public, got %q", got)
		}
		return nil
	})
	defer upstream.Close()

	srv, database := setupTestServer(t)

	// Override opencode's base URL by patching the registry temporarily.
	orig := provider.Registry["opencode"]
	patched := orig
	patched.BaseURL = upstream.URL
	provider.Registry["opencode"] = patched
	t.Cleanup(func() { provider.Registry["opencode"] = orig })

	// No connection exists — auto-provision should kick in.
	body := `{"model":"opencode/big-pickle","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify a connection was auto-provisioned.
	conns, _ := database.ListConnectionsByProvider("opencode")
	if len(conns) == 0 {
		t.Fatal("expected auto-provisioned connection for opencode")
	}
	if conns[0].AuthType != "none" {
		t.Fatalf("expected authType=none, got %s", conns[0].AuthType)
	}
}

// TestE2EStreaming verifies SSE streaming works end-to-end.
func TestE2EStreaming(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		chunks := []string{
			`{"id":"c1","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			`{"id":"c1","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"stream"},"finish_reason":null}]}`,
			`{"id":"c1","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	srv, database := setupTestServer(t)
	database.CreateConnection(&model.ProviderConnection{
		ID: "e2e-stream", Provider: "openai", AuthType: "api-key", IsActive: true,
		Data: model.ConnectionData{APIKey: "sk-stream", BaseURL: upstream.URL},
	})

	body := `{"model":"openai/gpt-4o","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	out := w.Body.String()
	if !strings.Contains(out, "data: [DONE]") {
		t.Fatalf("missing [DONE] in stream output")
	}
	if !strings.Contains(out, "stream") {
		t.Fatalf("missing content in stream output")
	}
}

// TestE2EAnthropicFormat verifies the full anthropic-format translation path:
// request is translated to Claude wire format, auth uses x-api-key + anthropic-version.
func TestE2EAnthropicFormat(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate anthropic auth headers.
		if got := r.Header.Get("x-api-key"); got != "sk-ant-test" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "bad key"})
			return
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "missing anthropic-version"})
			return
		}
		// Return anthropic-format response.
		resp := map[string]any{
			"id": "msg_e2e", "type": "message", "role": "assistant",
			"content":     []any{map[string]any{"type": "text", "text": "E2E anthropic OK"}},
			"model":       "claude-sonnet-4-20250514",
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 5, "output_tokens": 3},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	srv, database := setupTestServer(t)
	database.CreateConnection(&model.ProviderConnection{
		ID: "e2e-anthropic", Provider: "anthropic", AuthType: "api-key", IsActive: true,
		Data: model.ConnectionData{APIKey: "sk-ant-test", BaseURL: upstream.URL},
	})

	body := `{"model":"anthropic/claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}],"max_tokens":100}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// Response is translated back to OpenAI format.
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	choices, ok := resp["choices"].([]any)
	if !ok || len(choices) == 0 {
		t.Fatalf("expected choices, got %v", resp)
	}
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "E2E anthropic OK" {
		t.Fatalf("unexpected content: %v", msg["content"])
	}
}

// TestE2EGeminiFormat verifies the full gemini-format translation path:
// request is translated to Gemini wire format, auth uses ?key= query param.
func TestE2EGeminiFormat(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate gemini auth (query param).
		if got := r.URL.Query().Get("key"); got != "AIza-test" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "bad key"})
			return
		}
		// Return gemini-format response.
		resp := map[string]any{
			"candidates": []any{
				map[string]any{
					"content": map[string]any{
						"parts": []any{map[string]any{"text": "E2E gemini OK"}},
						"role":  "model",
					},
					"finishReason": "STOP",
				},
			},
			"usageMetadata": map[string]any{"promptTokenCount": 5, "candidatesTokenCount": 3, "totalTokenCount": 8},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	srv, database := setupTestServer(t)
	database.CreateConnection(&model.ProviderConnection{
		ID: "e2e-gemini", Provider: "gemini", AuthType: "api-key", IsActive: true,
		Data: model.ConnectionData{APIKey: "AIza-test", BaseURL: upstream.URL},
	})

	body := `{"model":"gemini/gemini-2.5-flash","messages":[{"role":"user","content":"hi"}]}`
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
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "E2E gemini OK" {
		t.Fatalf("unexpected content: %v", msg["content"])
	}
}

// TestE2ERegistryCompleteness verifies that all non-hidden chat providers in
// the registry have the minimum required fields for routing.
func TestE2ERegistryCompleteness(t *testing.T) {
	for id, info := range provider.Registry {
		if info.Hidden {
			continue
		}
		// Providers with runtime-resolved base URLs (OAuth-only) carry an
		// empty BaseURL by design; the category check covers them.
		if info.BaseURL == "" && info.Category != "oauth" {
			t.Errorf("provider %s: missing BaseURL", id)
		}
		if info.APIType == "" {
			t.Errorf("provider %s: missing APIType", id)
		}
		if info.Category == "" {
			t.Errorf("provider %s: missing Category", id)
		}
		if len(info.AuthModes) == 0 {
			t.Errorf("provider %s: missing AuthModes", id)
		}
	}
}

// TestE2ENoAuthProvidersHaveCorrectCategory verifies all visible NoAuth
// providers are in the "free" or "freeTier" category and have AuthType "none".
func TestE2ENoAuthProvidersHaveCorrectCategory(t *testing.T) {
	for id, info := range provider.Registry {
		if !info.NoAuth || info.Hidden {
			continue
		}
		if info.Category != "free" && info.Category != "freeTier" {
			t.Errorf("NoAuth provider %s: expected category free/freeTier, got %q", id, info.Category)
		}
		if info.AuthType != "none" {
			t.Errorf("NoAuth provider %s: expected authType=none, got %q", id, info.AuthType)
		}
	}
}
