package handler

import (
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/db"
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
