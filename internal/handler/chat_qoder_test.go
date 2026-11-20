package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/cache"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
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
	srv.proxyQoderStreaming(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, "efficient", uc, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if uc.Response != "Qoder OK" {
		t.Errorf("expected uc.Response to be 'Qoder OK', got %q", uc.Response)
	}
}

func TestQoderStreamingQueueingFrameSkipped(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Leading queueing handshake envelope (status 403 or 200 with isQueued: true)
	queueMsg := `{"code":"403","message":"{\"code\":\"10605\",\"message\":\"{\\\"isQueued\\\":true,\\\"modelKey\\\":\\\"dfmodel\\\",\\\"queueCount\\\":0}\"}"}`
	bQueue, _ := json.Marshal(queueMsg)
	envelopeQueue := `{"statusCodeValue":403,"body":` + string(bQueue) + `}`

	// Followed by valid output data
	bodyChunk := `{"id":"chatcmpl-qoder-q","choices":[{"delta":{"content":"Answer after queue"}}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`
	bData, _ := json.Marshal(bodyChunk)
	envelopeData := `{"statusCodeValue":200,"body":` + string(bData) + `}`

	sseData := "data: " + envelopeQueue + "\n\ndata: " + envelopeData + "\n\ndata: [DONE]\n\n"

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
	srv.proxyQoderStreaming(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, "efficient", uc, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 after skipping queue frame, got %d: body=%s", w.Code, w.Body.String())
	}
	if uc.Response != "Answer after queue" {
		t.Errorf("expected uc.Response 'Answer after queue', got %q", uc.Response)
	}
}
func TestQoderQueueAutoRetrySuccess(t *testing.T) {
	srv, _ := setupTestServer(t)

	var attempts atomic.Int32
	queueMsg := `{"code":"403","message":"{\"code\":\"10605\",\"message\":\"{\\\"isQueued\\\":true,\\\"modelKey\\\":\\\"dfmodel\\\",\\\"queueCount\\\":0,\\\"retryAfterSeconds\\\":1}\"}"}`
	bQueue, _ := json.Marshal(queueMsg)
	envelopeQueue := `data: {"statusCodeValue":403,"body":` + string(bQueue) + `}` + "\n\n"

	bodyChunk := `{"id":"chatcmpl-qoder-ok","choices":[{"delta":{"content":"Polled OK"}}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`
	bData, _ := json.Marshal(bodyChunk)
	envelopeSuccess := `data: {"statusCodeValue":200,"body":` + string(bData) + `}` + "\n\ndata: [DONE]\n\n"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		att := attempts.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if att == 1 {
			// First attempt returns queue envelope
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(envelopeQueue))
			return
		}
		// Second attempt succeeds
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(envelopeSuccess))
	}))
	defer upstream.Close()

	// Mock catalog cache so BuildQoderRequestBody finds dfmodel config immediately
	provider.SeedQoderCatalogForTest("user-queue-test", map[string]map[string]any{
		"dfmodel": {"name": "dfmodel", "max_output_tokens": 4096},
	})

	conn := &model.ProviderConnection{
		ID:       "qoder-queue-conn",
		Provider: "qoder",
		AuthType: "oauth",
		IsActive: true,
		Data: model.ConnectionData{
			AccessToken: "dt-mock-token",
			BaseURL:     upstream.URL,
			ProviderSpecificData: map[string]any{
				"userId": "user-queue-test",
			},
		},
	}

	reqBody := []byte(`{"model":"qoder/dfmodel","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	r := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(reqBody))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	var chatReq ChatCompletionRequest
	json.Unmarshal(reqBody, &chatReq)
	modelInfo := model.ModelInfo{Provider: "qoder", Model: "dfmodel"}
	providerInfo, _ := provider.GetProvider("qoder")

	srv.handleQoderChat(w, r, chatReq, reqBody, modelInfo, conn, providerInfo)

	if attempts.Load() < 2 {
		t.Fatalf("expected at least 2 attempts (retry after 10605), got %d", attempts.Load())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}
	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "Polled OK") {
		t.Fatalf("expected streamed content 'Polled OK', got body: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, ": qoder model dfmodel queued") {
		t.Errorf("expected keepalive comment frame with queue info, got body: %s", bodyStr)
	}
}

// TestQoderPeekPreservesBufferedChunks guards the peek path against dropping
// read-ahead bytes: the upstream writes several SSE frames into one TCP
// segment, so anything buffered past the peeked first line must still reach the
// client. A bufio.Scanner here would silently swallow them.
func TestQoderPeekPreservesBufferedChunks(t *testing.T) {
	srv, _ := setupTestServer(t)

	chunks := []string{"alpha-", "beta-", "gamma"}
	var sse strings.Builder
	for _, c := range chunks {
		body := `{"id":"chatcmpl-peek","choices":[{"delta":{"content":"` + c + `"}}]}`
		encoded, _ := json.Marshal(body)
		sse.WriteString(`data: {"statusCodeValue":200,"body":` + string(encoded) + "}\n\n")
	}
	sse.WriteString("data: [DONE]\n\n")
	payload := sse.String()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Single write: every frame lands in one segment and thus in the peek buffer.
		w.Write([]byte(payload))
	}))
	defer upstream.Close()

	provider.SeedQoderCatalogForTest("user-peek-test", map[string]map[string]any{
		"dfmodel": {"name": "dfmodel", "max_output_tokens": 4096},
	})

	conn := &model.ProviderConnection{
		ID:       "qoder-peek-conn",
		Provider: "qoder",
		AuthType: "oauth",
		IsActive: true,
		Data: model.ConnectionData{
			AccessToken: "dt-mock-token-peek",
			BaseURL:     upstream.URL,
			ProviderSpecificData: map[string]any{
				"userId": "user-peek-test",
			},
		},
	}

	reqBody := []byte(`{"model":"qoder/dfmodel","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	r := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(reqBody))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	var chatReq ChatCompletionRequest
	json.Unmarshal(reqBody, &chatReq)
	modelInfo := model.ModelInfo{Provider: "qoder", Model: "dfmodel"}
	providerInfo, _ := provider.GetProvider("qoder")

	srv.handleQoderChat(w, r, chatReq, reqBody, modelInfo, conn, providerInfo)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}
	out := w.Body.String()
	for _, c := range chunks {
		if !strings.Contains(out, c) {
			t.Fatalf("chunk %q was dropped from the stream; body: %s", c, out)
		}
	}
}

func TestQoderQueueHTTPStatusRetrySuccess(t *testing.T) {
	srv, _ := setupTestServer(t)

	var attempts atomic.Int32
	queueMsg := `{"code":"403","message":"{\"code\":\"10605\",\"message\":\"{\\\"isQueued\\\":true,\\\"modelKey\\\":\\\"dfmodel\\\",\\\"queueCount\\\":0,\\\"retryAfterSeconds\\\":1}\"}"}`

	bodyChunk := `{"id":"chatcmpl-qoder-ok2","choices":[{"delta":{"content":"Retry 403 OK"}}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`
	bData, _ := json.Marshal(bodyChunk)
	envelopeSuccess := `data: {"statusCodeValue":200,"body":` + string(bData) + `}` + "\n\ndata: [DONE]\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		att := attempts.Add(1)
		if att == 1 {
			// First attempt returns HTTP 403 with 10605 queue JSON
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(queueMsg))
			return
		}
		// Second attempt succeeds with SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(envelopeSuccess))
	}))
	defer upstream.Close()

	provider.SeedQoderCatalogForTest("user-queue-http", map[string]map[string]any{
		"dfmodel": {"name": "dfmodel", "max_output_tokens": 4096},
	})

	conn := &model.ProviderConnection{
		ID:       "qoder-queue-http-conn",
		Provider: "qoder",
		AuthType: "oauth",
		IsActive: true,
		Data: model.ConnectionData{
			AccessToken: "dt-mock-token-2",
			BaseURL:     upstream.URL,
			ProviderSpecificData: map[string]any{
				"userId": "user-queue-http",
			},
		},
	}

	reqBody := []byte(`{"model":"qoder/dfmodel","stream":true,"messages":[{"role":"user","content":"ping"}]}`)
	r := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(reqBody))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	var chatReq ChatCompletionRequest
	json.Unmarshal(reqBody, &chatReq)
	modelInfo := model.ModelInfo{Provider: "qoder", Model: "dfmodel"}
	providerInfo, _ := provider.GetProvider("qoder")

	srv.handleQoderChat(w, r, chatReq, reqBody, modelInfo, conn, providerInfo)

	if attempts.Load() < 2 {
		t.Fatalf("expected at least 2 attempts, got %d", attempts.Load())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}
	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "Retry 403 OK") {
		t.Fatalf("expected streamed content 'Retry 403 OK', got: %s", bodyStr)
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
	srv.proxyQoderNonStreaming(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, "efficient", uc, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if uc.Response != "Qoder NonStream OK" {
		t.Errorf("expected uc.Response to be 'Qoder NonStream OK', got %q", uc.Response)
	}
}

func qoderTestFrame(body string) string {
	encoded, _ := json.Marshal(body)
	return `data: {"statusCodeValue":200,"body":` + string(encoded) + "}\n\n"
}

type qoderBrokenReader struct{}

func (qoderBrokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestQoderFailuresAreNotSuccessfulOrCacheable(t *testing.T) {
	partial := qoderTestFrame(`{"choices":[{"index":0,"delta":{"content":"partial answer"}}]}`)
	for _, tc := range []struct {
		name   string
		body   string
		broken bool
		status int
	}{
		{"rate-limit", `data: {"statusCodeValue":429,"body":"rate limited"}` + "\n\n", false, 429},
		{"server-error-after-content", partial + `data: {"statusCodeValue":503,"body":"unavailable"}` + "\n\n", false, 503},
		{"invalid-http-status", `data: {"statusCodeValue":201,"body":"not a completion"}` + "\n\n", false, 502},
		{"inner-error", partial + qoderTestFrame(`{"error":{"message":"upstream failed","type":"server_error"}}`), false, 502},
		{"read-error", partial, true, 502},
		{"scanner-limit", partial + "data: " + strings.Repeat("x", 1024*1024) + "\n", false, 502},
		{"truncated", partial, false, 502},
		{"empty", "", false, 502},
		{"invalid-envelope", "data: {broken}\n\n", false, 502},
		{"invalid-inner", qoderTestFrame("not-json"), false, 502},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				srv, database := setupTestServer(t)
				var reader io.Reader = strings.NewReader(tc.body)
				if tc.broken {
					reader = io.MultiReader(reader, qoderBrokenReader{})
				}
				resp := &http.Response{StatusCode: 200, Body: io.NopCloser(reader)}
				uc := &usageContext{StartedAt: time.Now(), Provider: "qoder", Model: "efficient", Status: 200, Prompt: "test prompt"}
				w := httptest.NewRecorder()
				rec := cache.NewRecorder(w)
				r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
				if stream {
					srv.proxyQoderStreaming(rec, r, resp, "efficient", uc, nil)
					if !strings.Contains(w.Body.String(), "event: error\n") || strings.Contains(w.Body.String(), "[DONE]") {
						t.Errorf("failure disguised as successful SSE: %s", w.Body.String())
					}
				} else {
					srv.proxyQoderNonStreaming(rec, r, resp, "efficient", uc, nil)
					if w.Code != tc.status {
						t.Errorf("HTTP status = %d, want %d", w.Code, tc.status)
					}
					var result map[string]json.RawMessage
					if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result["error"]) == 0 || result["choices"] != nil {
						t.Errorf("expected protocol error, got %s", w.Body.String())
					}
				}
				if uc.Status != tc.status || rec.ShouldCache() {
					t.Errorf("failure status=%d cacheable=%t", uc.Status, rec.ShouldCache())
				}
				entries, err := database.GetUsageHistory(db.UsageFilter{Provider: "qoder"})
				if err != nil || len(entries) != 1 {
					t.Fatalf("usage entries=%v err=%v", entries, err)
				}
				if entries[0].Status != fmt.Sprint(tc.status) {
					t.Errorf("persisted usage status = %q", entries[0].Status)
				}
				details, err := database.GetRequestDetails(db.RequestDetailFilter{Provider: "qoder", Page: 1, PageSize: 10})
				if err != nil || len(details.Details) != 1 {
					t.Fatalf("request details=%v err=%v", details, err)
				}
				var detail map[string]any
				if err := json.Unmarshal(details.Details[0], &detail); err != nil {
					t.Fatal(err)
				}
				if detail["status"] != fmt.Sprint(tc.status) {
					t.Errorf("request detail status=%v", detail["status"])
				}
			})
		}
	}
}

