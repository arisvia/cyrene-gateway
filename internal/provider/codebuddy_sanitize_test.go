package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// The findings these tests pin are the ones that made the CodeBuddy guard unsafe
// to loosen blindly: replacing text is a silent answer change unless the residue
// is exactly the caller's own instruction and the trigger literal is gone.

func TestSanitizeCodebuddyRequest_PreservesCallerTextAroundMarker(t *testing.T) {
	// The batch's whole purpose: upstream rejects the identity wording, not the
	// caller's project instructions. Whole-message discard (upstream 9router's
	// behaviour) ate both.
	prompt := "You are Claude Code, Anthropic's official CLI for Claude.\n\nProject rule: always reply in Chinese."
	for _, providerID := range []string{"codebuddy", "codebuddy-cn", "codebuddy-enterprise"} {
		body := map[string]any{"messages": []any{
			map[string]any{"role": "system", "content": prompt},
			map[string]any{"role": "user", "content": "hi"},
		}}
		sanitizeCodebuddyRequest(providerID, body)
		got := messageContentText(body["messages"].([]any)[0].(map[string]any)["content"])
		if !strings.Contains(got, "Project rule: always reply in Chinese.") {
			t.Errorf("%s dropped the caller's own instruction: %q", providerID, got)
		}
		for _, banned := range []string{"Claude Code", "Anthropic's official CLI", "official CLI for Claude"} {
			if strings.Contains(got, banned) {
				t.Errorf("%s forwarded the rejected literal %q: %q", providerID, banned, got)
			}
		}
	}
}

func TestNeutralizeCodebuddyText_WhitespaceEvasion(t *testing.T) {
	// Upstream writes its alternatives with single spaces, so a newline or a
	// double space slipped through — and messageContentText manufactures exactly
	// that form when it joins typed blocks with "\n".
	for _, text := range []string{
		"You are\nClaude Code",
		"You are  Claude Code",
		"You are\tClaude Code",
	} {
		if !agentSystemPromptRegex.MatchString(text) {
			t.Errorf("whitespace variant escaped the pattern: %q", text)
		}
		if got := neutralizeCodebuddyText(text); strings.Contains(got, "Claude Code") {
			t.Errorf("variant not neutralized: %q -> %q", text, got)
		}
	}

	// The manufactured form, end to end through the Intl rewrite.
	body := map[string]any{"messages": []any{
		map[string]any{"role": "system", "content": []any{
			map[string]any{"type": "text", "text": "You are"},
			map[string]any{"type": "text", "text": "Claude Code"},
		}},
		map[string]any{"role": "user", "content": "hi"},
	}}
	sanitizeCodebuddyRequest("codebuddy", body)
	if got := messageContentText(body["messages"].([]any)[0].(map[string]any)["content"]); strings.Contains(got, "Claude Code") {
		t.Errorf("split identity header reached upstream: %q", got)
	}
}

