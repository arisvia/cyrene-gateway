package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestApplyTokenSaver_FallbackAndExclusion(t *testing.T) {
	srv, database := setupTestServer(t)

	// Configure settings: Caveman & Ponytail enabled without explicit levels, and deepseek excluded
	settings := &db.Settings{
		RTKEnabled:        true,
		CavemanEnabled:    true,
		CavemanLevel:      "", // Should fallback to lite
		PonytailEnabled:   true,
		PonytailLevel:     "", // Should fallback to lite
		TokenSaverExclude: []string{"deepseek"},
	}
	if err := database.SaveSettings(settings); err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	// 1. Test standard OpenAI request with fallback levels applied
	bodyMap := map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			map[string]any{"role": "user", "content": "Write some code."},
		},
	}
	srv.applyTokenSaver(bodyMap, "openai", "openai")

	msgs, ok := bodyMap["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("expected 2 messages after injection, got %v", msgs)
	}
	sysMsg := msgs[0].(map[string]any)
	sysContent, _ := sysMsg["content"].(string)
	if !strings.Contains(sysContent, "Respond tersely") {
		t.Errorf("expected CavemanLite fallback prompt injected, got: %s", sysContent)
	}
	if !strings.Contains(sysContent, "lazy senior developer") {
		t.Errorf("expected PonytailLite fallback prompt injected, got: %s", sysContent)
	}

	// 2. Test provider exclusion (deepseek)
	deepseekBody := map[string]any{
		"model": "deepseek-chat",
		"messages": []any{
			map[string]any{"role": "user", "content": "Hello"},
		},
	}
	srv.applyTokenSaver(deepseekBody, "openai", "deepseek")
	dsMsgs := deepseekBody["messages"].([]any)
	if len(dsMsgs) != 1 {
		t.Fatalf("expected excluded provider deepseek to have unchanged messages, got %d", len(dsMsgs))
	}

	// 3. Test RTK compression within applyTokenSaver
	var lines []string
	for range 300 {
		lines = append(lines, "log line "+strings.Repeat("a", 20))
	}
	toolBody := map[string]any{
		"model": "claude-3-5-sonnet-20241022",
		"messages": []any{
			map[string]any{"role": "tool", "content": strings.Join(lines, "\n")},
		},
	}
	srv.applyTokenSaver(toolBody, "openai", "anthropic")
	// Since system prompt was prepended at index 0, the tool message is at index 1
	toolMsg := toolBody["messages"].([]any)[1].(map[string]any)
	toolContent := toolMsg["content"].(string)
	if !strings.Contains(toolContent, "lines omitted") {
		t.Errorf("expected RTK compression to truncate tool output, got: %s", toolContent)
	}
}

func TestApplyTokenSaver_AllLevelsAndFormats(t *testing.T) {
	srv, database := setupTestServer(t)

	// Test case-insensitive exclusion by model prefix
	settings := &db.Settings{
		RTKEnabled:        true,
		CavemanEnabled:    true,
		CavemanLevel:      "wenyan",
		PonytailEnabled:   true,
		PonytailLevel:     "ultra",
		TokenSaverExclude: []string{"DeepSeek", "anthropic"},
	}
	if err := database.SaveSettings(settings); err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	// 1. OpenAI format with wenyan + ultra
	openaiBody := map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
	}
	srv.applyTokenSaver(openaiBody, "openai", "openai")
	msgs := openaiBody["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	sysContent := msgs[0].(map[string]any)["content"].(string)
	if !strings.Contains(sysContent, "文言文") {
		t.Errorf("expected wenyan prompt, got: %s", sysContent)
	}
	if !strings.Contains(sysContent, "YAGNI extremist") {
		t.Errorf("expected ponytail ultra prompt, got: %s", sysContent)
	}

	// 2. Claude format injection
	claudeBody := map[string]any{
		"model":    "claude-3-opus",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	}
	srv.applyTokenSaver(claudeBody, "claude", "qoder")
	sys, ok := claudeBody["system"].(string)
	if !ok || !strings.Contains(sys, "文言文") {
		t.Errorf("expected claude system prompt to contain wenyan, got: %v", sys)
	}

	// 3. Gemini format injection
	geminiBody := map[string]any{
		"model":    "gemini-2.0-flash",
		"contents": []any{map[string]any{"parts": []any{map[string]any{"text": "hi"}}}},
	}
	srv.applyTokenSaver(geminiBody, "gemini", "gemini")
	sysInst, ok := geminiBody["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatalf("expected systemInstruction in gemini body, got: %v", geminiBody)
	}
	parts := sysInst["parts"].([]any)
	text := parts[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "文言文") {
		t.Errorf("expected gemini systemInstruction to contain wenyan, got: %s", text)
	}

	// 4. Case-insensitive provider exclusion
	dsBody := map[string]any{
		"model": "deepseek-reasoner",
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
		},
	}
	srv.applyTokenSaver(dsBody, "openai", "deepseek")
	if len(dsBody["messages"].([]any)) != 1 {
		t.Errorf("expected case-insensitive deepseek exclusion to skip injection")
	}

	// 5. Model prefix exclusion (e.g. anthropic/claude-3-5-sonnet)
	prefixBody := map[string]any{
		"model": "anthropic/claude-3-5-sonnet",
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
		},
	}
	srv.applyTokenSaver(prefixBody, "openai", "openrouter")
	if len(prefixBody["messages"].([]any)) != 1 {
		t.Errorf("expected model prefix exclusion to skip injection")
	}
}

