package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/loopguard"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

const loopGuardHintMarker = "STOP repeating yourself"

const loopGuardAssistantTurn = `{"role":"assistant","content":[{"type":"text","text":"I will read the repository files before answering"}]}`

// loopGuardChatBody trips the text-repeat detector and declares tools.
func loopGuardChatBody(model string) string {
	return `{"model":"` + model + `","stream":false,"max_tokens":64,` +
		`"tools":[{"type":"function","function":{"name":"read_file","parameters":{"type":"object"}}}],` +
		`"messages":[{"role":"system","content":"You are helpful."},` +
		`{"role":"user","content":"summarize the repository"},` +
		`{"role":"assistant","content":"I will read the repository files before answering"},` +
		`{"role":"assistant","content":"I will read the repository files before answering"},` +
		`{"role":"assistant","content":"I will read the repository files before answering"}]}`
}

func loopGuardMessagesBody(model string) string {
	return `{"model":"` + model + `","stream":false,"max_tokens":64,"system":"Base prompt.",` +
		`"tools":[{"name":"read_file","input_schema":{"type":"object"}}],` +
		`"messages":[{"role":"user","content":[{"type":"text","text":"summarize the repository"}]},` +
		loopGuardAssistantTurn + `,` + loopGuardAssistantTurn + `,` + loopGuardAssistantTurn + `]}`
}

type loopGuardUpstream struct {
	url      string
	received chan []byte
}

func (u *loopGuardUpstream) next(t *testing.T) []byte {
	t.Helper()
	select {
	case body := <-u.received:
		return body
	case <-time.After(10 * time.Second):
		t.Fatal("upstream never received the request")
		return nil
	}
}

func newLoopGuardUpstream(t *testing.T, respond string) *loopGuardUpstream {
	t.Helper()
	upstream := &loopGuardUpstream{received: make(chan []byte, 1)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		upstream.received <- body
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, respond)
	}))
	t.Cleanup(server.Close)
	upstream.url = server.URL
	return upstream
}

func setLoopGuardEnabled(t *testing.T, database *db.DB, enabled bool) {
	t.Helper()
	settings, err := database.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.LoopGuardEnabled = enabled
	if err := database.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
}

func decodeJSONMap(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("upstream body is not JSON (%v): %s", err, data)
	}
	return out
}

// assertNoGuardText encodes the byte-identity requirement: with the switch off the
// upstream receives exactly the body the pipeline produces without the guard.
func assertNoGuardText(t *testing.T, captured []byte) {
	t.Helper()
	if bytes.Contains(captured, []byte(loopguard.TerminationPrompt)) {
		t.Errorf("termination contract leaked into the upstream body: %s", captured)
	}
	if bytes.Contains(captured, []byte(loopGuardHintMarker)) {
		t.Errorf("loop hint leaked into the upstream body: %s", captured)
	}
}

func assertGuardTexts(t *testing.T, captured []byte) {
	t.Helper()
	if !bytes.Contains(captured, []byte(loopguard.TerminationPrompt)) {
		t.Errorf("tools request is missing the termination contract: %s", captured)
	}
	if !bytes.Contains(captured, []byte(loopGuardHintMarker)) {
		t.Errorf("looping conversation is missing the loop hint: %s", captured)
	}
}

