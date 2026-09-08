package rtk

import (
	"strconv"
	"strings"
	"testing"
)

func TestCompressMessages_Disabled(t *testing.T) {
	body := map[string]any{"messages": []any{}}
	saved := CompressMessages(body, false)
	if saved != 0 {
		t.Errorf("expected 0 saved when disabled, got %d", saved)
	}
}

func TestCompressMessages_NilBody(t *testing.T) {
	saved := CompressMessages(nil, true)
	if saved != 0 {
		t.Errorf("expected 0 saved for nil body, got %d", saved)
	}
}

func TestCompressMessages_ShortToolContent(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "tool", "content": "short output"},
		},
	}
	saved := CompressMessages(body, true)
	if saved != 0 {
		t.Errorf("expected 0 saved for short content, got %d", saved)
	}
}

func TestCompressMessages_LargeToolContent(t *testing.T) {
	// Build a tool message with >250 lines
	var lines []string
	for i := range 400 {
		lines = append(lines, "line "+strconv.Itoa(i)+" with some padding content to make it longer than minimum")
	}
	content := strings.Join(lines, "\n")

	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "tool", "content": content},
		},
	}
	saved := CompressMessages(body, true)
	if saved <= 0 {
		t.Errorf("expected positive savings for large tool content, got %d", saved)
	}

	// Verify the compressed content contains the omission marker
	msg := body["messages"].([]any)[0].(map[string]any)
	compressed := msg["content"].(string)
	if !strings.Contains(compressed, "lines omitted") {
		t.Error("expected omission marker in compressed content")
	}
}

func TestCompressMessages_ToolResultBlock(t *testing.T) {
	var lines []string
	for i := range 300 {
		lines = append(lines, "output line "+strconv.Itoa(i)+" padding padding padding padding")
	}
	content := strings.Join(lines, "\n")

	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "tool_result", "content": content},
				},
			},
		},
	}
	saved := CompressMessages(body, true)
	if saved <= 0 {
		t.Errorf("expected positive savings for tool_result block, got %d", saved)
	}
}

func TestCompressMessages_ErrorBlockPreserved(t *testing.T) {
	var lines []string
	for i := range 300 {
		lines = append(lines, "error line "+strconv.Itoa(i)+" padding padding padding padding")
	}
	content := strings.Join(lines, "\n")

	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "tool_result", "is_error": true, "content": content},
				},
			},
		},
	}
	saved := CompressMessages(body, true)
	if saved != 0 {
		t.Errorf("expected 0 saved for error block (preserved), got %d", saved)
	}
}

func TestInjectCaveman_OpenAI(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": "You are helpful."},
			map[string]any{"role": "user", "content": "Hello"},
		},
	}
	InjectCaveman(body, "openai", CavemanLite)

	msgs := body["messages"].([]any)
	sysMsg := msgs[0].(map[string]any)
	content := sysMsg["content"].(string)
	if !strings.Contains(content, "Respond tersely") {
		t.Error("expected caveman prompt appended to system message")
	}
	if !strings.Contains(content, "You are helpful.") {
		t.Error("expected original system content preserved")
	}
}

func TestInjectCaveman_NoSystemMessage(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "Hello"},
		},
	}
	InjectCaveman(body, "openai", CavemanFull)

	msgs := body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after injection, got %d", len(msgs))
	}
	sysMsg := msgs[0].(map[string]any)
	if sysMsg["role"] != "system" {
		t.Error("expected prepended system message")
	}
	content := sysMsg["content"].(string)
	if !strings.Contains(content, "caveman") {
		t.Error("expected caveman prompt in new system message")
	}
}

func TestInjectPonytail_OpenAI(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "Write code"},
		},
	}
	InjectPonytail(body, "openai", PonytailUltra)

	msgs := body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	sysMsg := msgs[0].(map[string]any)
	content := sysMsg["content"].(string)
	if !strings.Contains(content, "lazy senior developer") {
		t.Error("expected ponytail prompt")
	}
}

func TestInjectCaveman_Claude(t *testing.T) {
	body := map[string]any{
		"system": "Be helpful.",
	}
	InjectCaveman(body, "anthropic", CavemanUltra)

	sys := body["system"].(string)
	if !strings.Contains(sys, "Be helpful.") {
		t.Error("expected original system preserved")
	}
	if !strings.Contains(sys, "ultra-terse") {
		t.Error("expected caveman ultra prompt")
	}
}

