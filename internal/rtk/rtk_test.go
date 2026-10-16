package rtk

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
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

func TestCompressMessages_UTF8RuneBoundarySafe(t *testing.T) {
	// Generate a single-line string with multi-byte Chinese runes that exceeds 16 KiB
	var sb strings.Builder
	for sb.Len() < 20000 {
		sb.WriteString("中文测试超长输出测试，用于验证字符截断时不会截断多字节编码。")
	}
	raw := sb.String()
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "tool", "content": raw},
		},
	}

	saved := CompressMessages(body, true)
	if saved <= 0 {
		t.Fatalf("expected savings, got %d", saved)
	}
	compressed := body["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !utf8.ValidString(compressed) {
		t.Fatal("compressed string contains invalid UTF-8 bytes")
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

func TestCleanAndCompactText(t *testing.T) {
	// 1. Test ANSI code stripping
	ansiText := "\x1b[32mPASS\x1b[0m tests/unit.test.ts\n\x1b[1;31mFAIL\x1b[0m tests/e2e.test.ts\x1b[?25h"
	cleanedAnsi := cleanAndCompactText(ansiText)
	if strings.Contains(cleanedAnsi, "\x1b[") {
		t.Errorf("expected ANSI codes to be stripped, got: %q", cleanedAnsi)
	}
	if !strings.Contains(cleanedAnsi, "PASS tests/unit.test.ts") {
		t.Errorf("expected clean text preserved, got: %q", cleanedAnsi)
	}

	// 2. Test multi-newline collapsing
	multiNL := "line 1\n\n\n\n\nline 2\n\n\nline 3"
	collapsed := cleanAndCompactText(multiNL)
	if strings.Contains(collapsed, "\n\n\n") {
		t.Errorf("expected redundant newlines to be collapsed, got: %q", collapsed)
	}
	if collapsed != "line 1\n\nline 2\n\nline 3" {
		t.Errorf("unexpected collapsed result: %q", collapsed)
	}

	// 3. Test JSON compaction
	prettyJSON := "{\n  \"status\": \"ok\",\n  \"data\": {\n    \"count\": 42,\n    \"items\": [\n      1,\n      2\n    ]\n  }\n}"
	compacted := cleanAndCompactText(prettyJSON)
	expected := `{"status":"ok","data":{"count":42,"items":[1,2]}}`
	if compacted != expected {
		t.Errorf("expected compacted JSON %q, got %q", expected, compacted)
	}
}

func TestCompressMessages_JSONCompactedWithoutTruncation(t *testing.T) {
	// A pretty-printed JSON of ~800 bytes with ~30 lines:
	// Larger than minCompressSize (500), but within 250 lines & 16KB.
	// It should be losslessly compacted into 1 line without any omitted marker.
	var builder strings.Builder
	builder.WriteString("[\n")
	for i := range 15 {
		builder.WriteString("  {\n")
		builder.WriteString("    \"id\": " + strconv.Itoa(i) + ",\n")
		builder.WriteString("    \"description\": \"this is a test entry with some padding\",\n")
		builder.WriteString("    \"active\": true\n")
		if i < 14 {
			builder.WriteString("  },\n")
		} else {
			builder.WriteString("  }\n")
		}
	}
	builder.WriteString("]\n")

	raw := builder.String()
	if len(raw) < 500 {
		t.Fatalf("test setup error: raw JSON must be > 500 bytes, got %d", len(raw))
	}

	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "tool", "content": raw},
		},
	}
	saved := CompressMessages(body, true)
	if saved <= 0 {
		t.Errorf("expected positive savings from JSON compaction, got %d", saved)
	}

	res := body["messages"].([]any)[0].(map[string]any)["content"].(string)
	if strings.Contains(res, "omitted") {
		t.Errorf("expected lossless compaction WITHOUT truncation, but got omitted marker: %s", res)
	}
	if strings.Contains(res, "\n") {
		t.Errorf("expected single-line compacted JSON, but found newlines: %s", res)
	}
}

func TestCompressMessages_TurnAware_ProtectsLatestTurn(t *testing.T) {
	// Build 150 lines of tool content.
	// 150 lines is >80 lines (so historical turns will be truncated),
	// but <=250 lines (so the latest turn is protected and NOT truncated).
	var lines []string
	for i := range 150 {
		lines = append(lines, "content line "+strconv.Itoa(i)+" with padding text to exceed minimum size limit")
	}
	content150 := strings.Join(lines, "\n")

	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "fix bug"},
			map[string]any{"role": "assistant", "content": "running test"},
			map[string]any{"role": "tool", "content": content150}, // Turn 1 (Historical)
			map[string]any{"role": "assistant", "content": "reading file"},
			map[string]any{"role": "tool", "content": content150}, // Turn 2 (Latest active turn!)
		},
	}

	saved := CompressMessages(body, true)
	if saved <= 0 {
		t.Fatalf("expected savings from historical turn compression, got %d", saved)
	}

	msgs := body["messages"].([]any)
	histTool := msgs[2].(map[string]any)["content"].(string)
	latestTool := msgs[4].(map[string]any)["content"].(string)

	// Historical tool MUST be collapsed with the historical marker
	if !strings.Contains(histTool, "historical tool output omitted by Cyrene Gateway") {
		t.Errorf("expected historical tool to contain omission marker, got: %s", histTool)
	}

	// Latest tool MUST be preserved 100% intact without any truncation!
	if strings.Contains(latestTool, "omitted") {
		t.Errorf("latest active tool was incorrectly truncated: %s", latestTool)
	}
	if latestTool != content150 {
		t.Errorf("latest active tool content was altered, expected %d bytes, got %d bytes", len(content150), len(latestTool))
	}
}

func TestCompressMessages_TurnAware_AnthropicErrors(t *testing.T) {
	var lines []string
	for i := range 150 {
		lines = append(lines, "stack trace line "+strconv.Itoa(i)+" at github.com/arisvia/gateway/test.go:42")
	}
	errorContent := strings.Join(lines, "\n")

	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "tool_result", "is_error": true, "content": errorContent}, // Turn 1
				},
			},
			map[string]any{"role": "assistant", "content": "analyzing previous error"},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "tool_result", "is_error": true, "content": errorContent}, // Turn 2 (Latest!)
				},
			},
		},
	}

	saved := CompressMessages(body, true)
	if saved <= 0 {
		t.Fatalf("expected savings from historical error compression, got %d", saved)
	}

	msgs := body["messages"].([]any)
	histBlock := msgs[0].(map[string]any)["content"].([]any)[0].(map[string]any)["content"].(string)
	latestBlock := msgs[2].(map[string]any)["content"].([]any)[0].(map[string]any)["content"].(string)

	// Historical error is collapsed to save prompt tokens
	if !strings.Contains(histBlock, "historical tool output omitted by Cyrene Gateway") {
		t.Errorf("expected historical error to be collapsed, got: %s", histBlock)
	}

	// Latest error MUST NOT be collapsed so the agent can debug the current failure
	if strings.Contains(latestBlock, "omitted") {
		t.Errorf("latest active error was incorrectly truncated: %s", latestBlock)
	}
}
