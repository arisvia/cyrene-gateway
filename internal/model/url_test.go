package model

import "testing"

func TestDeriveModelsURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://api.openai.com/v1/chat/completions", "https://api.openai.com/v1/models"},
		{"https://api.deepseek.com/chat/completions", "https://api.deepseek.com/models"},
		{"https://api.anthropic.com/v1/messages", "https://api.anthropic.com/v1/models"},
		{"https://api.groq.com/openai/v1/chat/completions", "https://api.groq.com/openai/v1/models"},
		{"https://api.cerebras.ai/v1/chat/completions", "https://api.cerebras.ai/v1/models"},
		{"https://api.x.ai/v1/chat/completions", "https://api.x.ai/v1/models"},
		{"https://chatgpt.com/backend-api/codex/responses", "https://chatgpt.com/backend-api/codex/models"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1/models"},
		{"https://api.openai.com/v1/", "https://api.openai.com/v1/models"},
		{"", ""},
	}

	for _, tt := range tests {
		got := DeriveModelsURL(tt.input)
		if got != tt.want {
			t.Errorf("DeriveModelsURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
