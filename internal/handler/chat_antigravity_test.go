package handler

import (
	"bytes"
	"encoding/json"
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
	"github.com/arisvia/cyrene-gateway/internal/usage"
)

func setupAntigravityReviewUpstream(t *testing.T, fixture string) (*Server, <-chan map[string]any) {
	t.Helper()
	received := make(chan map[string]any, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1internal:streamGenerateContent" || r.URL.Query().Get("alt") != "sse" {
			t.Errorf("not routed through Antigravity handler: %s", r.URL)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		received <- body
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, fixture)
	}))
	t.Cleanup(upstream.Close)
	srv, database := setupTestServer(t)
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "ag-review", Provider: "antigravity", AuthType: "oauth", IsActive: true,
		Data: model.ConnectionData{AccessToken: "test-only", BaseURL: upstream.URL,
			ProviderSpecificData: map[string]any{"projectId": "review-project"}},
	}); err != nil {
		t.Fatal(err)
	}
	return srv, received
}

func TestAntigravityReviewDeveloperInstructionsEndToEnd(t *testing.T) {
	for _, tc := range []struct {
		path, body string
		want       []string
	}{
		{"/v1/responses", `{"model":"antigravity/claude-sonnet-4-6","instructions":"First line.\n保留指令。","input":[{"role":"developer","content":[{"type":"input_text","text":"Another instruction."}]},{"role":"user","content":"hello"}]}`, []string{"First line.\n保留指令。", "Another instruction."}},
		{"/v1/chat/completions", `{"model":"antigravity/claude-sonnet-4-6","messages":[{"role":"system","content":"System text."},{"role":"developer","content":[{"type":"text","text":"First line.\n"},{"type":"text","text":"保留指令。"}]},{"role":"developer","content":"Another instruction."},{"role":"user","content":"hello"}]}`, []string{"System text.", "First line.\n保留指令。", "Another instruction."}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			srv, received := setupAntigravityReviewUpstream(t, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Answer\"}]}}]}\n\n")
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body)))
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			var body map[string]any
			select {
			case body = <-received:
			default:
				t.Fatal("no upstream request")
			}
			inner := body["request"].(map[string]any)
			system, ok := inner["systemInstruction"].(map[string]any)
			if !ok {
				t.Fatalf("missing systemInstruction: %v", inner)
			}
			parts := system["parts"].([]any)
			if len(parts) != len(tc.want) {
				t.Errorf("instruction parts=%v want=%v", parts, tc.want)
			} else {
				for i, want := range tc.want {
					if parts[i].(map[string]any)["text"] != want {
						t.Errorf("instruction %d changed: %v", i, parts[i])
					}
				}
			}
			contents := inner["contents"].([]any)
			if len(contents) != 1 || contents[0].(map[string]any)["role"] != "user" {
				t.Errorf("instruction roles leaked into contents: %v", contents)
			}
		})
	}
}

func TestAntigravityReviewResponsesUsageEndToEnd(t *testing.T) {
	for _, tc := range []struct {
		name, metadata     string
		known              bool
		prompt, completion int
	}{
		{"missing", "", false, 4, 4},
		{"null", `"usageMetadata":null`, false, 4, 4},
		{"empty", `"usageMetadata":{}`, false, 4, 4},
		{"zero", `"usageMetadata":{"promptTokenCount":0,"candidatesTokenCount":0,"totalTokenCount":0}`, true, 0, 0},
		{"output-zero", `"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":0,"totalTokenCount":5}`, true, 5, 0},
		{"known", `"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"totalTokenCount":18}`, true, 11, 7},
	} {
		for _, stream := range []bool{false, true} {
			for _, nested := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t/nested=%t", tc.name, stream, nested), func(t *testing.T) {
					fixture := "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Answer with text\"}]},\"finishReason\":\"STOP\"}]}}\n\n"
					if tc.metadata != "" {
						metadata := "{" + tc.metadata + "}"
						if nested {
							metadata = `{"response":` + metadata + `}`
						}
						fixture += "data: " + metadata + "\n\n"
					}
					fixture += "data: {}\n\ndata: [DONE]\n\n"
					srv, _ := setupAntigravityReviewUpstream(t, fixture)
					body := fmt.Sprintf(`{"model":"antigravity/claude-sonnet-4-6","input":"A sixteen prompt","stream":%t}`, stream)
					w := httptest.NewRecorder()
					srv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body)))
					if w.Code != http.StatusOK {
						t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
					}
					var response map[string]any
					if stream {
						for _, line := range strings.Split(w.Body.String(), "\n") {
							data, done, ok := translator.ParseSSEDataLineString(line)
							if !ok || done {
								continue
							}
							var event map[string]any
							if err := json.Unmarshal([]byte(data), &event); err != nil {
								t.Fatal(err)
							}
							if event["type"] == "response.completed" {
								response, _ = event["response"].(map[string]any)
							}
						}
					} else if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
					if response["output_text"] != "Answer with text" {
						t.Fatalf("lost output: %v", response)
					}
					u, hasUsage := response["usage"].(map[string]any)
					if hasUsage != tc.known {
						t.Errorf("usage presence=%t want=%t: %v", hasUsage, tc.known, response)
					} else if tc.known && (u["input_tokens"] != float64(tc.prompt) || u["output_tokens"] != float64(tc.completion) || u["total_tokens"] != float64(tc.prompt+tc.completion)) {
						t.Errorf("real usage changed: %v", u)
					}
					entries, err := srv.DB.GetUsageHistory(db.UsageFilter{Provider: "antigravity"})
					if err != nil || len(entries) != 1 {
						t.Fatalf("entries=%v err=%v", entries, err)
					}
					entry := entries[0]
					if entry.Endpoint != "/v1/responses" || entry.PromptTokens != tc.prompt || entry.CompletionTokens != tc.completion || strings.Contains(entry.Meta, `"usage_missing":true`) == tc.known {
						t.Errorf("incorrect accounting: %+v", entry)
					}
					details, err := srv.DB.GetRequestDetails(db.RequestDetailFilter{Provider: "antigravity", Page: 1, PageSize: 10})
					if err != nil || len(details.Details) != 1 {
						t.Fatalf("details=%v err=%v", details, err)
					}
					var detail map[string]any
					if err := json.Unmarshal(details.Details[0], &detail); err != nil {
						t.Fatal(err)
					}
					if detail["usageKnown"] != tc.known {
						t.Errorf("usageKnown=%v want=%t", detail["usageKnown"], tc.known)
					}
				})
			}
		}
	}
}

