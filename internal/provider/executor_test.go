package provider

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestSanitizeCodebuddyRequest_CN(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "Hello"},
		},
		"reasoning_effort": "high",
	}

	sanitizeCodebuddyRequest("codebuddy-cn", body)

	if body["stream"] != true {
		t.Errorf("expected stream=true, got %v", body["stream"])
	}
	if body["reasoning_summary"] != "auto" {
		t.Errorf("expected reasoning_summary=auto, got %v", body["reasoning_summary"])
	}
	msgs, ok := body["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %v", msgs)
	}
	first := msgs[0].(map[string]any)
	if first["role"] != "system" || first["content"] != "You are CodeBuddy Code." {
		t.Errorf("expected leading system message, got %+v", first)
	}

	// Test agent prompt replacement
	bodyWithAgent := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": "You are Claude Code, Anthropic's official CLI tool."},
			map[string]any{"role": "user", "content": "List files"},
		},
	}
	sanitizeCodebuddyRequest("codebuddy-cn", bodyWithAgent)
	agentMsgs := bodyWithAgent["messages"].([]any)
	sysMsg := agentMsgs[0].(map[string]any)
	if sysMsg["content"] != neutralCodebuddyPrompt {
		t.Errorf("expected neutral prompt replacement, got %v", sysMsg["content"])
	}
}

func TestSanitizeCodebuddyRequest_Intl(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "developer", "content": "Developer instructions"},
			map[string]any{"role": "user", "content": "Please review this code"},
		},
	}

	sanitizeCodebuddyRequest("codebuddy", body)

	msgs, ok := body["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("expected 2 messages after hoisting developer text into the leading system, got %v", msgs)
	}
	// The gateway demands a leading system message, not a specific sentence: the
	// caller's own instructions survive, only relocated into the first slot.
	first := msgs[0].(map[string]any)
	if first["role"] != "system" || first["content"] != "Developer instructions" {
		t.Errorf("expected caller system text preserved, got %+v", first)
	}
	userMsg := msgs[1].(map[string]any)
	contentArr, ok := userMsg["content"].([]any)
	if !ok || len(contentArr) != 1 {
		t.Fatalf("expected typed text block array, got %v", userMsg["content"])
	}
	block := contentArr[0].(map[string]any)
	if block["type"] != "text" || block["text"] != "Please review this code" {
		t.Errorf("unexpected content block: %+v", block)
	}
}

func TestSanitizeCodebuddyRequest_IntlIdentityAndFallback(t *testing.T) {
	// A coding-agent fingerprint is the part upstream actually rejects, so it is
	// replaced while the rest of the caller's text is kept.
	agent := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": "You are Claude Code, Anthropic's official CLI for Claude. Be terse."},
			map[string]any{"role": "user", "content": "List files"},
		},
	}
	sanitizeCodebuddyRequest("codebuddy", agent)
	agentMsgs := agent["messages"].([]any)
	if got := agentMsgs[0].(map[string]any)["content"]; got != neutralCodebuddyPrompt {
		t.Errorf("agent fingerprint not neutralized: %v", got)
	}

	// No caller prompt at all: the shape requirement still needs a leading system.
	bare := map[string]any{
		"messages": []any{map[string]any{"role": "user", "content": "Hello"}},
	}
	sanitizeCodebuddyRequest("codebuddy", bare)
	bareMsgs := bare["messages"].([]any)
	if len(bareMsgs) != 2 {
		t.Fatalf("expected a prepended system message, got %v", bareMsgs)
	}
	if first := bareMsgs[0].(map[string]any); first["role"] != "system" || first["content"] != codebuddyIdentityPrompt {
		t.Errorf("expected identity fallback, got %+v", first)
	}

	// Multiple system/developer messages join instead of being discarded.
	multi := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": "Project conventions"},
			map[string]any{"role": "developer", "content": "Reply in Chinese"},
			map[string]any{"role": "user", "content": "Hi"},
		},
	}
	sanitizeCodebuddyRequest("codebuddy", multi)
	if got := multi["messages"].([]any)[0].(map[string]any)["content"]; got != "Project conventions\n\nReply in Chinese" {
		t.Errorf("expected joined system text, got %v", got)
	}
}

