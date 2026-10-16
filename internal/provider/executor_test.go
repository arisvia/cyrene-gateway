package provider

import (
	"context"
	"encoding/json"
	"io"
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
		t.Fatalf("expected 2 messages after developer prompt filtering and system prompt prepend, got %v", msgs)
	}
	first := msgs[0].(map[string]any)
	if first["role"] != "system" || first["content"] != "You are CodeBuddy Code." {
		t.Errorf("expected leading system message, got %+v", first)
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
	if req.Header.Get("User-Agent") != "CLI/2.108.1 CodeBuddy/2.108.1" {
		t.Errorf("unexpected User-Agent: %s", req.Header.Get("User-Agent"))
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
}