func TestAntigravityUpstreamUsageAndMissingAccounting(t *testing.T) {
	for _, mode := range []string{"json", "aggregate-sse", "stream"} {
		for _, nested := range []bool{false, true} {
			for _, counts := range []string{"known", "no-total", "missing"} {
				t.Run(fmt.Sprintf("%s/nested=%t/%s", mode, nested, counts), func(t *testing.T) {
					srv, database := setupTestServer(t)
					candidates := `"candidates":[{"content":{"parts":[{"text":"Answer with text"},{"thought":true,"text":"Thinking about tools"},{"functionCall":{"name":"weather","args":{"city":"Paris"}}}]},"finishReason":"STOP"}]`
					metadata := `"usageMetadata":{"promptTokenCount":101,"candidatesTokenCount":17,"cachedContentTokenCount":13,"thoughtsTokenCount":9`
					if counts == "known" {
						metadata += `,"totalTokenCount":127`
					}
					metadata += `}`
					if counts == "missing" {
						metadata = ""
					}
					wrap := func(fields string) string {
						if nested {
							return `{"response":{` + fields + `}}`
						}
						return `{` + fields + `}`
					}
					var body string
					if mode == "json" {
						fields := candidates
						if metadata != "" {
							fields += "," + metadata
						}
						body = wrap(fields)
					} else {
						// Usage arrives in a separate terminal frame without candidates.
						body = "data: " + wrap(candidates) + "\n\n"
						if metadata != "" {
							body += "data: " + wrap(metadata) + "\n\n"
						}
						body += "data: [DONE]\n\n"
					}
					resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
					uc := &usageContext{StartedAt: time.Now(), Provider: "antigravity", Model: "gemini-test", Status: 200, Prompt: "A sixteen prompt"}
					w := httptest.NewRecorder()
					if mode == "stream" {
						srv.proxyAntigravityStreaming(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, "gemini-test", uc, nil)
					} else {
						srv.proxyAntigravityNonStreaming(w, resp, "gemini-test", uc, nil)
					}
					if w.Code != 200 || !strings.Contains(w.Body.String(), "weather") || !strings.Contains(w.Body.String(), "Thinking about tools") || uc.Response != "Answer with text" {
						t.Errorf("lost content/tool/thought: HTTP %d %s", w.Code, w.Body.String())
					}
					var responseUsage usage.Usage
					if mode == "stream" {
						for _, line := range strings.Split(w.Body.String(), "\n") {
							data, done, ok := translator.ParseSSEDataLineString(line)
							if ok && !done {
								if u := usage.ExtractFromOpenAI([]byte(data)); u.TotalTokens > 0 {
									responseUsage = u
								}
							}
						}
					} else {
						responseUsage = usage.ExtractFromOpenAI(w.Body.Bytes())
					}
					known := counts != "missing"
					wantUsage := usage.Usage{PromptTokens: 101, CompletionTokens: 26, TotalTokens: 127, CachedTokens: 13, ReasoningTokens: 9, Known: true}
					if known && responseUsage != wantUsage {
						t.Errorf("response usage=%+v, want %+v", responseUsage, wantUsage)
					}
					if !known && strings.Contains(w.Body.String(), `"usage"`) {
						t.Errorf("invented response usage: %s", w.Body.String())
					}
					entries, err := database.GetUsageHistory(db.UsageFilter{Provider: "antigravity"})
					if err != nil || len(entries) != 1 {
						t.Fatalf("usage entries=%v err=%v", entries, err)
					}
					entry := entries[0]
					wantPrompt, wantCompletion := 101, 26
					if !known {
						wantPrompt, wantCompletion = 4, 4
					}
					if entry.PromptTokens != wantPrompt || entry.CompletionTokens != wantCompletion {
						t.Errorf("stored tokens=%d/%d, want %d/%d", entry.PromptTokens, entry.CompletionTokens, wantPrompt, wantCompletion)
					}
					var meta map[string]any
					if err := json.Unmarshal([]byte(entry.Meta), &meta); err != nil {
						t.Fatal(err)
					}
					if known {
						if meta["cached_tokens"] != float64(13) || meta["reasoning_tokens"] != float64(9) || meta["usage_missing"] == true {
							t.Errorf("stored metadata=%v", meta)
						}
					} else if meta["usage_missing"] != true || meta["reasoning_tokens"] != nil {
						t.Errorf("missing usage not unknown: %v", meta)
					}
					details, err := database.GetRequestDetails(db.RequestDetailFilter{Provider: "antigravity", Page: 1, PageSize: 10})
					if err != nil || len(details.Details) != 1 {
						t.Fatalf("details=%v err=%v", details, err)
					}
					var detail map[string]any
					if err := json.Unmarshal(details.Details[0], &detail); err != nil {
						t.Fatal(err)
					}
					if detail["usageKnown"] != known {
						t.Errorf("usageKnown=%v want %t", detail["usageKnown"], known)
					}
				})
			}
		}
	}
}

func TestAntigravityToolAndThoughtOnlyUsage(t *testing.T) {
	for _, part := range []string{
		`{"thought":true,"text":"Thinking only now"}`,
		`{"functionCall":{"name":"search","args":{"q":"Go"}}}`,
	} {
		for _, stream := range []bool{false, true} {
			for _, known := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t/known=%t", part, stream, known), func(t *testing.T) {
					srv, database := setupTestServer(t)
					// Nested candidates with top-level metadata is also an upstream format.
					body := `{"response":{"candidates":[{"content":{"parts":[` + part + `]}}]}`
					if known {
						body += `,"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":2,"thoughtsTokenCount":7,"totalTokenCount":20}`
					}
					body += `}`
					if stream {
						body = "data: " + body + "\n\ndata: [DONE]\n\n"
					}
					w := httptest.NewRecorder()
					resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
					uc := &usageContext{Provider: "antigravity", Model: "model", StartedAt: time.Now(), Status: 200, Prompt: "test prompt"}
					if stream {
						srv.proxyAntigravityStreaming(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, "model", uc, nil)
					} else {
						srv.proxyAntigravityNonStreaming(w, resp, "model", uc, nil)
					}
					entries, err := database.GetUsageHistory(db.UsageFilter{Provider: "antigravity"})
					if err != nil || len(entries) != 1 {
						t.Fatalf("entries=%v err=%v", entries, err)
					}
					entry := entries[0]
					if known {
						if entry.PromptTokens != 11 || entry.CompletionTokens != 9 || !strings.Contains(w.Body.String(), `"total_tokens":20`) || !strings.Contains(w.Body.String(), `"reasoning_tokens":7`) {
							t.Errorf("lost real usage: entry=%+v response=%s", entry, w.Body.String())
						}
					} else {
						if !strings.Contains(entry.Meta, `"usage_missing":true`) || strings.Contains(w.Body.String(), `"usage"`) {
							t.Errorf("invented usage: entry=%+v response=%s", entry, w.Body.String())
						}
						if strings.Contains(part, "thought") && (uc.Response != "Thinking only now" || entry.CompletionTokens != 4) {
							t.Errorf("thought-only fallback lost: entry=%+v response=%q", entry, uc.Response)
						}
					}
				})
			}
		}
	}
}

