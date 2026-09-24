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

	sanitizeCodebuddyRequest("codebuddy-intl", body)

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
			Provider: "codebuddy-intl",
			AuthType: "oauth",
			Data: model.ConnectionData{
				AccessToken: "cb-test-token",
			},
		},
		ModelInfo: model.ModelInfo{
			Provider: "codebuddy-intl",
			Model:    "hy4-preview",
		},
		ProviderInfo: Registry["codebuddy-intl"],
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
