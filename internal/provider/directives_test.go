package provider

import (
	"testing"
)

func TestExtractAndScrubDirectives(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantCleaned string
		wantModel   string
		wantEffort  string
	}{
		{
			name:        "no directive",
			input:       "Hello world, write a quicksort algorithm in Go.",
			wantCleaned: "Hello world, write a quicksort algorithm in Go.",
			wantModel:   "",
			wantEffort:  "",
		},
		{
			name:        "cg-effort directive",
			input:       "Explain quantum computing @cg-effort:high in simple terms.",
			wantCleaned: "Explain quantum computing  in simple terms.",
			wantModel:   "",
			wantEffort:  "high",
		},
		{
			name:        "ocr-effort-up directive",
			input:       "Solve this hard leetcode question @ocr-effort-up",
			wantCleaned: "Solve this hard leetcode question",
			wantModel:   "",
			wantEffort:  "high",
		},
		{
			name:        "ocr-effort-down directive",
			input:       "What is 2+2? @cg-effort-down",
			wantCleaned: "What is 2+2?",
			wantModel:   "",
			wantEffort:  "low",
		},
		{
			name:        "cg-model directive",
			input:       "Translate to Spanish @cg-model:openai/gpt-4o please",
			wantCleaned: "Translate to Spanish  please",
			wantModel:   "openai/gpt-4o",
			wantEffort:  "",
		},
		{
			name:        "combined model and effort directive",
			input:       "Review my code @cg-model:claude-3-7-sonnet @cg-effort:max",
			wantCleaned: "Review my code",
			wantModel:   "claude-3-7-sonnet",
			wantEffort:  "max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCleaned, gotRes := ExtractAndScrubDirectives(tt.input)
			if gotCleaned != tt.wantCleaned {
				t.Errorf("ExtractAndScrubDirectives() gotCleaned = %q, want %q", gotCleaned, tt.wantCleaned)
			}
			if gotRes.Model != tt.wantModel {
				t.Errorf("ExtractAndScrubDirectives() gotModel = %q, want %q", gotRes.Model, tt.wantModel)
			}
			if gotRes.ReasoningEffort != tt.wantEffort {
				t.Errorf("ExtractAndScrubDirectives() gotEffort = %q, want %q", gotRes.ReasoningEffort, tt.wantEffort)
			}
		})
	}
}

func TestScrubDirectivesFromRawJSON(t *testing.T) {
	raw := []byte(`{
		"model": "auto",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "How to write a binary search tree? @cg-model:openai/gpt-4o @cg-effort:high"}
		]
	}`)

	updated, res, err := ScrubDirectivesFromRawJSON(raw)
	if err != nil {
		t.Fatalf("ScrubDirectivesFromRawJSON failed: %v", err)
	}
	if !res.HasOverrides {
		t.Fatalf("expected HasOverrides=true")
	}
	if res.Model != "openai/gpt-4o" {
		t.Errorf("expected model openai/gpt-4o, got %s", res.Model)
	}
	if res.ReasoningEffort != "high" {
		t.Errorf("expected effort high, got %s", res.ReasoningEffort)
	}
	if string(updated) == string(raw) {
		t.Errorf("expected updated json to be modified")
	}
}
