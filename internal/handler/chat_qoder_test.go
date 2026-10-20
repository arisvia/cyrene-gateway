package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/cache"
	"github.com/arisvia/cyrene-gateway/internal/db"
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