func TestLoopGuardSwitchOnChatCompletionsPath(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			upstream := newLoopGuardUpstream(t, `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
			srv, database := setupTestServer(t)
			if err := database.CreateConnection(&model.ProviderConnection{
				ID: "loopguard-openai", Provider: "openai", AuthType: "api-key", IsActive: true,
				Data: model.ConnectionData{APIKey: "sk-mock", BaseURL: upstream.url},
			}); err != nil {
				t.Fatal(err)
			}
			setLoopGuardEnabled(t, database, enabled)

			body := loopGuardChatBody("openai/mock-model")
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, newLocalRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)))
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			captured := upstream.next(t)
			sent := decodeJSONMap(t, []byte(body))
			upstreamBody := decodeJSONMap(t, captured)

			if !enabled {
				assertNoGuardText(t, captured)
				if !reflect.DeepEqual(upstreamBody["messages"], sent["messages"]) {
					t.Errorf("switch off modified the conversation:\nsent     =%v\nupstream =%v", sent["messages"], upstreamBody["messages"])
				}
				return
			}
			assertGuardTexts(t, captured)
			if n := len(upstreamBody["messages"].([]any)); n != len(sent["messages"].([]any)) {
				t.Errorf("guard must compose into the existing system message, got %d messages", n)
			}
		})
	}
}

func TestLoopGuardSwitchOnMessagesPath(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			upstream := newLoopGuardUpstream(t, `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"claude-sonnet-4-20250514","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
			srv, database := setupTestServer(t)
			if err := database.CreateConnection(&model.ProviderConnection{
				ID: "loopguard-anthropic", Provider: "anthropic", AuthType: "api-key", IsActive: true,
				Data: model.ConnectionData{APIKey: "sk-ant-mock", BaseURL: upstream.url},
			}); err != nil {
				t.Fatal(err)
			}
			setLoopGuardEnabled(t, database, enabled)

			body := loopGuardMessagesBody("anthropic/claude-sonnet-4-20250514")
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, newLocalRequest(http.MethodPost, "/v1/messages", strings.NewReader(body)))
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			captured := upstream.next(t)
			sent := decodeJSONMap(t, []byte(body))
			upstreamBody := decodeJSONMap(t, captured)

			if !enabled {
				assertNoGuardText(t, captured)
				if upstreamBody["system"] != "Base prompt." {
					t.Errorf("switch off modified the system block: %v", upstreamBody["system"])
				}
				if !reflect.DeepEqual(upstreamBody["messages"], sent["messages"]) {
					t.Errorf("switch off modified the conversation:\nsent     =%v\nupstream =%v", sent["messages"], upstreamBody["messages"])
				}
				return
			}
			assertGuardTexts(t, captured)
			sys, ok := upstreamBody["system"].(string)
			if !ok || !strings.HasPrefix(sys, "Base prompt.\n\n") {
				t.Errorf("guard must compose into the existing system block, got %v", upstreamBody["system"])
			}
		})
	}
}

func TestLoopGuardSwitchOnAntigravityPath(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			upstream := newLoopGuardUpstream(t, `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`)
			srv, database := setupTestServer(t)
			if err := database.CreateConnection(&model.ProviderConnection{
				ID: "loopguard-antigravity", Provider: "antigravity", AuthType: "oauth", IsActive: true,
				Data: model.ConnectionData{AccessToken: "test-only", BaseURL: upstream.url,
					ProviderSpecificData: map[string]any{"projectId": "loopguard-project"}},
			}); err != nil {
				t.Fatal(err)
			}
			setLoopGuardEnabled(t, database, enabled)

			body := loopGuardChatBody("antigravity/claude-sonnet-4-6")
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, newLocalRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)))
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			captured := upstream.next(t)
			envelope := decodeJSONMap(t, captured)
			inner, ok := envelope["request"].(map[string]any)
			if !ok {
				t.Fatalf("antigravity envelope lost its inner request: %s", captured)
			}
			parts, _ := inner["systemInstruction"].(map[string]any)["parts"].([]any)

			if !enabled {
				assertNoGuardText(t, captured)
				if len(parts) != 1 || parts[0].(map[string]any)["text"] != "You are helpful." {
					t.Errorf("switch off modified the gemini systemInstruction: %v", parts)
				}
				return
			}
			assertGuardTexts(t, captured)
			if len(parts) != 3 {
				t.Fatalf("hint and contract must land inside the envelope: %v", parts)
			}
			if last := parts[2].(map[string]any)["text"]; last != loopguard.TerminationPrompt {
				t.Errorf("unexpected final gemini system part: %v", last)
			}
		})
	}
}