func TestAntigravityThinkingTierRouting(t *testing.T) {
	cases := []struct {
		model          string
		effort         string
		expectedTarget string
		expectedTier   string
	}{
		// 1. gemini-3.8-flash default medium
		{
			model:          "antigravity/gemini-3.8-flash",
			effort:         "",
			expectedTarget: "gemini-3.8-flash-medium",
			expectedTier:   "medium",
		},
		// 2. gemini-3.8-flash explicit high
		{
			model:          "ag/gemini-3.8-flash",
			effort:         "high",
			expectedTarget: "gemini-3.8-flash-high",
			expectedTier:   "high",
		},
		// 3. gemini-3.8-flash explicit low
		{
			model:          "gemini-3.8-flash",
			effort:         "low",
			expectedTarget: "gemini-3.8-flash-low",
			expectedTier:   "low",
		},
		// 4. catalog-miss family keeps the requested tier as suffix
		{
			model:          "antigravity/gemini-3.5-flash",
			effort:         "high",
			expectedTarget: "gemini-3.5-flash-high",
			expectedTier:   "high",
		},
		// 5. dead extra-low mapping removed: generic suffix, no family table
		{
			model:          "antigravity/gemini-3.5-flash",
			effort:         "low",
			expectedTarget: "gemini-3.5-flash-low",
			expectedTier:   "low",
		},
		// 6. claude families get the same tier suffix treatment
		{
			model:          "antigravity/claude-sonnet-5-5",
			effort:         "low",
			expectedTarget: "claude-sonnet-5-5-low",
			expectedTier:   "low",
		},
		// 7. explicit model suffix preservation
		{
			model:          "gemini-3.8-flash-high",
			effort:         "low", // model suffix takes precedence
			expectedTarget: "gemini-3.8-flash-high",
			expectedTier:   "high",
		},
	}

	for _, c := range cases {
		t.Run(c.model+"_"+c.effort, func(t *testing.T) {
			targetModel, tier := resolveAntigravityModelTier(c.model, c.effort)
			if targetModel != c.expectedTarget {
				t.Errorf("model=%s effort=%s: expected target %s, got %s", c.model, c.effort, c.expectedTarget, targetModel)
			}
			if tier != c.expectedTier {
				t.Errorf("model=%s effort=%s: expected tier %s, got %s", c.model, c.effort, c.expectedTier, tier)
			}
		})
	}
}

func TestAntigravityAdaptiveModelResolution(t *testing.T) {
	mockCatalog := []model.ModelMetadata{
		{ID: "gemini-3.8-flash-high", Family: "gemini", Capabilities: []string{"chat", "code", "reasoning"}},
		{ID: "gemini-3.8-flash-medium", Family: "gemini", Capabilities: []string{"chat", "code", "reasoning"}},
		{ID: "gemini-3.8-flash-low", Family: "gemini", Capabilities: []string{"chat", "code", "reasoning"}},
		{ID: "gemini-4.0-flash-high", Family: "gemini", Capabilities: []string{"chat", "code", "reasoning"}},
		{ID: "claude-sonnet-4-6", Family: "claude", Capabilities: []string{"chat", "code"}},
		{ID: "gpt-oss-120b-medium", Family: "gpt-oss", Capabilities: []string{"chat", "code"}},
		{ID: "gemini-3.1-flash-image", Family: "gemini", Capabilities: []string{"image-generation"}},
	}

	// 1. Exact match for Claude: thinking MUST NOT be injected
	target, _, injectThinking, isImg := resolveAntigravityModel("claude-sonnet-4-6", "high", mockCatalog)
	if target != "claude-sonnet-4-6" {
		t.Errorf("expected claude-sonnet-4-6, got %s", target)
	}
	if injectThinking {
		t.Errorf("claude-sonnet-4-6 must never inject thinkingConfig")
	}
	if isImg {
		t.Errorf("claude-sonnet-4-6 is not an image model")
	}

	// 2. Exact match for GPT-OSS: thinking MUST NOT be injected
	target, _, injectThinking, _ = resolveAntigravityModel("gpt-oss-120b-medium", "medium", mockCatalog)
	if target != "gpt-oss-120b-medium" {
		t.Errorf("expected gpt-oss-120b-medium, got %s", target)
	}
	if injectThinking {
		t.Errorf("gpt-oss-120b-medium must never inject thinkingConfig")
	}

	// 3. Dynamic effort probing for new future model (gemini-4.0-flash + high)
	target, tier, injectThinking, _ := resolveAntigravityModel("gemini-4.0-flash", "high", mockCatalog)
	if target != "gemini-4.0-flash-high" {
		t.Errorf("expected gemini-4.0-flash-high via dynamic probe, got %s", target)
	}
	if tier != "high" {
		t.Errorf("expected tier high, got %s", tier)
	}
	if !injectThinking {
		t.Errorf("gemini-4.0-flash-high with reasoning capability should inject thinkingConfig")
	}

	// 4. Image model detection
	target, _, injectThinking, isImg = resolveAntigravityModel("gemini-3.1-flash-image", "", mockCatalog)
	if !isImg {
		t.Errorf("expected isImage=true for gemini-3.1-flash-image")
	}
	if injectThinking {
		t.Errorf("image model must not inject thinking")
	}
}

