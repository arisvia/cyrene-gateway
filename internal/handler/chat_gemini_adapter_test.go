package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
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

func TestGeminiAdapter_ForwardsSSEKeepaliveComment(t *testing.T) {
	rec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	adapter := newGeminiResponseAdapter(rec, true, "m")

	adapter.Write([]byte(": ping - queue waiting\n\n"))

	if !strings.Contains(rec.body.String(), ": ping - queue waiting") {
		t.Fatalf("keepalive comment must reach the client, got %q", rec.body.String())
	}
	if rec.flushes == 0 {
		t.Error("keepalive must be flushed immediately")
	}
}

func TestGeminiAdapter_KeepaliveDoesNotBreakEventStream(t *testing.T) {
	rec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	adapter := newGeminiResponseAdapter(rec, true, "m")

	adapter.Write([]byte(": ping - queue waiting\n\n"))
	adapter.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n"))

	out := rec.body.String()
	if !strings.Contains(out, ": ping - queue waiting\n\n") {
		t.Fatalf("comment frame must stay well-formed, got %q", out)
	}
	if !strings.Contains(out, "hi") {
		t.Fatalf("data frame after the keepalive must still be translated, got %q", out)
	}
}

func TestNativeGeminiPassthrough_LosslessFailoverAndAntigravityBypass(t *testing.T) {
	t.Run("lossless_nonstream_and_stream_with_failover", func(t *testing.T) {
		var sentBodies []map[string]any
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("key") == "key-fail" || r.Header.Get("Authorization") == "Bearer key-fail" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":{"code":429,"message":"quota exhausted","status":"RESOURCE_EXHAUSTED"}}`))
				return
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			sentBodies = append(sentBodies, body)
			if strings.Contains(r.URL.Path, "streamGenerateContent") {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Write([]byte("data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"thoughtSignature\":\"sig_stream_99\",\"text\":\"native-stream\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":9,\"candidatesTokenCount\":3,\"totalTokenCount\":12}}\n\n"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"thoughtSignature":"sig_resp_123","functionCall":{"name":"search","args":{"q":"test"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":6,"totalTokenCount":18}}`))
		}))
		defer upstream.Close()

		id := "native-gem-test"
		provider.Registry[id] = provider.ProviderInfo{ID: id, Name: id, BaseURL: upstream.URL + "/v1beta/models", APIType: "gemini", AuthType: "api-key"}
		defer delete(provider.Registry, id)

		srv, database := setupTestServer(t)
		_ = database.CreateConnection(&model.ProviderConnection{
			ID: "g1", Provider: id, AuthType: "api-key", IsActive: true, Priority: 1,
			Data: model.ConnectionData{APIKey: "key-fail"},
		})
		_ = database.CreateConnection(&model.ProviderConnection{
			ID: "g2", Provider: id, AuthType: "api-key", IsActive: true, Priority: 2,
			Data: model.ConnectionData{APIKey: "key-ok"},
		})

		reqBody := `{
			"contents": [{"role":"user","parts":[{"text":"hello"},{"thoughtSignature":"sig_in_abc","text":"ctx"}]}],
			"safetySettings": [{"category":"HARM_CATEGORY_HATE_SPEECH","threshold":"BLOCK_NONE"}],
			"tools": [{"googleSearch": {}}],
			"generationConfig": {
				"responseSchema": {"type": "OBJECT"},
				"thinkingConfig": {"thinkingLevel": "HIGH"}
			}
		}`
		w := httptest.NewRecorder()
		srv.Router.ServeHTTP(w, httptest.NewRequest("POST", "/v1beta/models/"+id+"/gemini-2.5-pro:generateContent", strings.NewReader(reqBody)))
		if w.Code != http.StatusOK {
			t.Fatalf("native gemini status=%d body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "sig_resp_123") {
			t.Fatalf("response dropped thoughtSignature: %s", w.Body.String())
		}
		if len(sentBodies) == 0 {
			t.Fatal("expected upstream hit")
		}
		sent := sentBodies[len(sentBodies)-1]
		if _, hasModel := sent["model"]; hasModel {
			t.Fatalf("native Gemini request must not carry top-level model field: %#v", sent["model"])
		}
		if sent["safetySettings"] == nil || sent["tools"] == nil {
			t.Fatalf("native Gemini fields lost upstream: %#v", sent)
		}
		gc := sent["generationConfig"].(map[string]any)
		tc := gc["thinkingConfig"].(map[string]any)
		if gc["responseSchema"] == nil || tc["thinkingLevel"] != "HIGH" {
			t.Fatalf("generationConfig lost fields: %#v", gc)
		}
		if !strings.Contains(mustMarshalTestJSON(sent["contents"]), "sig_in_abc") {
			t.Fatalf("request thoughtSignature lost upstream: %#v", sent["contents"])
		}

		// Streaming passthrough
		wS := httptest.NewRecorder()
		srv.Router.ServeHTTP(wS, httptest.NewRequest("POST", "/v1beta/models/"+id+"/gemini-2.5-pro:streamGenerateContent?alt=sse", strings.NewReader(reqBody)))
		if wS.Code != http.StatusOK || !strings.Contains(wS.Body.String(), "sig_stream_99") {
			t.Fatalf("native gemini stream failed: %d %s", wS.Code, wS.Body.String())
		}
		details, err := database.GetRequestDetails(db.RequestDetailFilter{Provider: id, Page: 1, PageSize: 10})
		if err != nil || details.Pagination.TotalItems < 2 {
			t.Fatalf("expected recorded gemini details, got %d err=%v", details.Pagination.TotalItems, err)
		}
	})

	t.Run("antigravity_does_not_use_native_gemini_passthrough", func(t *testing.T) {
		var sawV1Internal bool
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sawV1Internal = strings.Contains(r.URL.Path, "v1internal")
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("data: {\"response\":{\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"ag-ok\"}]},\"finishReason\":\"STOP\"}]}}\n\n"))
		}))
		defer upstream.Close()

		srv, database := setupTestServer(t)
		_ = database.CreateConnection(&model.ProviderConnection{
			ID: "ag-gem", Provider: "antigravity", AuthType: "oauth", IsActive: true,
			Data: model.ConnectionData{AccessToken: "ag-tok", BaseURL: upstream.URL, ProviderSpecificData: map[string]any{"projectId": "p1"}},
		})

		w := httptest.NewRecorder()
		srv.Router.ServeHTTP(w, httptest.NewRequest("POST", "/v1beta/models/antigravity/gemini-3.8-flash:generateContent", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)))
		if w.Code != http.StatusOK || !sawV1Internal || !strings.Contains(w.Body.String(), "ag-ok") {
			t.Fatalf("antigravity must route via v1internal envelope: code=%d sawV1Internal=%t body=%s", w.Code, sawV1Internal, w.Body.String())
		}
	})

	t.Run("loopguard_exclude_and_interrupted_forcestream_502", func(t *testing.T) {
		var sawBody map[string]any
		interrupt := false
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&sawBody)
			if interrupt {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Write([]byte("data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"half\"}]}}]}\n\n"))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				if hj, ok := w.(http.Hijacker); ok {
					if conn, _, err := hj.Hijack(); err == nil {
						conn.Close()
					}
				}
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"gem-ok"}]},"finishReason":"STOP"}]}`))
		}))
		defer upstream.Close()

		id := "native-gem-guard"
		provider.Registry[id] = provider.ProviderInfo{ID: id, Name: id, BaseURL: upstream.URL, APIType: "gemini", AuthType: "api-key"}
		defer delete(provider.Registry, id)

		srv, database := setupTestServer(t)
		_ = database.CreateConnection(&model.ProviderConnection{
			ID: "c-gem-g", Provider: id, AuthType: "api-key", IsActive: true, Data: model.ConnectionData{APIKey: "k"},
		})
		st, _ := database.GetSettings()
		st.LoopGuardEnabled = true
		_ = database.SaveSettings(st)

		reqBody := `{
			"tools": [{"functionDeclarations": [{"name": "lookup"}]}],
			"contents": [
				{"role": "model", "parts": [{"functionCall": {"name": "lookup", "args": {"k": "v"}}}]},
				{"role": "model", "parts": [{"functionCall": {"name": "lookup", "args": {"k": "v"}}}]},
				{"role": "model", "parts": [{"functionCall": {"name": "lookup", "args": {"k": "v"}}}]}
			]
		}`
		w := httptest.NewRecorder()
		srv.Router.ServeHTTP(w, httptest.NewRequest("POST", "/v1beta/models/"+id+"/gemini-2.5-pro:generateContent", strings.NewReader(reqBody)))
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(mustMarshalTestJSON(sawBody["systemInstruction"]), "STOP repeating") {
			t.Errorf("expected LoopGuard hint in Gemini systemInstruction, got %#v", sawBody["systemInstruction"])
		}

		// Excluded provider skips LoopGuard
		st.TokenSaverExclude = []string{id}
		_ = database.SaveSettings(st)
		sawBody = nil
		wEx := httptest.NewRecorder()
		srv.Router.ServeHTTP(wEx, httptest.NewRequest("POST", "/v1beta/models/"+id+"/gemini-2.5-pro:generateContent", strings.NewReader(reqBody)))
		if wEx.Code != http.StatusOK {
			t.Fatalf("excluded status=%d body=%s", wEx.Code, wEx.Body.String())
		}
		if sawBody["systemInstruction"] != nil {
			t.Errorf("excluded Gemini request must not inject systemInstruction, got %#v", sawBody["systemInstruction"])
		}

		// ForceStream interrupted non-streaming aggregation must return 502 instead of fake 200
		info := provider.Registry[id]
		info.ForceStream = true
		provider.Registry[id] = info
		interrupt = true
		wTrunc := httptest.NewRecorder()
		srv.Router.ServeHTTP(wTrunc, httptest.NewRequest("POST", "/v1beta/models/"+id+"/gemini-2.5-pro:generateContent", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)))
		if wTrunc.Code != http.StatusBadGateway {
			t.Fatalf("expected 502 on interrupted Gemini non-stream SSE aggregation, got %d body=%s", wTrunc.Code, wTrunc.Body.String())
		}
	})
}

func mustMarshalTestJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