func TestQoderStreamingWithoutFlusherDoesNotRelayUncheckedBody(t *testing.T) {
	srv, database := setupTestServer(t)
	w := httptest.NewRecorder()
	// Hide Flush without replacing the real writer's header/body behavior.
	writer := struct{ http.ResponseWriter }{w}
	uc := &usageContext{Provider: "qoder", Model: "efficient", StartedAt: time.Now(), Status: 200}
	resp := &http.Response{StatusCode: 200, Body: io.NopCloser(io.MultiReader(
		strings.NewReader(qoderTestFrame(`{"choices":[{"delta":{"content":"partial"}}]}`)), qoderBrokenReader{},
	))}
	srv.proxyQoderStreaming(writer, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, "efficient", uc, nil)
	if w.Code < 400 || uc.Status < 400 || !strings.Contains(w.Body.String(), `"error"`) {
		t.Errorf("unchecked relay succeeded: HTTP %d status=%d body=%s", w.Code, uc.Status, w.Body.String())
	}
	entries, err := database.GetUsageHistory(db.UsageFilter{Provider: "qoder"})
	if err != nil || len(entries) != 1 || entries[0].Status == "ok" {
		t.Errorf("failure not persisted: entries=%v err=%v", entries, err)
	}
}

func TestQoderTerminalFrameStopsReading(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, terminal := range []string{"data: [DONE]\n\n", qoderTestFrame("[DONE]")} {
			srv, _ := setupTestServer(t)
			body := qoderTestFrame(`{"choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`) + terminal
			resp := &http.Response{StatusCode: 200, Body: io.NopCloser(io.MultiReader(strings.NewReader(body), qoderBrokenReader{}))}
			uc := &usageContext{Provider: "qoder", Model: "efficient", StartedAt: time.Now(), Status: 200}
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
			if stream {
				srv.proxyQoderStreaming(w, r, resp, "efficient", uc, nil)
			} else {
				srv.proxyQoderNonStreaming(w, r, resp, "efficient", uc, nil)
			}
			if w.Code != 200 || uc.Status != 200 || !strings.Contains(w.Body.String(), `"content":"ok"`) || strings.Contains(w.Body.String(), `"error"`) {
				t.Errorf("terminal frame not respected: HTTP %d status=%d body=%s", w.Code, uc.Status, w.Body.String())
			}
		}
	}
}