func TestInjectCaveman_Gemini(t *testing.T) {
	body := map[string]any{
		"systemInstruction": map[string]any{
			"parts": []any{map[string]any{"text": "Be nice."}},
		},
	}
	InjectCaveman(body, "gemini", CavemanLite)

	sys := body["systemInstruction"].(map[string]any)
	parts := sys["parts"].([]any)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	lastPart := parts[1].(map[string]any)
	if !strings.Contains(lastPart["text"].(string), "tersely") {
		t.Error("expected caveman prompt in gemini system parts")
	}
}

func TestInjectSystemPrompt_InvalidLevel(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "Hello"},
		},
	}
	// Invalid level should be a no-op
	InjectCaveman(body, "openai", "nonexistent")
	msgs := body["messages"].([]any)
	if len(msgs) != 1 {
		t.Error("expected no injection for invalid level")
	}
}

func TestAllCavemanLevelsExist(t *testing.T) {
	levels := []string{CavemanLite, CavemanFull, CavemanUltra, CavemanWenyanLite, CavemanWenyan, CavemanWenyanUltra}
	for _, l := range levels {
		if _, ok := CavemanPrompts[l]; !ok {
			t.Errorf("missing caveman prompt for level %q", l)
		}
	}
}

func TestAllPonytailLevelsExist(t *testing.T) {
	levels := []string{PonytailLite, PonytailFull, PonytailUltra}
	for _, l := range levels {
		if _, ok := PonytailPrompts[l]; !ok {
			t.Errorf("missing ponytail prompt for level %q", l)
		}
	}
}

func TestCompressMessages_SingleLineLargeContent(t *testing.T) {
	// Massive single line (e.g. minified JSON > 16KB)
	content := strings.Repeat("{\"key\": \"very large value that repeats over and over again\"},", 400)
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "tool", "content": content},
		},
	}
	saved := CompressMessages(body, true)
	if saved <= 0 {
		t.Errorf("expected positive savings for single-line large tool output, got %d", saved)
	}
	compressed := body["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(compressed, "characters omitted") {
		t.Errorf("expected 'characters omitted' marker, got %s", compressed)
	}
}

func TestCompressMessages_GeminiContents(t *testing.T) {
	var lines []string
	for i := range 300 {
		lines = append(lines, "gemini output line "+strconv.Itoa(i)+" "+strings.Repeat("x", 40))
	}
	content := strings.Join(lines, "\n")

	body := map[string]any{
		"contents": []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{
						"functionResponse": map[string]any{
							"name": "search",
							"response": map[string]any{
								"content": content,
							},
						},
					},
				},
			},
		},
	}
	saved := CompressMessages(body, true)
	if saved <= 0 {
		t.Errorf("expected positive savings for Gemini contents, got %d", saved)
	}
}

func TestInjectSystemPrompt_Idempotency(t *testing.T) {
	bodyOpenAI := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": "You are helpful."},
		},
	}
	InjectCaveman(bodyOpenAI, "openai", CavemanLite)
	first := bodyOpenAI["messages"].([]any)[0].(map[string]any)["content"].(string)

	// Injecting again should be a no-op
	InjectCaveman(bodyOpenAI, "openai", CavemanLite)
	second := bodyOpenAI["messages"].([]any)[0].(map[string]any)["content"].(string)
	if first != second {
		t.Errorf("expected idempotent injection for OpenAI, but content changed: %s vs %s", first, second)
	}

	bodyClaude := map[string]any{
		"system": "Be concise.",
	}
	InjectPonytail(bodyClaude, "anthropic", PonytailLite)
	firstClaude := bodyClaude["system"].(string)
	InjectPonytail(bodyClaude, "anthropic", PonytailLite)
	secondClaude := bodyClaude["system"].(string)
	if firstClaude != secondClaude {
		t.Errorf("expected idempotent injection for Claude, but content changed")
	}

	bodyResponses := map[string]any{
		"instructions": "Be helpful.",
	}
	InjectCaveman(bodyResponses, "openai", CavemanLite)
	firstResp := bodyResponses["instructions"].(string)
	InjectCaveman(bodyResponses, "openai", CavemanLite)
	secondResp := bodyResponses["instructions"].(string)
	if firstResp != secondResp {
		t.Errorf("expected idempotent injection for instructions field, but content changed")
	}
}