func TestApplyTokenSaver_HeaderOverride(t *testing.T) {
	srv, database := setupTestServer(t)

	// 1. Settings has RTKEnabled = false, but request has X-Cyrene-Tool-Slimmer = 1
	st, _ := database.GetSettings()
	st.RTKEnabled = false
	st.CavemanEnabled = false
	st.PonytailEnabled = false
	database.SaveSettings(st)

	var lines []string
	for range 300 {
		lines = append(lines, "overflow log line "+strings.Repeat("x", 20))
	}
	content := strings.Join(lines, "\n")

	reqEnable := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	reqEnable.Header.Set("X-Cyrene-Tool-Slimmer", "1")

	body1 := map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			map[string]any{"role": "tool", "content": content},
		},
	}
	srv.applyTokenSaver(body1, "openai", "openai", reqEnable)
	res1 := body1["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(res1, "lines omitted") {
		t.Errorf("expected X-Cyrene-Tool-Slimmer: 1 to force-enable RTK compression")
	}

	// 2. Settings has RTKEnabled = true, but request has X-Cyrene-Tool-Slimmer = 0
	st.RTKEnabled = true
	database.SaveSettings(st)

	reqDisable := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	reqDisable.Header.Set("X-Cyrene-Tool-Slimmer", "0")

	body2 := map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			map[string]any{"role": "tool", "content": content},
		},
	}
	srv.applyTokenSaver(body2, "openai", "openai", reqDisable)
	res2 := body2["messages"].([]any)[0].(map[string]any)["content"].(string)
	if strings.Contains(res2, "lines omitted") {
		t.Errorf("expected X-Cyrene-Tool-Slimmer: 0 to force-disable RTK compression")
	}
}

func TestMessagesPassthrough_TokenSaverIntegration(t *testing.T) {
	var capturedBody string
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":10,"output_tokens":5}}`))
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)
	st, _ := database.GetSettings()
	st.RTKEnabled = true
	database.SaveSettings(st)

	conn := &model.ProviderConnection{
		ID:       "ant-native",
		Provider: "anthropic",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-ant-test",
			BaseURL: mockUpstream.URL,
		},
	}
	if err := database.CreateConnection(conn); err != nil {
		t.Fatal(err)
	}

	var lines []string
	for i := range 300 {
		lines = append(lines, "stack trace line "+strconv.Itoa(i)+" with lots of output padding")
	}
	historicalOutput := strings.Join(lines, "\n")

	reqMap := map[string]any{
		"model":      "anthropic/claude-sonnet-4-20250514",
		"max_tokens": 100,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type":    "tool_result",
						"content": historicalOutput,
					},
				},
			},
			map[string]any{
				"role":    "assistant",
				"content": "got it",
			},
			map[string]any{
				"role":    "user",
				"content": "now continue",
			},
		},
	}
	reqBytes, _ := json.Marshal(reqMap)

	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(reqBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if !strings.Contains(capturedBody, "historical tool output omitted by Cyrene Gateway") {
		t.Errorf("expected native messages passthrough to execute RTK compression on historical tool_result, body: %s", capturedBody)
	}
}
