package usage

import (
	"encoding/json"
	"testing"
)

func TestResponsesUsage(t *testing.T) {
	for _, total := range []string{"", `,"total_tokens":18`} {
		data := []byte(`{"usage":{"input_tokens":11,"output_tokens":7,"input_tokens_details":{"cached_tokens":3},"output_tokens_details":{"reasoning_tokens":2}` + total + `}}`)
		u := ExtractFromResponses(data)
		want := Usage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18, CachedTokens: 3, ReasoningTokens: 2}
		if u != want {
			t.Fatalf("got %+v, want %+v", u, want)
		}
		body, err := json.Marshal(map[string]any{"usage": u.OpenAI()})
		if err != nil {
			t.Fatal(err)
		}
		if got := ExtractFromSSELine(body); got != want {
			t.Fatalf("OpenAI round trip got %+v, want %+v", got, want)
		}
	}
}

func TestOpenAIUsageWithoutTotal(t *testing.T) {
	data := []byte(`{"usage":{"prompt_tokens":11,"completion_tokens":7}}`)
	if got := ExtractFromSSELine(data); got.TotalTokens != 18 {
		t.Fatalf("missing normalized total: %+v", got)
	}
	for _, empty := range []string{`{}`, `{"usage":null}`, `[DONE]`, `{`} {
		if got := ExtractFromSSELine([]byte(empty)); got != (Usage{}) {
			t.Fatalf("unexpected usage for %q: %+v", empty, got)
		}
	}
}