func TestAntigravityConvertOpenAIToGeminiContents(t *testing.T) {
	messages := []Message{
		{
			Role:    "system",
			Content: json.RawMessage(`"You are a helpful assistant"`),
		},
		{
			Role:    "user",
			Content: json.RawMessage(`"What is the weather?"`),
		},
		{
			Role:    "assistant",
			Content: json.RawMessage(`""`),
			ToolCalls: json.RawMessage(`[
				{
					"id": "call_123",
					"type": "function",
					"function": {
						"name": "get_weather",
						"arguments": "{\"location\":\"Beijing\"}"
					}
				}
			]`),
		},
		{
			Role:    "tool",
			Content: json.RawMessage(`"Sunny, 25C"`),
		},
	}

	contents, systemParts := convertOpenAIToGeminiContents(messages)

	if len(systemParts) != 1 || systemParts[0]["text"] != "You are a helpful assistant" {
		t.Fatalf("unexpected systemParts: %+v", systemParts)
	}
	if len(contents) != 3 {
		t.Fatalf("expected 3 contents, got %d", len(contents))
	}

	// Check assistant message has functionCall with defaultThinkingAgSignature
	modelContent := contents[1]
	if modelContent["role"] != "model" {
		t.Errorf("expected role model, got %s", modelContent["role"])
	}
	parts, ok := modelContent["parts"].([]map[string]any)
	if !ok || len(parts) != 1 {
		t.Fatalf("expected 1 part (functionCall only when text is empty), got %+v", modelContent["parts"])
	}
	fcPart := parts[0]
	if fcPart["thoughtSignature"] != defaultThinkingAgSignature {
		t.Errorf("expected thoughtSignature %q, got %v", defaultThinkingAgSignature, fcPart["thoughtSignature"])
	}
	fc, ok := fcPart["functionCall"].(map[string]any)
	if !ok || fc["name"] != "get_weather" {
		t.Errorf("expected functionCall get_weather, got %+v", fc)
	}
}

func TestAntigravityConvertOpenAIToGeminiContents_OmitEmptyTextParts(t *testing.T) {
	messages := []Message{
		{
			Role:    "system",
			Content: json.RawMessage(`""`),
		},
		{
			Role:    "user",
			Content: json.RawMessage(`"hello"`),
		},
		{
			Role:    "assistant",
			Content: json.RawMessage(`""`),
			ToolCalls: json.RawMessage(`[
				{
					"id": "call_1",
					"type": "function",
					"function": {
						"name": "lookup",
						"arguments": "{}"
					}
				}
			]`),
		},
	}

	contents, systemParts := convertOpenAIToGeminiContents(messages)
	if len(systemParts) != 0 {
		t.Fatalf("expected 0 systemParts for empty text, got %+v", systemParts)
	}
	if len(contents) != 2 {
		t.Fatalf("expected 2 contents (user and model), got %d: %+v", len(contents), contents)
	}
	modelTurn := contents[1]
	parts := modelTurn["parts"].([]map[string]any)
	if len(parts) != 1 {
		t.Fatalf("expected model turn to omit empty text part and have only 1 part, got %d: %+v", len(parts), parts)
	}
	if parts[0]["functionCall"] == nil {
		t.Fatalf("expected functionCall part, got %+v", parts[0])
	}
	if _, hasText := parts[0]["text"]; hasText {
		t.Fatalf("part should not have text key: %+v", parts[0])
	}
}

func TestAntigravityGemini4SamplingParamStrippingAndFROrder(t *testing.T) {
	// 1. Verify isGemini4Model detection
	if !isGemini4Model("gemini-4") || !isGemini4Model("antigravity/gemini-4.0-flash") || !isGemini4Model("gemini-4-argon") {
		t.Fatal("expected isGemini4Model to match gemini-4 variations")
	}
	if isGemini4Model("gemini-3.8-flash") || isGemini4Model("gemini-2.5-pro") {
		t.Fatal("expected isGemini4Model not to match gemini 2.x or 3.x")
	}

	// 2. Verify functionResponse placed last for Gemini 4
	messages := []Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{Role: "tool", ToolCallID: "call_1", Content: json.RawMessage(`"tool output 1"`)},
		{Role: "user", Content: json.RawMessage(`"and what about this"`)},
		{Role: "tool", ToolCallID: "call_2", Content: json.RawMessage(`"tool output 2"`)},
	}

	contents, _ := convertOpenAIToGeminiContents(messages, "gemini-4")
	if len(contents) != 1 {
		t.Fatalf("expected adjacent user/tool turns merged into 1 user turn, got %d", len(contents))
	}
	userTurn := contents[0]
	parts := userTurn["parts"].([]map[string]any)
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts, got %d", len(parts))
	}
	// The last two parts MUST be functionResponse
	if _, ok := parts[2]["functionResponse"]; !ok {
		t.Errorf("expected part 2 to be functionResponse, got %+v", parts[2])
	}
	if _, ok := parts[3]["functionResponse"]; !ok {
		t.Errorf("expected part 3 to be functionResponse, got %+v", parts[3])
	}
}

func TestAntigravityStreamingCandidatesPayloads(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Test 1: root candidates with thought and text
	sseData := `data: {"candidates": [{"content": {"role": "model", "parts": [{"thought": true, "text": "Thinking..."}]}}]}

data: {"candidates": [{"content": {"role": "model", "parts": [{"text": "Hello world"}]}}]}

data: [DONE]
`
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(sseData)),
		Header:     make(http.Header),
	}
	uc := &usageContext{
		StartedAt: time.Now(),
		Provider:  "antigravity",
		Model:     "claude-sonnet-4-6",
	}
	w := httptest.NewRecorder()
	srv.proxyAntigravityStreaming(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, "claude-sonnet-4-6", uc, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if uc.Response != "Hello world" {
		t.Errorf("expected uc.Response to be 'Hello world', got %q", uc.Response)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Thinking...") || !strings.Contains(body, "Hello world") {
		t.Errorf("expected stream to contain both reasoning and content: %s", body)
	}
}