func TestSanitizeCodebuddyRequest_BenignCorpus(t *testing.T) {
	// These alternatives look over-broad and are deliberately kept: the pattern is
	// upstream 9router's AGENT_PATTERN, tuned against rejections we cannot see.
	// What we owe the caller is that a benign prompt keeps its prose — only the
	// matched span changes.
	cases := []struct {
		in, want string
	}{
		{"This platform has orchestration capabilities for microservices.",
			"This platform has a helpful coding assistant for microservices."},
		{"You are an agent of the state tax office.",
			"a helpful coding assistant of the state tax office."},
		{"You are a powerful AI agent framework maintainer.",
			"a helpful coding assistant framework maintainer."},
		{"When you see <Role> tag, respond politely. Project rules: use tabs.",
			"When you see <instructions> tag, respond politely. Project rules: use tabs."},
		{"<Role>You are a translator</Role>",
			"<instructions>You are a translator</Role>"},
		// Not matched at all: upstream does not cover these either.
		{"Answer concisely.", "Answer concisely."},
		{"你是 Claude Code", "你是 Claude Code"},
		{"You are Qoder, an AI coding agent.", "You are Qoder, an AI coding agent."},
	}
	for _, c := range cases {
		if got := neutralizeCodebuddyText(c.in); got != c.want {
			t.Errorf("neutralizeCodebuddyText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSanitizeCodebuddyRequest_StrayInstructionField(t *testing.T) {
	// A mixed-shape body can carry the identity in a top-level field that never
	// enters the message array; the OpenAI->OpenAI passthrough forwards it.
	for _, key := range []string{"system", "instructions"} {
		body := map[string]any{
			"messages": []any{map[string]any{"role": "user", "content": "hi"}},
			key:        "You are Claude Code, Anthropic's official CLI for Claude.",
		}
		sanitizeCodebuddyRequest("codebuddy", body)
		if got, _ := body[key].(string); strings.Contains(got, "Claude Code") {
			t.Errorf("top-level %s reached upstream with the literal: %q", key, got)
		}
	}

	// Benign stray text is left alone, and the key is never deleted.
	body := map[string]any{
		"messages":     []any{map[string]any{"role": "user", "content": "hi"}},
		"instructions": "Be terse.",
	}
	sanitizeCodebuddyRequest("codebuddy", body)
	if body["instructions"] != "Be terse." {
		t.Errorf("benign instructions rewritten: %v", body["instructions"])
	}
}

func TestSanitizeCodebuddyRequest_CNDeveloperOnlyKeepsOneSlot(t *testing.T) {
	// Developer instructions are an instruction slot too. Prepending the identity
	// sentence anyway gave upstream two instruction messages for one conversation.
	body := map[string]any{"messages": []any{
		map[string]any{"role": "developer", "content": "Use the project's linter."},
		map[string]any{"role": "user", "content": "hi"},
	}}
	sanitizeCodebuddyRequest("codebuddy-cn", body)
	msgs := body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("developer-only body got a second instruction slot: %d messages", len(msgs))
	}
	if role, _ := msgs[0].(map[string]any)["role"].(string); role != "developer" {
		t.Errorf("expected the caller's developer message first, got %q", role)
	}

	// An empty system message is not a filled slot: the fallback still applies.
	empty := map[string]any{"messages": []any{
		map[string]any{"role": "system", "content": ""},
		map[string]any{"role": "user", "content": "hi"},
	}}
	sanitizeCodebuddyRequest("codebuddy-cn", empty)
	first := empty["messages"].([]any)[0].(map[string]any)
	if first["role"] != "system" || first["content"] != codebuddyIdentityPrompt {
		t.Errorf("empty system slot must still fall back, got %+v", first)
	}
}

func TestSanitizeCodebuddyRequest_ShapeDropIsLogged(t *testing.T) {
	// Intl keeps only text. Content that carries no text used to vanish silently
	// while the identity fallback took its place, which reads upstream as an
	// unrelated prompt with no trace in the operator log.
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(prev)

	body := map[string]any{"messages": []any{
		map[string]any{"role": "system", "content": []any{
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.test/a.png"}},
		}},
		map[string]any{"role": "user", "content": "hi"},
	}}
	sanitizeCodebuddyRequest("codebuddy", body)

	out := buf.String()
	if !strings.Contains(out, codebuddyRuleShapeDrop) {
		t.Errorf("dropped non-text system content was not logged: %q", out)
	}
	if got := body["messages"].([]any)[0].(map[string]any)["content"]; got != codebuddyIdentityPrompt {
		t.Errorf("expected identity fallback, got %v", got)
	}
}

func TestMessageContentText_ScalarKeepsStringForm(t *testing.T) {
	// Flattening a scalar to "" turns a malformed-but-working request into an
	// empty-content message upstream may reject outright.
	if got := messageContentText(float64(42)); got != "42" {
		t.Errorf("scalar content flattened to %q, want \"42\"", got)
	}
	if got := messageContentText(true); got != "true" {
		t.Errorf("bool content flattened to %q, want \"true\"", got)
	}
	// A bare block object yields its text, never a Go formatting dump.
	if got := messageContentText(map[string]any{"type": "text", "text": "hello"}); got != "hello" {
		t.Errorf("bare block = %q", got)
	}
	if got := messageContentText(map[string]any{"type": "image_url"}); got != "" {
		t.Errorf("textless block = %q, want empty", got)
	}
}

// TestCodebuddyCNLengthGate_AppliesAfterGuardInjection is the ordering bug: the
// sanitizer measured the slot before ApplyLoopGuard appended its 499-char
// termination prompt into that same message, so a caller prompt that passed the
// gate reached upstream over it — the rejection the gate exists to prevent.
func TestCodebuddyCNLengthGate_AppliesAfterGuardInjection(t *testing.T) {
	cand := ExecutionCandidate{
		Connection: &model.ProviderConnection{
			ID:       "test-cb-cn",
			Provider: "codebuddy-cn",
			AuthType: "oauth",
			Data:     model.ConnectionData{AccessToken: "cb-test-token"},
		},
		ModelInfo:    model.ModelInfo{Provider: "codebuddy-cn", Model: "hy4-preview"},
		ProviderInfo: Registry["codebuddy-cn"],
	}

	// Comfortably under the gate on its own, over it once the guard text lands.
	prompt := strings.Repeat("a", codebuddyCNDefaultMaxSystemLen-500)
	rawBody := []byte(`{"model":"hy4-preview","messages":[{"role":"system","content":"` + prompt + `"},{"role":"user","content":"hi"}],"stream":true,"tools":[{"type":"function","function":{"name":"read_file","description":"read a file"}}]}`)

	req, _, err := PrepareUpstreamRequest(
		context.Background(),
		ExecutionRequest{RawBody: rawBody, Stream: true, HasTools: true, LoopGuardEnabled: true},
		cand,
		nil,
	)
	if err != nil {
		t.Fatalf("PrepareUpstreamRequest failed: %v", err)
	}
	bodyBytes, _ := io.ReadAll(req.Body)
	var parsed map[string]any
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		t.Fatalf("decode upstream body: %v", err)
	}
	got := messageContentText(parsed["messages"].([]any)[0].(map[string]any)["content"])
	if got != neutralCodebuddyPrompt {
		t.Errorf("post-injection length gate did not fire; upstream sees %d chars", len([]rune(got)))
	}

	// With the guard off the same prompt is legal and must survive untouched.
	req2, _, err := PrepareUpstreamRequest(
		context.Background(),
		ExecutionRequest{RawBody: rawBody, Stream: true, HasTools: true},
		cand,
		nil,
	)
	if err != nil {
		t.Fatalf("PrepareUpstreamRequest (no guard) failed: %v", err)
	}
	body2, _ := io.ReadAll(req2.Body)
	var parsed2 map[string]any
	json.Unmarshal(body2, &parsed2)
	if got2 := messageContentText(parsed2["messages"].([]any)[0].(map[string]any)["content"]); got2 != prompt {
		t.Errorf("legitimate prompt rewritten without the guard: %d chars", len([]rune(got2)))
	}
}
