package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQoderStreamingResponseCapture(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Qoder SSE envelope wrapping OpenAI chunks
	bodyChunk1 := `{"id":"chatcmpl-qoder-1","choices":[{"delta":{"content":"Qoder "}}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`
	b1, _ := json.Marshal(bodyChunk1)
	envelope1 := `{"statusCodeValue":200,"body":` + string(b1) + `}`

	bodyChunk2 := `{"id":"chatcmpl-qoder-1","choices":[{"delta":{"content":"OK"}}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`
	b2, _ := json.Marshal(bodyChunk2)
	envelope2 := `{"statusCodeValue":200,"body":` + string(b2) + `}`

	sseData := "data: " + envelope1 + "\n\ndata: " + envelope2 + "\n\ndata: [DONE]\n\n"

	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(sseData)),
		Header:     make(http.Header),
	}
	uc := &usageContext{
		StartedAt: time.Now(),
		Provider:  "qoder",
		Model:     "efficient",
		Prompt:    "ping",
	}
	w := httptest.NewRecorder()
	srv.proxyQoderStreaming(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, "efficient", uc)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if uc.Response != "Qoder OK" {
		t.Errorf("expected uc.Response to be 'Qoder OK', got %q", uc.Response)
	}
}

func TestQoderNonStreamingResponseCapture(t *testing.T) {
	srv, _ := setupTestServer(t)

	bodyChunk := `{"id":"chatcmpl-qoder-2","choices":[{"delta":{"role":"assistant","content":"Qoder NonStream OK"}}],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`
	b, _ := json.Marshal(bodyChunk)
	envelope := `{"statusCodeValue":200,"body":` + string(b) + `}`

	sseData := "data: " + envelope + "\n\ndata: [DONE]\n\n"

	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(sseData)),
		Header:     make(http.Header),
	}
	uc := &usageContext{
		StartedAt: time.Now(),
		Provider:  "qoder",
		Model:     "efficient",
		Prompt:    "hello",
	}
	w := httptest.NewRecorder()
	srv.proxyQoderNonStreaming(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, "efficient", uc)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if uc.Response != "Qoder NonStream OK" {
		t.Errorf("expected uc.Response to be 'Qoder NonStream OK', got %q", uc.Response)
	}
}