func TestAntigravityNonStreamingFormats(t *testing.T) {
	srv, _ := setupTestServer(t)

	// 1. Raw JSON response (from generateContent or direct JSON relay)
	rawJSON := `{"response": {"candidates": [{"content": {"role": "model", "parts": [{"thought": true, "text": "Reasoning"}, {"text": "Answer"}]}}]}, "usageMetadata": {"promptTokenCount": 5, "candidatesTokenCount": 10}}`
	respJSON := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(rawJSON)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	uc1 := &usageContext{
		StartedAt: time.Now(),
		Provider:  "antigravity",
		Model:     "gemini-3.8-flash",
	}
	w1 := httptest.NewRecorder()
	srv.proxyAntigravityNonStreaming(w1, respJSON, "gemini-3.8-flash", uc1, nil)
	if w1.Code != http.StatusOK {
		t.Fatalf("raw JSON: expected 200, got %d", w1.Code)
	}
	if uc1.Response != "Answer" {
		t.Errorf("expected 'Answer', got %q", uc1.Response)
	}
	var obj1 map[string]any
	if err := json.Unmarshal(w1.Body.Bytes(), &obj1); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	msg := obj1["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "Answer" || msg["reasoning_content"] != "Reasoning" {
		t.Errorf("unexpected message payload: %+v", msg)
	}
}

func TestAntigravityToolCallingSupport(t *testing.T) {
	srv, _ := setupTestServer(t)

	// 1. Verify convertOpenAIToGeminiContents handles tool role with functionResponse
	messages := []Message{
		{
			Role:      "assistant",
			ToolCalls: json.RawMessage(`[{"id":"call_weather_1","type":"function","function":{"name":"get_weather","arguments":"{\"location\":\"Tokyo\"}"}}]`),
		},
		{
			Role:       "tool",
			ToolCallID: "call_weather_1",
			Content:    json.RawMessage(`"{\"temp\": 22}"`),
		},
	}
	contents, _ := convertOpenAIToGeminiContents(messages)
	if len(contents) != 2 {
		t.Fatalf("expected 2 contents, got %d", len(contents))
	}
	toolMsg := contents[1]
	if toolMsg["role"] != "user" {
		t.Errorf("expected toolMsg role 'user', got %v", toolMsg["role"])
	}
	parts := toolMsg["parts"].([]map[string]any)
	if len(parts) != 1 || parts[0]["functionResponse"] == nil {
		t.Fatalf("expected functionResponse in parts, got %+v", parts)
	}
	fr := parts[0]["functionResponse"].(map[string]any)
	if fr["name"] != "get_weather" {
		t.Errorf("expected functionResponse name 'get_weather', got %v", fr["name"])
	}

	// 2. Verify proxyAntigravityStreaming emits tool_calls
	toolSSE := `data: {"candidates": [{"content": {"role": "model", "parts": [{"functionCall": {"name": "get_weather", "args": {"location": "Paris"}}}]}}], "finishReason": "STOP"}` + "\n\n"
	respStream := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(toolSSE)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	ucStream := &usageContext{StartedAt: time.Now(), Provider: "antigravity", Model: "gemini-3.8-flash"}
	wStream := httptest.NewRecorder()
	srv.proxyAntigravityStreaming(wStream, httptest.NewRequest("POST", "/v1/chat/completions", nil), respStream, "gemini-3.8-flash", ucStream, nil)
	if !strings.Contains(wStream.Body.String(), `"tool_calls"`) || !strings.Contains(wStream.Body.String(), "get_weather") {
		t.Errorf("expected tool_calls in stream, got: %s", wStream.Body.String())
	}

	// 3. Verify proxyAntigravityNonStreaming emits tool_calls
	toolJSON := `{"candidates": [{"content": {"role": "model", "parts": [{"functionCall": {"name": "search", "args": {"q": "golang"}}}]}}]}`
	respNonStream := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(toolJSON)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	ucNonStream := &usageContext{StartedAt: time.Now(), Provider: "antigravity", Model: "gemini-3.8-flash"}
	wNonStream := httptest.NewRecorder()
	srv.proxyAntigravityNonStreaming(wNonStream, respNonStream, "gemini-3.8-flash", ucNonStream, nil)
	var nonStreamObj map[string]any
	if err := json.Unmarshal(wNonStream.Body.Bytes(), &nonStreamObj); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	choice0 := nonStreamObj["choices"].([]any)[0].(map[string]any)
	if choice0["finish_reason"] != "tool_calls" {
		t.Errorf("expected finish_reason 'tool_calls', got %v", choice0["finish_reason"])
	}
	msgObj := choice0["message"].(map[string]any)
	tcList := msgObj["tool_calls"].([]any)
	if len(tcList) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(tcList))
	}
}

func TestAntigravityToolDeclarationSchemaSanitization(t *testing.T) {
	// Replicates grep tool with type: ["number", "null"] and additionalProperties: false
	rawTools := json.RawMessage(`[
		{
			"type": "function",
			"function": {
				"name": "grep",
				"description": "Regex search",
				"parameters": {
					"type": "object",
					"properties": {
						"pattern": {"type": "string"},
						"skip": {"type": ["number", "null"]}
					},
					"required": ["pattern"],
					"additionalProperties": false
				}
			}
		}
	]`)

	var toolsList []map[string]any
	if err := json.Unmarshal(rawTools, &toolsList); err != nil {
		t.Fatalf("unmarshal tools: %v", err)
	}

	var declarations []any
	for _, raw := range toolsList {
		fn := raw["function"].(map[string]any)
		params := fn["parameters"].(map[string]any)
		cleaned := translator.CleanJSONSchemaForGemini(params)
		declarations = append(declarations, map[string]any{
			"name":        fn["name"],
			"description": fn["description"],
			"parameters":  cleaned,
		})
	}

	if len(declarations) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(declarations))
	}
	decl := declarations[0].(map[string]any)
	params := decl["parameters"].(map[string]any)

	// Proto field is not repeating: type must be string, not slice
	props := params["properties"].(map[string]any)
	skip := props["skip"].(map[string]any)
	if skip["type"] != "number" {
		t.Errorf("skip.type = %v, want 'number'", skip["type"])
	}
	if _, ok := params["additionalProperties"]; ok {
		t.Errorf("additionalProperties must be stripped for Gemini/Antigravity compatibility")
	}
}

