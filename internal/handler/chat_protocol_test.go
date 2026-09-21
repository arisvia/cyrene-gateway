package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
	"github.com/arisvia/cyrene-gateway/internal/translator"
)

func TestProtocolComboProtocolParity(t *testing.T) {
	cases := []struct {
		format   string
		field    string
		response string
	}{
		{"openai", "messages", `{"id":"audit","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`},
		{"anthropic", "system", `{"id":"audit","type":"message","role":"assistant","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`},
		{"gemini", "contents", `{"candidates":[{"content":{"role":"model","parts":[{"text":"hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"totalTokenCount":18}}`},
		{"responses", "input", `{"id":"audit","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}`},
	}
	for _, tc := range cases {
		for _, combo := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/combo=%t", tc.format, combo), func(t *testing.T) {
				captured := make(chan map[string]any, 1)
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					captured <- body
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, tc.response)
				}))
				t.Cleanup(upstream.Close)
				id := "audit-chain-" + tc.format
				provider.Registry[id] = provider.ProviderInfo{ID: id, Name: id, BaseURL: upstream.URL, APIType: tc.format, AuthType: "api-key"}
				t.Cleanup(func() { delete(provider.Registry, id) })
				srv, database := setupTestServer(t)
				if err := database.CreateConnection(&model.ProviderConnection{ID: id, Provider: id, AuthType: "api-key", IsActive: true, Data: model.ConnectionData{APIKey: "audit-dummy"}}); err != nil {
					t.Fatal(err)
				}
				modelName := id + "/model"
				if combo {
					if err := database.CreateCombo(&model.Combo{ID: "audit-combo", Name: "audit-combo", Kind: "fallback", Models: []string{modelName}}); err != nil {
						t.Fatal(err)
					}
					modelName = "audit-combo"
				}
				body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"system","content":"system instruction"},{"role":"user","content":"hello"}],"max_tokens":64}`, modelName)
				r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				srv.Handler.ServeHTTP(w, r)
				if w.Code != http.StatusOK {
					t.Fatalf("unexpected HTTP %d: %s", w.Code, w.Body.String())
				}
				select {
				case sent := <-captured:
					if _, ok := sent[tc.field]; !ok {
						t.Errorf("native %s upstream missing %q: %v", tc.format, tc.field, sent)
					}
				default:
					t.Fatal("upstream not called")
				}
				var result struct {
					Choices []struct {
						Message struct {
							Content string `json:"content"`
						} `json:"message"`
					} `json:"choices"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if len(result.Choices) != 1 || result.Choices[0].Message.Content != "hello" {
					t.Errorf("expected Chat response with hello, got %s", w.Body.String())
				}
			})
		}
	}
}

func TestProtocolAggregateToolCalls(t *testing.T) {
	srv, _ := setupTestServer(t)
	stream := "data: " + `{"id":"audit","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"weather","arguments":"{\"city\":"}}]},"finish_reason":null}]}` + "\n\n" +
		"data: " + `{"id":"audit","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Tokyo\"}"}}]},"finish_reason":null}]}` + "\n\n" +
		"data: " + `{"id":"audit","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}
	w := httptest.NewRecorder()
	srv.proxyResponse(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, false, true, translator.FormatOpenAI, "audit", &usageContext{Model: "audit", StartedAt: time.Now(), Status: 200})
	var result struct {
		Choices []struct {
			Message struct {
				ToolCalls []json.RawMessage `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Choices) != 1 || len(result.Choices[0].Message.ToolCalls) != 1 || result.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("tool call and finish_reason lost: %s", w.Body.String())
	}
}