func TestQoderNonStreamingAggregatesInterleavedToolsByChoice(t *testing.T) {
	srv, _ := setupTestServer(t)
	frames := []string{
		`{"id":"chatcmpl-tools","created":123,"choices":[{"index":1,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"other","type":"function","function":{"name":"other","arguments":"{}"}}]}},{"index":0,"delta":{"role":"assistant","reasoning_content":"think","tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"search","arguments":"{\"q\":"}},{"index":0,"id":"call_a","type":"function","function":{"name":"weather","arguments":"{\"city\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\"}"}},{"index":1,"function":{"arguments":"\"Go\"}"}}]},"finish_reason":"tool_calls"},{"index":1,"delta":{},"finish_reason":"tool_calls"}]}`,
	}
	body := qoderTestFrame(frames[0]) + qoderTestFrame(frames[1]) + "data: [DONE]\n\n"
	w := httptest.NewRecorder()
	srv.proxyQoderNonStreaming(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, "efficient", nil, nil)
	var result struct {
		ID      string `json:"id"`
		Created int    `json:"created"`
		Choices []struct {
			Index        int    `json:"index"`
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Role      string           `json:"role"`
				Reasoning string           `json:"reasoning_content"`
				ToolCalls []map[string]any `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Choices) != 2 {
		t.Fatalf("expected two choices, got %s", w.Body.String())
	}
	if result.ID != "chatcmpl-tools" || result.Created != 123 || result.Choices[0].Message.Reasoning != "think" {
		t.Errorf("metadata lost: %s", w.Body.String())
	}
	wants := [][]struct{ id, name, args string }{
		{{"call_a", "weather", `{"city":"Paris"}`}, {"call_b", "search", `{"q":"Go"}`}},
		{{"other", "other", `{}`}},
	}
	for i, choice := range result.Choices {
		if choice.Index != i || choice.FinishReason != "tool_calls" || choice.Message.Role != "assistant" {
			t.Errorf("bad choice: %+v", choice)
		}
		if len(choice.Message.ToolCalls) != len(wants[i]) {
			t.Fatalf("tool fragments not merged: %+v", choice)
		}
		for j, call := range choice.Message.ToolCalls {
			want := wants[i][j]
			fn, ok := call["function"].(map[string]any)
			if !ok || call["id"] != want.id || call["type"] != "function" || fn["name"] != want.name || fn["arguments"] != want.args {
				t.Errorf("tool = %+v, want %+v", call, want)
			}
			if _, exists := call["index"]; exists {
				t.Errorf("stream-only tool index leaked: %+v", call)
			}
		}
	}
}

// The COSY signature covers Cosy-Version, but the chat request also needs the
// CLI User-Agent: Go would otherwise send "Go-http-client/2.0", naming the
// relay to Qoder's channel check.
func TestQoderChatSendsCLIUserAgent(t *testing.T) {
	srv, _ := setupTestServer(t)

	uaCh := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case uaCh <- r.Header.Get("User-Agent"):
		default:
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	provider.SeedQoderCatalogForTest("user-ua", map[string]map[string]any{
		"dfmodel": {"name": "dfmodel", "max_output_tokens": 4096},
	})
	conn := &model.ProviderConnection{
		ID: "qoder-ua-conn", Provider: "qoder", AuthType: "oauth", IsActive: true,
		Data: model.ConnectionData{
			AccessToken:          "dt-mock-token-ua",
			BaseURL:              upstream.URL,
			ProviderSpecificData: map[string]any{"userId": "user-ua"},
		},
	}

	reqBody := []byte(`{"model":"qoder/dfmodel","stream":true,"messages":[{"role":"user","content":"ping"}]}`)
	r := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(reqBody))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	var chatReq ChatCompletionRequest
	json.Unmarshal(reqBody, &chatReq)
	modelInfo := model.ModelInfo{Provider: "qoder", Model: "dfmodel"}
	providerInfo, _ := provider.GetProvider("qoder")
	srv.handleQoderChat(w, r, chatReq, reqBody, modelInfo, conn, providerInfo)

	select {
	case got := <-uaCh:
		if got != provider.QoderCLIUserAgent {
			t.Errorf("User-Agent = %q, want %q", got, provider.QoderCLIUserAgent)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("upstream never received the request")
	}
}

// qoderQueueMessage renders the 10605 pre-execution queue payload Qoder returns with
// an HTTP 403, asking for waitSec before the model will be admitted.
func qoderQueueMessage(t *testing.T, waitSec int) string {
	t.Helper()
	inner, err := json.Marshal(map[string]any{
		"isQueued": true, "modelKey": "dfmodel", "queueCount": 0, "retryAfterSeconds": waitSec,
	})
	if err != nil {
		t.Fatal(err)
	}
	mid, err := json.Marshal(map[string]any{"code": "10605", "message": string(inner)})
	if err != nil {
		t.Fatal(err)
	}
	outer, err := json.Marshal(map[string]any{"code": "403", "message": string(mid)})
	if err != nil {
		t.Fatal(err)
	}
	return string(outer)
}

// TestQoderQueueFailsOverToIdleAccount: Qoder's 10605 is a *pre-execution* queue, so
// a queued account has not begun to bill anything and an idle sibling can take the
// request at no risk. Measured on production, one account spent 453 queue events in a
// 54-minute window (p50 43.5 s, max 301.9 s, zero completions) while the provider's
// other accounts answered in seconds — and clients ran out of their own retry budget
// first, which is how a whole agent fleet appears to stall.
func TestQoderQueueFailsOverToIdleAccount(t *testing.T) {
	srv, database := setupTestServer(t)

	var busyHits, idleHits atomic.Int32
	busy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		busyHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, qoderQueueMessage(t, 30))
	}))
	defer busy.Close()

	frame := qoderTestFrame(`{"id":"chatcmpl-failover","choices":[{"index":0,"delta":{"role":"assistant","content":"Served by the idle account"}}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`) + "data: [DONE]\n\n"
	idle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idleHits.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, frame)
	}))
	defer idle.Close()

	provider.SeedQoderCatalogForTest("user-busy", map[string]map[string]any{"dfmodel": {"name": "dfmodel", "max_output_tokens": 4096}})
	provider.SeedQoderCatalogForTest("user-idle", map[string]map[string]any{"dfmodel": {"name": "dfmodel", "max_output_tokens": 4096}})

	// Priority decides the first pick, so the queued account is genuinely tried first.
	for _, c := range []*model.ProviderConnection{
		{ID: "qoder-busy", Provider: "qoder", AuthType: "oauth", IsActive: true, Priority: 1,
			Data: model.ConnectionData{AccessToken: "dt-busy", BaseURL: busy.URL, ProviderSpecificData: map[string]any{"userId": "user-busy"}}},
		{ID: "qoder-idle", Provider: "qoder", AuthType: "oauth", IsActive: true, Priority: 5,
			Data: model.ConnectionData{AccessToken: "dt-idle", BaseURL: idle.URL, ProviderSpecificData: map[string]any{"userId": "user-idle"}}},
	} {
		if err := database.CreateConnection(c); err != nil {
			t.Fatalf("create %s: %v", c.ID, err)
		}
	}

	body := []byte(`{"model":"qoder/dfmodel","stream":false,"max_tokens":8,"messages":[{"role":"user","content":"ping"}]}`)
	started := time.Now()
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, newLocalRequest("POST", "/v1/chat/completions", bytes.NewReader(body)))

	if w.Code != http.StatusOK {
		t.Fatalf("expected the idle account to serve the request, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Served by the idle account") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("took %s; a 30 s suggested wait must be handed off, not slept through", elapsed)
	}
	if hits := busyHits.Load(); hits != 1 {
		t.Errorf("queued account was contacted %d times, want 1 before it gets parked", hits)
	}
	if idleHits.Load() == 0 {
		t.Error("the idle sibling never received the hand-off")
	}

	// The park must be durable and model-scoped: the next request for this model skips
	// the queued account, while the sibling stays untouched.
	stored, err := database.GetConnection("qoder-busy")
	if err != nil {
		t.Fatal(err)
	}
	if !provider.IsModelLockActive(stored, "dfmodel") {
		t.Errorf("queued account holds no model lock: %+v", stored.Data.ProviderSpecificData)
	}
	sibling, err := database.GetConnection("qoder-idle")
	if err != nil {
		t.Fatal(err)
	}
	if provider.IsModelLockActive(sibling, "dfmodel") {
		t.Error("the account that answered got locked")
	}
}

// TestQoderQueueWaitsWhenNoSiblingExists: with a single account there is nowhere to
// hand off, so the gateway must keep the honest 503 + Retry-After answer instead of
// failing the request outright or parking the only credential it has.
func TestQoderQueueWaitsWhenNoSiblingExists(t *testing.T) {
	srv, database := setupTestServer(t)

	var hits atomic.Int32
	busy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, qoderQueueMessage(t, 30))
	}))
	defer busy.Close()

	provider.SeedQoderCatalogForTest("user-solo", map[string]map[string]any{"dfmodel": {"name": "dfmodel", "max_output_tokens": 4096}})
	conn := &model.ProviderConnection{ID: "qoder-solo", Provider: "qoder", AuthType: "oauth", IsActive: true,
		Data: model.ConnectionData{AccessToken: "dt-solo", BaseURL: busy.URL, ProviderSpecificData: map[string]any{"userId": "user-solo"}}}
	if err := database.CreateConnection(conn); err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"model":"qoder/dfmodel","stream":false,"max_tokens":8,"messages":[{"role":"user","content":"ping"}]}`)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, newLocalRequest("POST", "/v1/chat/completions", bytes.NewReader(body)))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 from the queue, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Retry-After"); got != "30" {
		t.Errorf("Retry-After=%q, want the upstream's suggested wait", got)
	}
	if hits.Load() != 1 {
		t.Errorf("solo account contacted %d times, want one queue report then the 503", hits.Load())
	}
	stored, err := database.GetConnection(conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if provider.IsModelLockActive(stored, "dfmodel") {
		t.Error("parking the only account would leave the model unroutable")
	}
}

// qoderPlanRefusalMessage renders the production-measured plan-refusal
// envelope (2026-11-20): isQueued=true phrased like congestion, but
// serviceAvailable=false with subCode RESOURCE_LIMIT and an empty lane —
// waiting on this account cannot succeed, so the gateway must not spend the
// queue budget on it.
func qoderPlanRefusalMessage(t *testing.T, waitSec int) string {
	t.Helper()
	inner, err := json.Marshal(map[string]any{
		"isQueued": true, "modelKey": "dfmodel", "queueCount": 0, "queueType": "p2",
		"retryAfterSeconds": waitSec, "serviceAvailable": false,
		"subCode": "RESOURCE_LIMIT", "waitTime": waitSec,
	})
	if err != nil {
		t.Fatal(err)
	}
	mid, err := json.Marshal(map[string]any{"code": "10605", "message": string(inner), "subCode": "RESOURCE_LIMIT"})
	if err != nil {
		t.Fatal(err)
	}
	outer, err := json.Marshal(map[string]any{"code": "403", "message": string(mid)})
	if err != nil {
		t.Fatal(err)
	}
	return string(outer)
}

// TestQoderPlanRefusalFailsFastWithoutSibling: the plan refusal arrives on the
// only account — the honest answer is an immediate, distinguishable 503. The
// old behavior slept through 40 attempts of "queue" that was never a queue.
func TestQoderPlanRefusalFailsFastWithoutSibling(t *testing.T) {
	srv, database := setupTestServer(t)

	var hits atomic.Int32
	refuser := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, qoderPlanRefusalMessage(t, 26))
	}))
	defer refuser.Close()

	provider.SeedQoderCatalogForTest("user-solo-refuse", map[string]map[string]any{"dfmodel": {"name": "dfmodel", "max_output_tokens": 4096}})
	conn := &model.ProviderConnection{ID: "qoder-solo-refuse", Provider: "qoder", AuthType: "oauth", IsActive: true,
		Data: model.ConnectionData{AccessToken: "dt-solo", BaseURL: refuser.URL, ProviderSpecificData: map[string]any{"userId": "user-solo-refuse"}}}
	if err := database.CreateConnection(conn); err != nil {
		t.Fatal(err)
	}

	for _, stream := range []bool{false, true} {
		name := "non-streaming"
		if stream {
			name = "streaming"
		}
		t.Run(name, func(t *testing.T) {
			body := []byte(`{"model":"qoder/dfmodel","stream":` + strconv.FormatBool(stream) + `,"max_tokens":8,"messages":[{"role":"user","content":"ping"}]}`)
			started := time.Now()
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, newLocalRequest("POST", "/v1/chat/completions", bytes.NewReader(body)))

			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("want 503, got %d: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "qoder_resource_limit") {
				t.Errorf("client must see a type distinct from qoder_queue_wait: %s", w.Body.String())
			}
			if got := w.Header().Get("Retry-After"); got != "26" {
				t.Errorf("Retry-After=%q, want the advisory upstream wait", got)
			}
			if elapsed := time.Since(started); elapsed > 3*time.Second {
				t.Errorf("took %s; a plan refusal must fail fast, not sleep through the queue budget", elapsed)
			}
		})
	}
	if hits.Load() != 2 {
		t.Errorf("refusing account contacted %d times, want exactly one attempt per client request", hits.Load())
	}
}