func TestSanitizeCodebuddyRequest_CNTypedBlocks(t *testing.T) {
	// The CN filter reads the same fingerprint whether the client sent a string or
	// typed blocks; matching only the string form let block-shaped agent prompts
	// reach upstream verbatim.
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "system",
				"content": []any{map[string]any{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."}},
			},
			map[string]any{"role": "user", "content": "List files"},
		},
	}

	sanitizeCodebuddyRequest("codebuddy-cn", body)

	sys := body["messages"].([]any)[0].(map[string]any)
	blocks, ok := sys["content"].([]any)
	if !ok {
		t.Fatalf("expected the typed-block shape to be preserved, got %#v", sys["content"])
	}
	if len(blocks) != 1 || blocks[0].(map[string]any)["text"] != neutralCodebuddyPrompt {
		t.Errorf("expected neutral prompt inside typed blocks, got %#v", blocks)
	}

	// The CN length catch-all stays on: an unrecognised agent CLI must not slip by.
	long := strings.Repeat("x", codebuddyCNMaxSystemLen+1)
	longBody := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": long},
			map[string]any{"role": "user", "content": "Hi"},
		},
	}
	sanitizeCodebuddyRequest("codebuddy-cn", longBody)
	if got := longBody["messages"].([]any)[0].(map[string]any)["content"]; got != neutralCodebuddyPrompt {
		t.Errorf("expected over-long system prompt replaced, got %v", got)
	}

	// Intl has no such gate upstream, so a long legitimate prompt survives there.
	intlBody := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": long},
			map[string]any{"role": "user", "content": "Hi"},
		},
	}
	sanitizeCodebuddyRequest("codebuddy", intlBody)
	if got := intlBody["messages"].([]any)[0].(map[string]any)["content"]; got != long {
		t.Errorf("expected long prompt preserved on Intl, got %v", got)
	}
}

func TestSanitizeCodebuddyRequest_SiblingKeepsShape(t *testing.T) {
	// An unlisted codebuddy-* sibling gets WAF neutralization (the part that costs
	// an account) but no shape guessing: neither the Intl leading-system rewrite
	// nor the CN length gate.
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": "You are Cline, an AI coding agent."},
			map[string]any{"role": "user", "content": "Hi"},
		},
	}

	sanitizeCodebuddyRequest("codebuddy-enterprise", body)

	msgs := body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected no message to be injected, got %v", msgs)
	}
	if got := msgs[0].(map[string]any)["content"]; got != neutralCodebuddyPrompt {
		t.Errorf("expected agent fingerprint neutralized, got %v", got)
	}
	if got := msgs[1].(map[string]any)["content"]; got != "Hi" {
		t.Errorf("expected the plain OpenAI shape to survive, got %#v", got)
	}
}

func TestSanitizeGithubRequest(t *testing.T) {
	body := map[string]any{
		"max_tokens":       float64(4096),
		"reasoning_effort": "none",
	}

	sanitizeGithubRequest("gpt-5", body)

	if _, ok := body["max_tokens"]; ok {
		t.Error("max_tokens should have been removed for gpt-5")
	}
	if body["max_completion_tokens"] != float64(4096) {
		t.Errorf("expected max_completion_tokens=4096, got %v", body["max_completion_tokens"])
	}
	if _, ok := body["reasoning_effort"]; ok {
		t.Error("reasoning_effort none should have been removed")
	}
}