func TestProtocolAggregateResponsesTools(t *testing.T) {
	srv, _ := setupTestServer(t)
	var stream strings.Builder
	for _, event := range []string{
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_a","call_id":"call_a","name":"weather"}}`,
		`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_b","call_id":"call_b","name":"weather"}}`,
		`{"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"city\":\"Paris\"}"}`,
		`{"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"city\":"}`,
		`{"type":"response.function_call_arguments.delta","output_index":0,"delta":"\"Tokyo\"}"}`,
		`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}}`,
	} {
		fmt.Fprintf(&stream, "data: %s\n\n", event)
	}
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream.String()))}
	w := httptest.NewRecorder()
	srv.proxyResponse(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, false, true, translator.FormatResponses, "model", &usageContext{StartedAt: time.Now(), Status: 200})
	var result struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Choices) != 1 || len(result.Choices[0].Message.ToolCalls) != 2 || result.Choices[0].FinishReason != "tool_calls" || result.Usage.TotalTokens != 18 {
		t.Fatalf("incorrect aggregation: %s", w.Body.String())
	}
	for i, call := range result.Choices[0].Message.ToolCalls {
		if call.ID != []string{"call_a", "call_b"}[i] || call.Function.Name != "weather" || call.Function.Arguments != []string{`{"city":"Tokyo"}`, `{"city":"Paris"}`}[i] {
			t.Fatalf("incorrect tool call: %+v", call)
		}
	}
}

