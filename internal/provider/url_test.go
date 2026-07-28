package provider

import (
	"encoding/base64"
	"testing"
)

func TestBuildChatURL(t *testing.T) {
	tests := []struct {
		base    string
		apiType string
		want    string
	}{
		// Full endpoint URLs are returned as-is
		{"https://api.openai.com/v1/chat/completions", "openai", "https://api.openai.com/v1/chat/completions"},
		{"https://api.anthropic.com/v1/messages", "anthropic", "https://api.anthropic.com/v1/messages"},
		{"https://opencode.ai/zen/v1/chat/completions", "openai", "https://opencode.ai/zen/v1/chat/completions"},
		{"https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation", "openai", "https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation"},
		// Base URLs get path appended
		{"https://api.openai.com/v1", "openai", "https://api.openai.com/v1/chat/completions"},
		{"https://openrouter.ai/api/v1", "openai", "https://openrouter.ai/api/v1/chat/completions"},
		{"https://api.anthropic.com", "anthropic", "https://api.anthropic.com/v1/messages"},
		// Trailing slash stripped
		{"https://api.openai.com/v1/", "openai", "https://api.openai.com/v1/chat/completions"},
		// Empty
		{"", "openai", ""},
	}

	for _, tt := range tests {
		got := BuildChatURL(tt.base, tt.apiType)
		if got != tt.want {
			t.Errorf("BuildChatURL(%q, %q) = %q, want %q", tt.base, tt.apiType, got, tt.want)
		}
	}
}

func TestBuildModelsURL(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{"https://api.openai.com/v1/chat/completions", "https://api.openai.com/v1/models"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1/models"},
		{"https://opencode.ai/zen/v1/chat/completions", "https://opencode.ai/zen/v1/models"},
		{"https://openrouter.ai/api/v1/chat/completions", "https://openrouter.ai/api/v1/models"},
	}

	for _, tt := range tests {
		got := BuildModelsURL(tt.base)
		if got != tt.want {
			t.Errorf("BuildModelsURL(%q) = %q, want %q", tt.base, got, tt.want)
		}
	}
}

func TestBuildGeminiURL(t *testing.T) {
	base := "https://generativelanguage.googleapis.com/v1beta/models"
	got := BuildGeminiURL(base, "gemini-2.0-flash", false)
	want := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"
	if got != want {
		t.Errorf("BuildGeminiURL non-stream = %q, want %q", got, want)
	}

	got = BuildGeminiURL(base, "gemini-2.0-flash", true)
	want = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:streamGenerateContent?alt=sse"
	if got != want {
		t.Errorf("BuildGeminiURL stream = %q, want %q", got, want)
	}
}

func TestQoderEncodeBody(t *testing.T) {
	// Verify the encoding is reversible structure: same length as base64 input
	input := []byte(`{"hello":"world"}`)
	encoded := QoderEncodeBody(input)

	stdB64 := base64.StdEncoding.EncodeToString(input)
	if len(encoded) != len(stdB64) {
		t.Errorf("QoderEncodeBody length = %d, want %d (same as base64)", len(encoded), len(stdB64))
	}

	// Encoded output should not contain standard base64 chars that have mappings
	// (spot check: 'A' maps to '_', 'B' maps to 'd', etc.)
	if encoded == stdB64 {
		t.Error("QoderEncodeBody returned unmodified base64 — substitution not applied")
	}
}