// TestQoderPlanRefusalYieldsToIdleSibling: refusal parking is the same
// mechanism as congestion yielding — a healthy sibling answers immediately,
// and the plan-refused account is not retried per-request by other clients.
func TestQoderPlanRefusalYieldsToIdleSibling(t *testing.T) {
	srv, database := setupTestServer(t)

	var busyHits, idleHits atomic.Int32
	busy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		busyHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, qoderPlanRefusalMessage(t, 26))
	}))
	defer busy.Close()

	frame := qoderTestFrame(`{"id":"chatcmpl-refusal-handoff","choices":[{"index":0,"delta":{"role":"assistant","content":"served by sibling"}}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`) + "data: [DONE]\n\n"
	idle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idleHits.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, frame)
	}))
	defer idle.Close()

	provider.SeedQoderCatalogForTest("user-refuse", map[string]map[string]any{"dfmodel": {"name": "dfmodel", "max_output_tokens": 4096}})
	provider.SeedQoderCatalogForTest("user-serve", map[string]map[string]any{"dfmodel": {"name": "dfmodel", "max_output_tokens": 4096}})
	for _, c := range []*model.ProviderConnection{
		{ID: "qoder-refuse", Provider: "qoder", AuthType: "oauth", IsActive: true, Priority: 1,
			Data: model.ConnectionData{AccessToken: "dt-refuse", BaseURL: busy.URL, ProviderSpecificData: map[string]any{"userId": "user-refuse"}}},
		{ID: "qoder-serve", Provider: "qoder", AuthType: "oauth", IsActive: true, Priority: 5,
			Data: model.ConnectionData{AccessToken: "dt-serve", BaseURL: idle.URL, ProviderSpecificData: map[string]any{"userId": "user-serve"}}},
	} {
		if err := database.CreateConnection(c); err != nil {
			t.Fatalf("create %s: %v", c.ID, err)
		}
	}

	body := []byte(`{"model":"qoder/dfmodel","stream":false,"max_tokens":8,"messages":[{"role":"user","content":"ping"}]}`)
	started := time.Now()
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, newLocalRequest("POST", "/v1/chat/completions", bytes.NewReader(body)))

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "served by sibling") {
		t.Fatalf("expected the healthy sibling to answer, got %d: %s", w.Code, w.Body.String())
	}
	if time.Since(started) > 3*time.Second {
		t.Error("hand-off must be immediate; a refusal wait cannot be slept through")
	}
	if busyHits.Load() != 1 {
		t.Errorf("refused account contacted %d times, want 1 before parking", busyHits.Load())
	}
	stored, err := database.GetConnection("qoder-refuse")
	if err != nil {
		t.Fatal(err)
	}
	if !provider.IsModelLockActive(stored, "dfmodel") {
		t.Error("plan-refused account holds no model lock; every client would re-probe it")
	}
}