func TestProtocolAggregateTruncation(t *testing.T) {
	for _, finish := range []string{"length", "content_filter", ""} {
		t.Run(finish, func(t *testing.T) {
			srv, _ := setupTestServer(t)
			data := fmt.Sprintf("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"},\"finish_reason\":%q}]}\n\n", finish)
			resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(data))}
			w := httptest.NewRecorder()
			srv.proxyResponse(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, false, true, translator.FormatOpenAI, "model", &usageContext{StartedAt: time.Now()})
			if finish == "" {
				if w.Code != http.StatusBadGateway {
					t.Fatalf("unexpected success for incomplete stream: %s", w.Body.String())
				}
			} else if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"finish_reason":"`+finish+`"`) {
				t.Fatalf("lost finish reason: HTTP %d %s", w.Code, w.Body.String())
			}
		})
	}
}

type protocolBrokenReader struct{}

func (protocolBrokenReader) Read([]byte) (int, error) {
	return 0, errors.New("audit upstream disconnected")
}

func TestProtocolNativeTerminalEvent(t *testing.T) {
	for _, tc := range []struct {
		format   translator.Format
		terminal string
	}{
		{translator.FormatAnthropic, `{"type":"message_stop"}`},
		{translator.FormatResponses, `{"type":"response.completed","response":{"id":"audit","status":"completed","output":[]}}`},
	} {
		t.Run(string(tc.format), func(t *testing.T) {
			srv, _ := setupTestServer(t)
			stream := "data: " + tc.terminal + "\n\n"
			resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(io.MultiReader(strings.NewReader(stream), protocolBrokenReader{}))}
			w := httptest.NewRecorder()
			srv.proxyResponse(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, true, true, tc.format, "audit", &usageContext{Model: "audit", StartedAt: time.Now(), Status: 200})
			if strings.Contains(w.Body.String(), "upstream_error") {
				t.Fatalf("read past protocol terminal event: %s", w.Body.String())
			}
		})
	}
}

func TestProtocolAggregateReadFailure(t *testing.T) {
	srv, _ := setupTestServer(t)
	stream := "data: " + `{"choices":[{"delta":{"content":"partial"}}]}` + "\n\n"
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(io.MultiReader(strings.NewReader(stream), protocolBrokenReader{}))}
	w := httptest.NewRecorder()
	srv.proxyResponse(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, false, true, translator.FormatOpenAI, "audit", &usageContext{Model: "audit", StartedAt: time.Now(), Status: 200})
	if w.Code < 400 {
		t.Fatalf("upstream failure became success: HTTP %d %s", w.Code, w.Body.String())
	}
}

func TestProtocolCodexEndpoint(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if r.URL.Path != "/responses" || body["input"] == nil || body["messages"] != nil || body["stream"] != true || body["store"] != false {
			t.Errorf("incorrect Codex request: %s %v", r.URL.Path, body)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing upstream auth")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"codex-answer\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":11,\"output_tokens\":7,\"total_tokens\":18}}}\n\n")
	}))
	t.Cleanup(upstream.Close)
	srv, database := setupTestServer(t)
	if err := database.CreateConnection(&model.ProviderConnection{ID: "codex-test", Provider: "codex", AuthType: "api-key", IsActive: true, Data: model.ConnectionData{APIKey: "test-key", BaseURL: upstream.URL + "/responses"}}); err != nil {
		t.Fatal(err)
	}
	for _, stream := range []bool{false, true} {
		w := httptest.NewRecorder()
		body := fmt.Sprintf(`{"model":"codex/model","input":"hello","stream":%t}`, stream)
		srv.Handler.ServeHTTP(w, httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body)))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "codex-answer") || !strings.Contains(w.Body.String(), `"total_tokens":18`) {
			t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
		}
		if stream && !strings.Contains(w.Body.String(), "event: response.completed") {
			t.Fatalf("missing final Responses event: %s", w.Body.String())
		}
	}
}

func TestProtocolStreamFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		format translator.Format
		data   string
	}{
		{"openai", translator.FormatOpenAI, `{"error":{"message":"upstream failed","type":"server_error"}}`},
		{"anthropic", translator.FormatAnthropic, `{"type":"error","error":{"message":"upstream failed","type":"overloaded_error"}}`},
		{"gemini", translator.FormatGemini, `{"error":{"code":503,"message":"upstream failed","status":"UNAVAILABLE"}}`},
		{"responses", translator.FormatResponses, `{"type":"response.failed","response":{"status":"failed","error":{"message":"upstream failed","code":"server_error"}}}`},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				srv, database := setupTestServer(t)
				resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + tc.data + "\n\n"))}
				w := httptest.NewRecorder()
				srv.proxyResponse(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, stream, true, tc.format, "model", &usageContext{Provider: "failure-test", Model: "model", StartedAt: time.Now(), Status: 200})
				if !strings.Contains(w.Body.String(), "upstream failed") || (!stream && w.Code != http.StatusBadGateway) {
					t.Fatalf("lost failure: HTTP %d %s", w.Code, w.Body.String())
				}
				details, err := database.GetRequestDetails(db.RequestDetailFilter{Provider: "failure-test", Page: 1, PageSize: 10})
				if err != nil {
					t.Fatal(err)
				}
				if details.Pagination.TotalItems != 0 {
					t.Fatal("failed stream was recorded as successful")
				}
			})
		}
	}
}

func TestProtocolDownstreamStreamErrors(t *testing.T) {
	for _, inbound := range []string{"messages", "gemini", "responses"} {
		t.Run(inbound, func(t *testing.T) {
			srv, _ := setupTestServer(t)
			w := httptest.NewRecorder()
			var adapter http.ResponseWriter
			var finish func()
			switch inbound {
			case "messages":
				a := newAnthropicResponseAdapter(w, true, "model")
				adapter, finish = a, a.Finish
			case "gemini":
				a := newGeminiResponseAdapter(w, true, "model")
				adapter, finish = a, a.Finish
			case "responses":
				a := newResponsesResponseAdapter(w, true, "model")
				adapter, finish = a, a.Finish
			}
			resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"error\":{\"message\":\"stream-failed\",\"type\":\"upstream_error\"}}\n\n"))}
			srv.proxyResponse(adapter, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, true, true, translator.FormatOpenAI, "model", &usageContext{StartedAt: time.Now()})
			finish()
			body := w.Body.String()
			if !strings.Contains(body, "stream-failed") || strings.Contains(body, `"end_turn"`) || strings.Contains(body, `"finishReason":"STOP"`) || strings.Contains(body, "event: response.completed") {
				t.Fatalf("failure became success: %s", body)
			}
		})
	}
}

func TestProtocolDownstreamTruncation(t *testing.T) {
	for _, target := range []string{"messages", "gemini", "responses"} {
		t.Run(target, func(t *testing.T) {
			var translate func([]byte) ([]byte, bool, error)
			var expected string
			switch target {
			case "messages":
				translate = translator.NewOpenAIToClaudeSSETranslator("model").TranslateChunk
				expected = `"stop_reason":"max_tokens"`
			case "gemini":
				translate = translator.NewOpenAIToGeminiSSETranslator("model").TranslateChunk
				expected = `"finishReason":"MAX_TOKENS"`
			case "responses":
				translate = translator.NewOpenAIToResponsesSSETranslator("model").TranslateChunk
				expected = "event: response.incomplete"
			}
			var output strings.Builder
			for _, data := range []string{`{"choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":"length"}]}`, `{"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`, `[DONE]`} {
				payload, _, err := translate([]byte(data))
				if err != nil {
					t.Fatal(err)
				}
				output.Write(payload)
			}
			if !strings.Contains(output.String(), expected) {
				t.Fatalf("truncation lost: %s", output.String())
			}
			if target == "gemini" && strings.Contains(output.String(), `"finishReason":"STOP"`) {
				t.Fatalf("false success after truncation: %s", output.String())
			}
		})
	}
}