func TestAntigravityToolCallIDsSurviveHistoryConversion(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: json.RawMessage(`"read the file"`)},
		{Role: "assistant", ToolCalls: json.RawMessage(`[{"id":"call_9f60_0","type":"function","function":{"name":"read","arguments":"{\"path\":\"AGENTS.md\"}"}},{"id":"call_9f60_1","type":"function","function":{"name":"bash","arguments":"{\"command\":\"ls\"}"}}]`)},
		{Role: "tool", ToolCallID: "call_9f60_0", Content: json.RawMessage(`"file text"`)},
		{Role: "tool", ToolCallID: "call_9f60_1", Content: json.RawMessage(`"dir listing"`)},
	}
	contents, _ := convertOpenAIToGeminiContents(messages, "claude-opus-5-5-high")
	if len(contents) != 3 {
		t.Fatalf("expected 3 turns (tool results merge), got %d: %+v", len(contents), contents)
	}

	aParts := contents[1]["parts"].([]map[string]any)
	if len(aParts) != 2 {
		t.Fatalf("expected 2 functionCall parts, got %+v", aParts)
	}
	wantIDs := map[string]string{}
	for i, part := range aParts {
		fc, ok := part["functionCall"].(map[string]any)
		if !ok {
			t.Fatalf("part %d is not a functionCall: %+v", i, part)
		}
		id, _ := fc["id"].(string)
		if id == "" {
			t.Errorf("functionCall part %d lost its id: %+v", i, fc)
		}
		wantIDs[fc["name"].(string)] = id
	}

	uParts := contents[2]["parts"].([]map[string]any)
	if len(uParts) != 2 {
		t.Fatalf("expected 2 functionResponse parts, got %+v", uParts)
	}
	for _, part := range uParts {
		fr, ok := part["functionResponse"].(map[string]any)
		if !ok {
			t.Fatalf("expected functionResponse, got %+v", part)
		}
		if fr["id"] != wantIDs[fr["name"].(string)] {
			t.Errorf("functionResponse %v id = %v, want %v", fr["name"], fr["id"], wantIDs[fr["name"].(string)])
		}
	}
}

func TestAntigravitySynthesizesMissingToolCallIDs(t *testing.T) {
	messages := []Message{
		{Role: "assistant", ToolCalls: json.RawMessage(`[{"type":"function","function":{"name":"read","arguments":"{}"}},{"type":"function","function":{"name":"read","arguments":"{}"}}]`)},
	}
	contents, _ := convertOpenAIToGeminiContents(messages)
	parts := contents[0]["parts"].([]map[string]any)
	seen := map[string]bool{}
	for i, part := range parts {
		fc := part["functionCall"].(map[string]any)
		id, _ := fc["id"].(string)
		if id == "" {
			t.Errorf("functionCall %d missing id; Claude backends 400 on tool_use.id", i)
		}
		if seen[id] {
			t.Errorf("duplicate synthesized id %q across calls", id)
		}
		seen[id] = true
	}
}

func TestAntigravityStreamingSeparatesParallelToolCalls(t *testing.T) {
	srv, _ := setupTestServer(t)
	stream := "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"functionCall\":{\"name\":\"read\",\"args\":{\"path\":\"a\"}}}]}}]}\n\n" +
		"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"functionCall\":{\"name\":\"bash\",\"args\":{\"command\":\"ls\"}}}]}}]}\n\n"
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(stream)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	uc := &usageContext{StartedAt: time.Now(), Provider: "antigravity", Model: "gemini-3.8-flash"}
	w := httptest.NewRecorder()
	srv.proxyAntigravityStreaming(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, "gemini-3.8-flash", uc, nil)

	slots := map[string]int{}
	count := 0
	for _, line := range strings.Split(w.Body.String(), "\n") {
		data, isDone, ok := translator.ParseSSEDataLineString(line)
		if !ok || isDone {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name string `json:"name"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil || len(chunk.Choices) == 0 {
			continue
		}
		for _, tc := range chunk.Choices[0].Delta.ToolCalls {
			slots[tc.Function.Name] = tc.Index
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected 2 tool_call deltas, got %d:\n%s", count, w.Body.String())
	}
	if slots["read"] == slots["bash"] {
		t.Errorf("parallel tool calls landed in the same slot (clients would concatenate arguments): %+v", slots)
	}
}

func runAntigravityStreamFixture(t *testing.T, fixture string) (string, int) {
	t.Helper()
	srv, _ := setupTestServer(t)
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(fixture)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	uc := &usageContext{StartedAt: time.Now(), Provider: "antigravity", Model: "claude-opus-5-5-high", Status: http.StatusOK}
	w := httptest.NewRecorder()
	srv.proxyAntigravityStreaming(w, httptest.NewRequest("POST", "/v1/chat/completions", nil), resp, "claude-opus-5-5-high", uc, nil)
	return w.Body.String(), uc.Status
}

func TestAntigravityStreamInBandErrorFrameAborts(t *testing.T) {
	body, status := runAntigravityStreamFixture(t,
		"data: {\"error\":{\"code\":429,\"message\":\"quota exceeded\",\"status\":\"RESOURCE_EXHAUSTED\"}}\n\n")
	if !strings.Contains(body, "quota exceeded") {
		t.Fatalf("expected in-band error to reach the client, got: %s", body)
	}
	if !strings.Contains(body, "rate_limit_error") {
		t.Errorf("expected anthropic-mapped error type, got: %s", body)
	}
	if strings.Contains(body, "[DONE]") {
		t.Errorf("error must replace the terminal frame, got: %s", body)
	}
	if status != 429 {
		t.Errorf("uc.Status = %d, want 429", status)
	}
}

func TestAntigravityStreamRefusalTurnIsNotAbort(t *testing.T) {
	// SAFETY with zero parts is a legitimate terminal turn; the emptiness
	// guard must not false-positive (F17) and no error frame may appear.
	body, status := runAntigravityStreamFixture(t,
		"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[]},\"finishReason\":\"SAFETY\"}]}\n\n")
	if !strings.Contains(body, "content_filter") {
		t.Fatalf("expected finish_reason content_filter, got: %s", body)
	}
	if strings.Contains(body, "\"error\"") {
		t.Errorf("refusal turn must not emit an error frame: %s", body)
	}
	if status != 200 {
		t.Errorf("uc.Status = %d, want 200", status)
	}
}

func TestAntigravityStreamMaxTokensSurfacesLength(t *testing.T) {
	body, _ := runAntigravityStreamFixture(t,
		"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"partial\"}]},\"finishReason\":\"MAX_TOKENS\"}]}\n\n")
	if !strings.Contains(body, `"finish_reason":"length"`) {
		t.Fatalf("expected mapped length finish (→ anthropic max_tokens), got: %s", body)
	}
}

func TestAntigravityStreamZeroArgCallEmitsEmptyObject(t *testing.T) {
	body, _ := runAntigravityStreamFixture(t,
		"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"functionCall\":{\"name\":\"noop\"}}]}}]}\n\n")
	if !strings.Contains(body, `"arguments":"{}"`) {
		t.Fatalf("zero-arg call must marshal as {}, got: %s", body)
	}
	if strings.Contains(body, `"arguments":"null"`) {
		t.Errorf("arguments must never be null: %s", body)
	}
}

func TestAntigravityEmptyStreamAborts(t *testing.T) {
	body, status := runAntigravityStreamFixture(t, "")
	if !strings.Contains(body, "no content before closing") {
		t.Fatalf("expected abort frame for header-only stream, got: %s", body)
	}
	if strings.Contains(body, "[DONE]") {
		t.Errorf("aborted stream must not send [DONE]: %s", body)
	}
	if status != 502 {
		t.Errorf("uc.Status = %d, want 502", status)
	}
}

func TestAntigravityNonStreamingMaxTokensMapping(t *testing.T) {
	srv, _ := setupTestServer(t)
	payload := `{"candidates":[{"content":{"role":"model","parts":[{"text":"half an answer"}]},"finishReason":"MAX_TOKENS"}]}`
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(payload)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	uc := &usageContext{StartedAt: time.Now(), Provider: "antigravity", Model: "claude-opus-5-5-high"}
	w := httptest.NewRecorder()
	srv.proxyAntigravityNonStreaming(w, resp, "claude-opus-5-5-high", uc, nil)
	var obj map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &obj); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	fr := obj["choices"].([]any)[0].(map[string]any)["finish_reason"]
	if fr != "length" {
		t.Fatalf("finish_reason = %v, want length (truncation must be visible)", fr)
	}
}

func TestAntigravityContentPartsCarryImages(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"why does this fail?"},{"type":"image_url","image_url":{"url":"data:image/png;base64,QUJD"}}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"image_url","image_url":{"url":"data:image/png;base64,REFE"}}]`)},
	}
	contents, _ := convertOpenAIToGeminiContents(msgs)
	if len(contents) != 1 {
		t.Fatalf("adjacent user turns merge, got %d: %+v", len(contents), contents)
	}
	p := contents[0]["parts"].([]map[string]any)
	if len(p) != 3 {
		t.Fatalf("expected text+inlineData+inlineData parts, got %+v", p)
	}
	ined := p[1]["inlineData"].(map[string]any)
	if ined["mimeType"] != "image/png" || ined["data"] != "QUJD" {
		t.Errorf("unexpected inlineData: %+v", ined)
	}
	if p[2]["inlineData"] == nil {
		t.Errorf("image-only turn lost its part: %+v", p)
	}
}

