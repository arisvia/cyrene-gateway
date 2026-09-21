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

func TestGemini_NonStream(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)

		msgs, ok := req["messages"].([]any)
		if !ok || len(msgs) != 2 {
			t.Errorf("expected 2 messages (system + user), got: %v", msgs)
		}

		resp := map[string]any{
			"id":     "chatcmpl-gemini-1",
			"object": "chat.completion",
			"model":  "openai/mock-openai-model",
			"choices": []any{
				map[string]any{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "Gemini generateContent succeeded!",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     12,
				"completion_tokens": 6,
				"total_tokens":      18,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	database.CreateConnection(&model.ProviderConnection{
		ID:        "conn-mock-gemini-inbound",
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

	geminiReqBody := `{
		"systemInstruction": {
			"parts": [{"text": "You are Gemini."}]
		},
		"contents": [
			{
				"role": "user",
				"parts": [{"text": "Hello Gemini!"}]
			}
		],
		"generationConfig": {
			"temperature": 0.5,
			"maxOutputTokens": 1000
		}
	}`

	req := httptest.NewRequest("POST", "/v1beta/models/mock-openai-model:generateContent", strings.NewReader(geminiReqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-mock")

	rec := httptest.NewRecorder()
	srv.Router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var geminiResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &geminiResp); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	candidates, ok := geminiResp["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		t.Fatalf("expected candidates in response, got: %+v", geminiResp)
	}

	cand0 := candidates[0].(map[string]any)
	content := cand0["content"].(map[string]any)
	parts := content["parts"].([]any)
	part0 := parts[0].(map[string]any)
	if part0["text"] != "Gemini generateContent succeeded!" {
		t.Errorf("unexpected output text: %v", part0["text"])
	}

	meta := geminiResp["usageMetadata"].(map[string]any)
	if meta["promptTokenCount"] != 12.0 && meta["promptTokenCount"] != 12 {
		t.Errorf("expected promptTokenCount=12, got: %v", meta["promptTokenCount"])
	}
}

func TestGemini_Stream(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		chunk := `data: {"id":"chatcmpl-stream-1","object":"chat.completion.chunk","created":1720000000,"model":"mock-openai-model","choices":[{"index":0,"delta":{"content":"Chunk from Gemini"},"finish_reason":null}]}` + "\n\n"
		w.Write([]byte(chunk))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		finishChunk := `data: {"id":"chatcmpl-stream-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n"
		w.Write([]byte(finishChunk))

		usageChunk := `data: {"id":"chatcmpl-stream-1","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":20,"completion_tokens":10,"total_tokens":30}}` + "\n\n"
		w.Write([]byte(usageChunk))

		done := "data: [DONE]\n\n"
		w.Write([]byte(done))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)

	database.CreateConnection(&model.ProviderConnection{
		ID:        "conn-mock-gemini-stream",
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

	geminiReqBody := `{
		"contents": [
			{
				"role": "user",
				"parts": [{"text": "Stream test"}]
			}
		]
	}`

	req := httptest.NewRequest("POST", "/v1beta/models/mock-openai-model:streamGenerateContent", strings.NewReader(geminiReqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-mock")

	rec := httptest.NewRecorder()
	srv.Router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	bodyStr := rec.Body.String()
	if !strings.Contains(bodyStr, "Chunk from Gemini") {
		t.Fatalf("expected response body to contain Chunk from Gemini, got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "STOP") {
		t.Fatalf("expected response body to contain STOP on stream finish, got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "usageMetadata") || !strings.Contains(bodyStr, "promptTokenCount") {
		t.Fatalf("expected response body to contain usageMetadata, got: %s", bodyStr)
	}
}