func TestProtocolMalformedStream(t *testing.T) {
	for _, format := range []translator.Format{translator.FormatOpenAI, translator.FormatAnthropic, translator.FormatGemini, translator.FormatResponses} {
		for _, body := range []string{`{`, `null`, `[]`} {
			if _, _, err := translator.NewSSETranslator(format, "model").TranslateChunk([]byte(body)); err == nil {
				t.Fatalf("malformed %s payload silently ignored: %s", format, body)
			}
		}
	}
}

func TestProtocolEndpointMatrix(t *testing.T) {
	fixtures := []struct {
		format, field, response string
		events                  []string
	}{
		{"openai", "messages", `{"id":"chat_1","choices":[{"index":0,"message":{"role":"assistant","content":"protocol-answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`, []string{
			`{"id":"chat_1","choices":[{"index":0,"delta":{"content":"protocol-answer"},"finish_reason":"stop"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`, `[DONE]`,
		}},
		{"anthropic", "messages", `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"protocol-answer"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`, []string{
			`{"type":"message_start","message":{"id":"msg_1","role":"assistant","content":[],"usage":{"input_tokens":11,"output_tokens":0}}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"protocol-answer"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`, `{"type":"message_stop"}`,
		}},
		{"gemini", "contents", `{"candidates":[{"content":{"role":"model","parts":[{"text":"protocol-answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"totalTokenCount":18}}`, []string{
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"protocol-answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"totalTokenCount":18}}`,
		}},
		{"responses", "input", `{"id":"resp_1","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"protocol-answer"}]}],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}`, []string{
			`{"type":"response.output_text.delta","output_index":0,"delta":"protocol-answer"}`,
			`{"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}}`,
		}},
	}
	for _, fixture := range fixtures {
		for _, inbound := range []string{"chat", "messages", "responses", "gemini"} {
			for _, stream := range []bool{false, true} {
				for _, combo := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s-to-%s/stream=%t/combo=%t", inbound, fixture.format, stream, combo), func(t *testing.T) {
						upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							var body map[string]any
							if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
								t.Error(err)
							}
							if body[fixture.field] == nil {
								t.Errorf("missing native %s: %v", fixture.field, body)
							}
							if stream {
								w.Header().Set("Content-Type", "text/event-stream")
								for _, event := range fixture.events {
									fmt.Fprintf(w, "data: %s\n\n", event)
								}
							} else {
								w.Header().Set("Content-Type", "application/json")
								io.WriteString(w, fixture.response)
							}
						}))
						t.Cleanup(upstream.Close)
						id := "matrix-" + fixture.format
						provider.Registry[id] = provider.ProviderInfo{ID: id, Name: id, BaseURL: upstream.URL, APIType: fixture.format, AuthType: "api-key"}
						t.Cleanup(func() { delete(provider.Registry, id) })
						srv, database := setupTestServer(t)
						if err := database.CreateConnection(&model.ProviderConnection{ID: id, Provider: id, AuthType: "api-key", IsActive: true, Data: model.ConnectionData{APIKey: "test-key"}}); err != nil {
							t.Fatal(err)
						}
						modelName := id + "/model"
						if combo {
							if err := database.CreateCombo(&model.Combo{ID: "matrix", Name: "matrix", Kind: "fallback", Models: []string{modelName}}); err != nil {
								t.Fatal(err)
							}
							modelName = "matrix"
						}
						path := "/v1/chat/completions"
						body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hello"}],"max_tokens":64,"stream":%t}`, modelName, stream)
						switch inbound {
						case "messages":
							path = "/v1/messages"
						case "responses":
							path = "/v1/responses"
							body = fmt.Sprintf(`{"model":%q,"input":"hello","stream":%t}`, modelName, stream)
						case "gemini":
							path = "/v1beta/models/" + modelName + ":generateContent"
							if stream {
								path = "/v1beta/models/" + modelName + ":streamGenerateContent?alt=sse"
							}
							body = `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`
						}
						w := httptest.NewRecorder()
						srv.Handler.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(body)))
						if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "protocol-answer") || strings.Contains(w.Body.String(), `"error"`) {
							t.Fatalf("response HTTP %d: %s", w.Code, w.Body.String())
						}
						details, err := database.GetRequestDetails(db.RequestDetailFilter{Provider: id, Page: 1, PageSize: 10})
						if err != nil {
							t.Fatal(err)
						}
						if details.Pagination.TotalItems != 1 {
							t.Fatalf("expected usage record, got %d", details.Pagination.TotalItems)
						}
					})
				}
			}
		}
	}
}