func TestAntigravityToolResultNumberPrecision(t *testing.T) {
	msgs := []Message{
		{Role: "assistant", ToolCalls: json.RawMessage(`[{"id":"call_num","type":"function","function":{"name":"get","arguments":"{}"}}]`)},
		{Role: "tool", ToolCallID: "call_num", Content: json.RawMessage(`"173037189432876549823"`)},
	}
	contents, _ := convertOpenAIToGeminiContents(msgs)
	fr := contents[1]["parts"].([]map[string]any)[0]["functionResponse"].(map[string]any)
	out, _ := json.Marshal(fr["response"])
	if !strings.Contains(string(out), "173037189432876549823") {
		t.Errorf("bare-number tool output mangled by float64: %s", out)
	}
}

func TestAntigravityObjectShapedArgumentsKept(t *testing.T) {
	msgs := []Message{
		{Role: "assistant", ToolCalls: json.RawMessage(`[{"id":"call_obj","type":"function","function":{"name":"f","arguments":{"a":1}}}]`)},
	}
	contents, _ := convertOpenAIToGeminiContents(msgs)
	parts := contents[0]["parts"].([]map[string]any)
	if len(parts) != 1 {
		t.Fatalf("object-shaped arguments must not drop the call, got %+v", parts)
	}
	fc := parts[0]["functionCall"].(map[string]any)
	args, _ := fc["args"].(map[string]any)
	if args["a"] != 1.0 {
		t.Errorf("expected args {a:1}, got %+v", fc["args"])
	}
}

func TestAnthropicErrorEnvelopeRewritesGatewayErrors(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		wantType    string
		wantMessage string
	}{
		{"flat gateway string", 400, `{"error":"antigravity upstream error (HTTP 400): boom"}`, "invalid_request_error", "antigravity upstream error (HTTP 400): boom"},
		{"openai nested", 429, `{"error":{"message":"slow down","type":"rate_limit"}}`, "rate_limit_error", "slow down"},
		{"non-json", 500, `gateway is down`, "api_error", "gateway is down"},
	}
	for _, c := range cases {
		out := anthropicErrorEnvelope(c.status, []byte(c.body))
		var obj map[string]any
		if err := json.Unmarshal(out, &obj); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if obj["type"] != "error" {
			t.Errorf("%s: type = %v, want error", c.name, obj["type"])
		}
		inner, _ := obj["error"].(map[string]any)
		if inner["type"] != c.wantType {
			t.Errorf("%s: error.type = %v, want %s", c.name, inner["type"], c.wantType)
		}
		if inner["message"] != c.wantMessage {
			t.Errorf("%s: error.message = %v, want %q", c.name, inner["message"], c.wantMessage)
		}
	}

	// Already-shaped bodies pass through untouched.
	shaped := []byte(`{"type":"error","error":{"type":"invalid_request_error","message":"keep me"}}`)
	if got := anthropicErrorEnvelope(400, shaped); !bytes.Equal(got, shaped) {
		t.Errorf("shaped body mutated: %s", got)
	}
}

