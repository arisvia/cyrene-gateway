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

// A streaming client must still get its answer when the upstream ignores
// stream=true and replies with a plain JSON body.
func TestStreamingClient_NonSSEUpstream_StillStreams(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","model":"up-model","choices":[{"index":0,"message":{"role":"assistant","content":"PLAIN ANSWER"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	}))
	defer up.Close()

	srv, database := setupTestServer(t)
	database.CreateConnection(&model.ProviderConnection{
		ID: "conn-nonsse", Provider: "openai", AuthType: "api-key", IsActive: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Data: model.ConnectionData{APIKey: "sk", BaseURL: up.URL + "/chat/completions"},
	})

	cases := []struct{ name, path, body string }{
		{"messages", "/v1/messages", `{"model":"openai/m","max_tokens":10,"stream":true,"messages":[{"role":"user","content":"hi"}]}`},
		{"responses", "/v1/responses", `{"model":"openai/m","stream":true,"input":"hi"}`},
		{"gemini", "/v1beta/models/m:streamGenerateContent", `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`},
	}
	for _, c := range cases {
		req := httptest.NewRequest("POST", c.path, strings.NewReader(c.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("[%s] expected 200, got %d: %s", c.name, rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "PLAIN ANSWER") {
			t.Errorf("[%s] answer lost when upstream ignored stream=true: %s", c.name, body)
		}
		if strings.Contains(body, "stream ended before completion") {
			t.Errorf("[%s] bogus truncation error: %s", c.name, body)
		}
	}
}

func TestGeminiCountTokens(t *testing.T) {
	srv, _ := setupTestServer(t)
	body := `{"contents":[{"role":"user","parts":[{"text":"count these tokens please"}]}]}`
	req := httptest.NewRequest("POST", "/v1beta/models/m:countTokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := resp["candidates"]; ok {
		t.Error("countTokens must not return a generation payload")
	}
	if n, ok := resp["totalTokens"].(float64); !ok || n <= 0 {
		t.Errorf("expected positive totalTokens, got %v", resp["totalTokens"])
	}
}

func TestGeminiEmbedContent(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]}],"usage":{"prompt_tokens":2,"total_tokens":2}}`))
	}))
	defer up.Close()

	srv, database := setupTestServer(t)
	database.CreateConnection(&model.ProviderConnection{
		ID: "conn-emb", Provider: "openai", AuthType: "api-key", IsActive: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Data: model.ConnectionData{APIKey: "sk", BaseURL: up.URL + "/chat/completions"},
	})

	req := httptest.NewRequest("POST", "/v1beta/models/emb:embedContent", strings.NewReader(`{"content":{"parts":[{"text":"hello"}]}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Embedding struct {
			Values []float64 `json:"values"`
		} `json:"embedding"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp.Embedding.Values) != 3 {
		t.Errorf("expected 3 embedding values, got %v (%s)", resp.Embedding.Values, rec.Body.String())
	}
}

func TestGeminiUnsupportedAction(t *testing.T) {
	srv, _ := setupTestServer(t)
	req := httptest.NewRequest("POST", "/v1beta/models/m:countText", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown action, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMessagesUpstreamErrorCarriesHint(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"bad key","type":"auth_error"}}`))
	}))
	defer up.Close()

	srv, database := setupTestServer(t)
	database.CreateConnection(&model.ProviderConnection{
		ID: "conn-401", Provider: "anthropic", AuthType: "oauth", IsActive: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Data: model.ConnectionData{AccessToken: "tok", BaseURL: up.URL},
	})

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"anthropic/claude-3","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "hint") {
		t.Errorf("expected actionable hint on upstream error, got %s", rec.Body.String())
	}
}