func TestProtocolNativeStreamUsage(t *testing.T) {
	cases := []struct {
		format translator.Format
		stream string
	}{
		{translator.FormatOpenAI, "data: " + `{"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}` + "\n\ndata: [DONE]\n\n"},
		{translator.FormatAnthropic, "data: " + `{"type":"message_start","message":{"id":"audit","role":"assistant","usage":{"input_tokens":11,"output_tokens":0}}}` + "\n\ndata: " + `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}` + "\n\ndata: " + `{"type":"message_stop"}` + "\n\n"},
		{translator.FormatGemini, "data: " + `{"candidates":[{"content":{"parts":[{"text":"hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"totalTokenCount":18}}` + "\n\n"},
		{translator.FormatResponses, "data: " + `{"type":"response.completed","response":{"id":"audit","status":"completed","output":[],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}}` + "\n\n"},
	}
	for _, tc := range cases {
		t.Run(string(tc.format), func(t *testing.T) {
			srv, database := setupTestServer(t)
			resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(tc.stream))}
			w := httptest.NewRecorder()
			srv.proxyResponse(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, true, true, tc.format, "audit", &usageContext{Provider: "audit", Model: "audit", StartedAt: time.Now(), Status: 200})
			details, err := database.GetRequestDetails(db.RequestDetailFilter{Provider: "audit", Page: 1, PageSize: 10})
			if err != nil {
				t.Fatal(err)
			}
			if details.Pagination.TotalItems != 1 {
				t.Fatalf("native stream usage 11+7 not recorded, details=%d", details.Pagination.TotalItems)
			}
			var recorded struct {
				PromptTokens     int `json:"promptTokens"`
				CompletionTokens int `json:"completionTokens"`
			}
			if err := json.Unmarshal(details.Details[0], &recorded); err != nil {
				t.Fatal(err)
			}
			if recorded.PromptTokens != 11 || recorded.CompletionTokens != 7 {
				t.Fatalf("incorrect usage: %+v", recorded)
			}
		})
	}
}
