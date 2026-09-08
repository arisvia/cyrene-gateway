package model

import (
	"testing"
	"time"
)

func TestAPIKey_IsModelAllowed(t *testing.T) {
	// Nil key allows all
	var nilKey *APIKey
	if !nilKey.IsModelAllowed("openai/gpt-4o") {
		t.Fatal("nil key should allow all models")
	}

	// Empty AllowedModels allows all
	emptyKey := &APIKey{}
	if !emptyKey.IsModelAllowed("openai/gpt-4o") {
		t.Fatal("empty AllowedModels should allow all models")
	}

	// Wildcard "*" allows all
	allKey := &APIKey{AllowedModels: []string{"*"}}
	if !allKey.IsModelAllowed("deepseek/deepseek-chat") {
		t.Fatal("wildcard * should allow any model")
	}

	// Specific models and prefixes
	key := &APIKey{
		AllowedModels: []string{
			"deepseek/*",
			"openai/gpt-4o",
			"anthropic/claude-3-5*",
		},
	}

	tests := []struct {
		model string
		want  bool
	}{
		{"deepseek/deepseek-chat", true},
		{"deepseek/deepseek-reasoner", true},
		{"openai/gpt-4o", true},
		{"OPENAI/GPT-4O", true}, // Case insensitive
		{"openai/gpt-4o-mini", false},
		{"anthropic/claude-3-5-sonnet", true},
		{"anthropic/claude-3-opus", false},
		{"gemini/gemini-2.5-flash", false},
	}

	for _, tt := range tests {
		if got := key.IsModelAllowed(tt.model); got != tt.want {
			t.Errorf("IsModelAllowed(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestAPIKey_IsExpired(t *testing.T) {
	// Nil or empty ExpiresAt never expires
	var nilKey *APIKey
	if nilKey.IsExpired() {
		t.Fatal("nil key should not be expired")
	}
	emptyKey := &APIKey{}
	if emptyKey.IsExpired() {
		t.Fatal("empty ExpiresAt should not be expired")
	}

	// Future time -> not expired
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	futureKey := &APIKey{ExpiresAt: future}
	if futureKey.IsExpired() {
		t.Fatal("future key should not be expired")
	}

	// Past time -> expired
	past := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	pastKey := &APIKey{ExpiresAt: past}
	if !pastKey.IsExpired() {
		t.Fatal("past key should be expired")
	}

	// Invalid format -> not considered expired
	invalidKey := &APIKey{ExpiresAt: "invalid-time"}
	if invalidKey.IsExpired() {
		t.Fatal("invalid time format should not be expired")
	}
}