func TestAntigravityNonStreamInBandErrorWithPartialContentFailsOver(t *testing.T) {
	srv, _ := setupTestServer(t)
	conn := &model.ProviderConnection{
		ID: "opus-1", Provider: "antigravity", AuthType: "oauth", IsActive: true,
		Data: model.ConnectionData{AccessToken: "tok"},
	}
	// Partial content followed by an in-band quota frame (enum-only prose).
	sse := "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"half a\"}]}}]}\n\n" +
		"data: {\"error\":{\"message\":\"Resource has been exhausted (e.g. check quota).\"}}\n\n"
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(sse)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	uc := &usageContext{StartedAt: time.Now(), Provider: "antigravity", Model: "claude-opus-5-5-high"}
	w := httptest.NewRecorder()
	retryable := srv.proxyAntigravityNonStreaming(w, resp, "claude-opus-5-5-high", uc, conn)
	if !retryable {
		t.Fatalf("quota-class in-band error must hand off to the sibling connection, body=%s", w.Body.String())
	}
	if w.Body.Len() != 0 {
		t.Errorf("nothing may be written when the turn is retried: %s", w.Body.String())
	}
	if conn.Data.RateLimitedUntil != "" {
		t.Errorf("quota exhaustion must park the POOL, not the whole account (Gemini would melt down): %s", conn.Data.RateLimitedUntil)
	}
	lock, _ := conn.Data.ProviderSpecificData["modelLock_claude-opus-5-5-high"].(string)
	if lock == "" {
		t.Fatalf("RESOURCE_EXHAUSTED must set a pool-scoped model lock, got TestStatus=%v LastError=%q", conn.Data.TestStatus, conn.Data.LastError)
	}
	until, perr := time.Parse(time.RFC3339, lock)
	if perr != nil {
		t.Fatalf("model lock unparseable: %v", perr)
	}
	// Must be the 15-min quota window (CooldownQuota), not the 2s exponential
	// backoff the bare derived-429 status rule would give.
	if remaining := time.Until(until); remaining < 5*time.Minute {
		t.Errorf("quota park too short (%v) — enum piggyback onto the classified text regressed", remaining)
	}
}

func TestAntigravityNonStreamInBandDeterministicErrorIsHonest(t *testing.T) {
	srv, _ := setupTestServer(t)
	conn := &model.ProviderConnection{
		ID: "opus-2", Provider: "antigravity", AuthType: "oauth", IsActive: true,
		Data: model.ConnectionData{AccessToken: "tok"},
	}
	sse := "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"half a\"}]}}]}\n\n" +
		"data: {\"error\":{\"code\":400,\"message\":\"bad shape\",\"status\":\"INVALID_ARGUMENT\"}}\n\n"
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(sse)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	uc := &usageContext{StartedAt: time.Now(), Provider: "antigravity", Model: "claude-opus-5-5-high"}
	w := httptest.NewRecorder()
	retryable := srv.proxyAntigravityNonStreaming(w, resp, "claude-opus-5-5-high", uc, conn)
	if retryable {
		t.Errorf("deterministic 400-class error must not burn the sibling")
	}
	if strings.Contains(w.Body.String(), "half a") || strings.Contains(w.Body.String(), "finish_reason") {
		t.Errorf("truncated partial must never surface as a completion: %s", w.Body.String())
	}
	var obj map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &obj); err != nil {
		t.Fatalf("nested error envelope expected: %v (%s)", err, w.Body.String())
	}
	inner, _ := obj["error"].(map[string]any)
	if inner["type"] != "invalid_request_error" {
		t.Errorf("error.type = %v, want invalid_request_error", inner["type"])
	}
	if uc.Status != 400 {
		t.Errorf("uc.Status = %d, want 400", uc.Status)
	}
}

func newAntigravityQuotaUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"code":429,"message":"Resource has been exhausted (e.g. check quota).","status":"RESOURCE_EXHAUSTED"}}`))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestAntigravityQuotaExhaustionParksPoolNotAccount(t *testing.T) {
	srv, _ := setupTestServer(t)
	ts := newAntigravityQuotaUpstream(t)
	conn := &model.ProviderConnection{
		ID: "opus-pool", Provider: "antigravity", AuthType: "oauth", IsActive: true,
		Data: model.ConnectionData{
			AccessToken:          "tok",
			BaseURL:              ts.URL,
			ProviderSpecificData: map[string]any{"projectId": "proj-1"},
		},
	}
	req := ChatCompletionRequest{
		Model:    "antigravity/claude-opus-5-5-high",
		Messages: []Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}
	raw, _ := json.Marshal(req)
	modelInfo := model.ModelInfo{Provider: "antigravity", Model: "claude-opus-5-5-high"}
	w := httptest.NewRecorder()
	retryable := srv.handleAntigravityChat(w, httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(raw)), req, raw, modelInfo, conn, provider.ProviderInfo{})
	if !retryable {
		t.Fatalf("quota 429 must hop to the next account, body=%s", w.Body.String())
	}
	if conn.Data.RateLimitedUntil != "" {
		t.Errorf("quota exhaustion must NOT park the whole account (Gemini pools would melt down): RateLimitedUntil=%s", conn.Data.RateLimitedUntil)
	}
	lock, _ := conn.Data.ProviderSpecificData["modelLock_claude-opus-5-5-high"].(string)
	if lock == "" {
		t.Fatalf("expected pool-scoped model lock on the exhausted claude model, got %+v", conn.Data.ProviderSpecificData)
	}
	if _, banded := conn.Data.ProviderSpecificData["modelLock_gemini-3.8-flash-high"]; banded {
		t.Errorf("gemini pool must stay servable when only claude is exhausted")
	}

	// Selector parity: the locked claude model is skipped, gemini still picks this account.
	conns := []model.ProviderConnection{*conn}
	if c := provider.SelectCredential(conns, "claude-opus-5-5-high", nil); c != nil {
		t.Errorf("selector must skip the pool-locked claude model")
	}
	if c := provider.SelectCredential(conns, "gemini-3.8-flash-high", nil); c == nil {
		t.Errorf("selector must still serve gemini on the same account")
	}
}

func TestAntigravityQuotaPoolMapping(t *testing.T) {
	cases := map[string]string{
		"claude-opus-5-5-high":   "claude",
		"gpt-oss-120b-medium":    "claude",
		"gemini-3.8-flash-high":  "gemini",
		"gemini-3.1-flash-image": "gemini",
		"gemini-4-argon-high":    "gemini4",
		"gemini-4":               "gemini4",
		"mystery-model":          "",
	}
	for m, want := range cases {
		if got := antigravityQuotaPool(m); got != want {
			t.Errorf("antigravityQuotaPool(%q) = %q, want %q", m, got, want)
		}
	}
}