func TestQoderComputeSigPath(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result&Encode=1", "/api/v2/service/pro/sse/agent_chat_generation"},
		{"https://api3.qoder.sh/algo/api/v2/model/list", "/api/v2/model/list"},
		{"https://example.com/other/path", "/other/path"},
	}

	for _, tt := range tests {
		got := qoderComputeSigPath(tt.url)
		if got != tt.want {
			t.Errorf("qoderComputeSigPath(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestBuildQoderCosyHeaders(t *testing.T) {
	creds := QoderCosyCreds{
		UserID:    "test-user-123",
		AuthToken: "dt-testtoken",
		Name:      "Test",
		Email:     "test@example.com",
		MachineID: "machine-abc",
	}

	headers, err := BuildQoderCosyHeaders([]byte("test body"), QoderChatURL, creds)
	if err != nil {
		t.Fatalf("BuildQoderCosyHeaders failed: %v", err)
	}

	required := []string{
		"Authorization", "Cosy-Key", "Cosy-User", "Cosy-Date", "Cosy-Version",
		"Cosy-Machineid", "Cosy-Machinetoken", "Cosy-Machinetype", "Cosy-Machineos",
		"Cosy-Clienttype", "Cosy-Clientip", "Cosy-Bodyhash", "Cosy-Bodylength",
		"Cosy-Sigpath", "Cosy-Data-Policy", "Login-Version", "X-Request-Id",
	}
	for _, key := range required {
		if _, ok := headers[key]; !ok {
			t.Errorf("missing required header: %s", key)
		}
	}

	// Authorization format: Bearer COSY.<payloadB64>.<md5sig>
	auth := headers["Authorization"]
	if len(auth) < 13 || auth[:13] != "Bearer COSY. "[:13] {
		// Check prefix "Bearer COSY."
		if auth[:12] != "Bearer COSY." {
			t.Errorf("Authorization header has wrong format: %q", auth[:30])
		}
	}

	if headers["Cosy-User"] != "test-user-123" {
		t.Errorf("Cosy-User = %q, want test-user-123", headers["Cosy-User"])
	}
	if headers["Cosy-Machineid"] != "machine-abc" {
		t.Errorf("Cosy-Machineid = %q, want machine-abc", headers["Cosy-Machineid"])
	}
	if headers["Cosy-Sigpath"] != "/api/v2/service/pro/sse/agent_chat_generation" {
		t.Errorf("Cosy-Sigpath = %q", headers["Cosy-Sigpath"])
	}

	// Missing creds should error
	_, err = BuildQoderCosyHeaders(nil, QoderChatURL, QoderCosyCreds{AuthToken: "x"})
	if err == nil {
		t.Error("expected error for missing userId")
	}
	_, err = BuildQoderCosyHeaders(nil, QoderChatURL, QoderCosyCreds{UserID: "x"})
	if err == nil {
		t.Error("expected error for missing authToken")
	}
}

func TestUnwrapQoderSSELine(t *testing.T) {
	// Normal envelope
	data, done := UnwrapQoderSSELine(`data: {"statusCodeValue":200,"body":"{\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}"}`, "qoder/auto")
	if done {
		t.Error("unexpected done")
	}
	if data == "" {
		t.Error("expected unwrapped data")
	}

	// Inner [DONE]
	data, done = UnwrapQoderSSELine(`data: {"statusCodeValue":200,"body":"[DONE]"}`, "qoder/auto")
	if !done {
		t.Error("expected done for inner [DONE]")
	}
	_ = data

	// Outer [DONE]
	_, done = UnwrapQoderSSELine("data: [DONE]", "qoder/auto")
	if !done {
		t.Error("expected done for outer [DONE]")
	}

	// Error status
	data, done = UnwrapQoderSSELine(`data: {"statusCodeValue":429,"body":"rate limited"}`, "qoder/auto")
	if !done {
		t.Error("expected done for error status")
	}
	if data == "" {
		t.Error("expected error chunk data")
	}

	// Non-data line
	data, done = UnwrapQoderSSELine("event: message", "qoder/auto")
	if done || data != "" {
		t.Error("non-data line should be skipped")
	}
}

func TestQoderParseExpiry(t *testing.T) {
	// Numeric ms
	got := QoderParseExpiry(float64(1781594470000), nil)
	if got != 1781594470000 {
		t.Errorf("numeric expiry = %d, want 1781594470000", got)
	}

	// Numeric string
	got = QoderParseExpiry("1781594470000", nil)
	if got != 1781594470000 {
		t.Errorf("string expiry = %d, want 1781594470000", got)
	}

	// RFC3339
	got = QoderParseExpiry("2026-06-16T07:15:04Z", nil)
	if got <= 0 {
		t.Error("RFC3339 expiry should be positive")
	}

	// expires_in seconds
	now := float64(1000000000000)
	_ = now
	got = QoderParseExpiry(nil, float64(3600))
	if got <= 0 {
		t.Error("expires_in should produce future timestamp")
	}

	// Fallback: 30 days
	got = QoderParseExpiry(nil, nil)
	if got <= 0 {
		t.Error("fallback should produce future timestamp")
	}
}