func TestSanitizeOpenAIRequest(t *testing.T) {
	body := map[string]any{
		"max_tokens": float64(2048),
	}
	sanitizeOpenAIRequest("gpt-5.6", body)
	if _, ok := body["max_tokens"]; ok {
		t.Error("max_tokens should have been removed for gpt-5.6")
	}
	if body["max_completion_tokens"] != float64(2048) {
		t.Errorf("expected max_completion_tokens=2048, got %v", body["max_completion_tokens"])
	}

	// gpt-4o should leave max_tokens intact
	body4o := map[string]any{
		"max_tokens": float64(1024),
	}
	sanitizeOpenAIRequest("gpt-4o", body4o)
	if body4o["max_tokens"] != float64(1024) {
		t.Errorf("expected max_tokens intact for gpt-4o, got %v", body4o["max_tokens"])
	}
	if _, ok := body4o["max_completion_tokens"]; ok {
		t.Error("max_completion_tokens should not be injected for gpt-4o")
	}
}

func TestClaudeOAuthHeaders(t *testing.T) {
	info, ok := Registry["claude"]
	if !ok {
		t.Fatal("claude provider not found in registry")
	}
	if info.URLSuffix != "?beta=true" {
		t.Errorf("expected URLSuffix=?beta=true, got %q", info.URLSuffix)
	}
	if info.Headers["Anthropic-Beta"] == "" {
		t.Error("expected Anthropic-Beta header in claude registry info")
	}
	if info.Headers["User-Agent"] == "" {
		t.Error("expected User-Agent header in claude registry info")
	}
}

