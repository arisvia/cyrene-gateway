package metrics

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestObserveRequestAndHandler(t *testing.T) {
	m := New("test-1.2.3")
	m.ObserveRequest("anthropic", "claude-sonnet-4-5", "/v1/chat/completions", 200, 1.5, &Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		CachedTokens:     20,
		ReasoningTokens:  10,
	})
	m.ObserveRequest("openai", "gpt-5", "/v1/chat/completions", 502, 0.2, nil)

	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	out := string(body)

	for _, want := range []string{
		`cyrene_requests_total{endpoint="/v1/chat/completions",model="claude-sonnet-4-5",provider="anthropic",status="200"} 1`,
		`cyrene_requests_total{endpoint="/v1/chat/completions",model="gpt-5",provider="openai",status="502"} 1`,
		`cyrene_tokens_total{provider="anthropic",type="prompt"} 100`,
		`cyrene_tokens_total{provider="anthropic",type="completion"} 50`,
		`cyrene_tokens_total{provider="anthropic",type="cached"} 20`,
		`cyrene_tokens_total{provider="anthropic",type="reasoning"} 10`,
		`cyrene_build_info{version="test-1.2.3"} 1`,
		`cyrene_request_duration_seconds`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics output missing %q", want)
		}
	}
}