func TestPrepareUpstreamRequest_Codebuddy(t *testing.T) {
	cand := ExecutionCandidate{
		Connection: &model.ProviderConnection{
			ID:       "test-cb",
			Provider: "codebuddy",
			AuthType: "oauth",
			Data: model.ConnectionData{
				AccessToken: "cb-test-token",
			},
		},
		ModelInfo: model.ModelInfo{
			Provider: "codebuddy",
			Model:    "hy4-preview",
		},
		ProviderInfo: Registry["codebuddy"],
	}

	rawBody := []byte(`{"messages":[{"role":"user","content":"Hello"}],"stream":false}`)
	req, format, err := PrepareUpstreamRequest(
		context.Background(),
		ExecutionRequest{RawBody: rawBody, Stream: false},
		cand,
		nil,
	)
	if err != nil {
		t.Fatalf("PrepareUpstreamRequest failed: %v", err)
	}
	if format != "openai" {
		t.Errorf("expected openai format, got %s", format)
	}
	if req.Header.Get("User-Agent") != CodebuddyIDEUA {
		t.Errorf("unexpected User-Agent: %s", req.Header.Get("User-Agent"))
	}
	if req.Header.Get("X-IDE-Type") != "IDE" {
		t.Errorf("intl chat must present the IDE identity, got X-IDE-Type=%q", req.Header.Get("X-IDE-Type"))
	}
	if req.Header.Get("X-Domain") != "www.codebuddy.ai" {
		t.Errorf("unexpected X-Domain: %s", req.Header.Get("X-Domain"))
	}
	convID := req.Header.Get("X-Conversation-Id")
	reqID := req.Header.Get("X-Conversation-Request-Id")
	msgID := req.Header.Get("X-Conversation-Message-Id")
	if convID == "" || reqID == "" || msgID == "" {
		t.Errorf("expected X-Conversation-* headers, got conv=%q req=%q msg=%q", convID, reqID, msgID)
	}

	bodyBytes, _ := io.ReadAll(req.Body)
	var parsed map[string]any
	json.Unmarshal(bodyBytes, &parsed)

	if parsed["stream"] != true {
		t.Errorf("expected stream=true in body, got %v", parsed["stream"])
	}
	msgs := parsed["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	first := msgs[0].(map[string]any)
	if first["role"] != "system" {
		t.Errorf("expected leading system message, got %+v", first)
	}
}

func TestCodebuddyConversationHeaders_StabilityAndPrecedence(t *testing.T) {
	cand := ExecutionCandidate{
		Connection: &model.ProviderConnection{
			ID:       "test-cb",
			Provider: "codebuddy",
			AuthType: "oauth",
			Data:     model.ConnectionData{AccessToken: "token"},
		},
		ModelInfo:    model.ModelInfo{Provider: "codebuddy", Model: "hy4-preview"},
		ProviderInfo: Registry["codebuddy"],
	}

	// 1. Same user content produces stable X-Conversation-Id across retries
	bodyA := []byte(`{"messages":[{"role":"user","content":"Hello world"}]}`)
	reqA, _, _ := PrepareUpstreamRequest(context.Background(), ExecutionRequest{RawBody: bodyA}, cand, nil)
	reqB, _, _ := PrepareUpstreamRequest(context.Background(), ExecutionRequest{RawBody: bodyA}, cand, nil)

	convA := reqA.Header.Get("X-Conversation-Id")
	convB := reqB.Header.Get("X-Conversation-Id")
	if convA == "" || convA != convB {
		t.Errorf("expected stable conversation ID for same content, got convA=%q convB=%q", convA, convB)
	}
	// Request and Message IDs must be fresh
	if reqA.Header.Get("X-Conversation-Request-Id") == reqB.Header.Get("X-Conversation-Request-Id") {
		t.Error("expected fresh request ID per attempt")
	}
	if reqA.Header.Get("X-Conversation-Message-Id") == reqB.Header.Get("X-Conversation-Message-Id") {
		t.Error("expected fresh message ID per attempt")
	}

	// 2. Explicit session_id takes precedence
	bodySess1 := []byte(`{"session_id":"sess-123","messages":[{"role":"user","content":"Hello world"}]}`)
	bodySess2 := []byte(`{"session_id":"sess-456","messages":[{"role":"user","content":"Hello world"}]}`)
	reqS1, _, _ := PrepareUpstreamRequest(context.Background(), ExecutionRequest{RawBody: bodySess1}, cand, nil)
	reqS2, _, _ := PrepareUpstreamRequest(context.Background(), ExecutionRequest{RawBody: bodySess2}, cand, nil)

	convS1 := reqS1.Header.Get("X-Conversation-Id")
	convS2 := reqS2.Header.Get("X-Conversation-Id")
	if convS1 == convA {
		t.Errorf("explicit session_id should take precedence over message content")
	}
	if convS1 == convS2 {
		t.Errorf("different session_ids should produce different conversation IDs")
	}

	// 3. Typed blocks user content
	bodyBlocks := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"Hello world"}]}]}`)
	reqBlocks, _, _ := PrepareUpstreamRequest(context.Background(), ExecutionRequest{RawBody: bodyBlocks}, cand, nil)
	if reqBlocks.Header.Get("X-Conversation-Id") != convA {
		t.Errorf("typed block content should yield same stable ID as string content: got %q, want %q", reqBlocks.Header.Get("X-Conversation-Id"), convA)
	}

	// 4. Empty messages fallback to valid UUID
	bodyEmpty := []byte(`{"messages":[]}`)
	reqEmpty, _, _ := PrepareUpstreamRequest(context.Background(), ExecutionRequest{RawBody: bodyEmpty}, cand, nil)
	if reqEmpty.Header.Get("X-Conversation-Id") == "" {
		t.Errorf("expected fallback conversation ID for empty messages")
	}

	// 5. Explicit client header preservation
	reqWithHeader, _, _ := PrepareUpstreamRequest(context.Background(), ExecutionRequest{RawBody: bodyA}, cand, nil)
	reqWithHeader.Header.Set("X-Conversation-Id", "custom-client-conversation-123")
	applyCodebuddyConversationHeaders(reqWithHeader, map[string]any{"session_id": "other-session"})
	if got := reqWithHeader.Header.Get("X-Conversation-Id"); got != "custom-client-conversation-123" {
		t.Errorf("expected custom client conversation ID preserved, got %q", got)
	}
}
