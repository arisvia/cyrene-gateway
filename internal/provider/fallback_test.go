package provider

import (
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestGetQuotaCooldown(t *testing.T) {
	tests := []struct {
		level    int
		expected time.Duration
	}{
		{0, 2 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{10, 5 * time.Minute}, // 1024s > 5min, capped at max
		{15, 5 * time.Minute},
	}
	for _, tt := range tests {
		got := GetQuotaCooldown(tt.level)
		if got != tt.expected {
			t.Errorf("GetQuotaCooldown(%d) = %v, want %v", tt.level, got, tt.expected)
		}
	}
}

func TestCheckFallbackError(t *testing.T) {
	// Text-based: rate limit triggers backoff
	result := CheckFallbackError(429, "rate limit exceeded", 0)
	if !result.ShouldFallback {
		t.Error("expected ShouldFallback=true for rate limit")
	}
	if result.NewBackoffLevel != 1 {
		t.Errorf("expected NewBackoffLevel=1, got %d", result.NewBackoffLevel)
	}
	if result.Cooldown != 2*time.Second {
		t.Errorf("expected 2s cooldown at level 1, got %v", result.Cooldown)
	}

	// Text-based: no credentials → fixed cooldown
	result = CheckFallbackError(401, "no credentials available", 0)
	if result.Cooldown != CooldownLong {
		t.Errorf("expected CooldownLong for 'no credentials', got %v", result.Cooldown)
	}
	if result.NewBackoffLevel != 0 {
		t.Errorf("expected no backoff for 'no credentials', got level %d", result.NewBackoffLevel)
	}

	// Status-based: 403 → fixed cooldown
	result = CheckFallbackError(403, "", 0)
	if result.Cooldown != CooldownLong {
		t.Errorf("expected CooldownLong for 403, got %v", result.Cooldown)
	}

	// Unknown error → transient cooldown
	result = CheckFallbackError(500, "something weird happened", 0)
	if result.Cooldown != TransientCooldown {
		t.Errorf("expected TransientCooldown for unknown error, got %v", result.Cooldown)
	}
}

func TestSelectCredential(t *testing.T) {
	now := time.Now()
	future := now.Add(5 * time.Minute).UTC().Format(time.RFC3339)
	past := now.Add(-5 * time.Minute).UTC().Format(time.RFC3339)

	conns := []model.ProviderConnection{
		{ID: "conn-1", Provider: "openai", IsActive: true, Priority: 1,
			Data: model.ConnectionData{RateLimitedUntil: future}}, // rate-limited
		{ID: "conn-2", Provider: "openai", IsActive: true, Priority: 2,
			Data: model.ConnectionData{RateLimitedUntil: past}}, // cooldown expired
		{ID: "conn-3", Provider: "openai", IsActive: false, Priority: 3,
			Data: model.ConnectionData{}}, // inactive
		{ID: "conn-4", Provider: "openai", IsActive: true, Priority: 4,
			Data: model.ConnectionData{}}, // available
	}

	// Should pick conn-2 (first available after skipping rate-limited conn-1)
	selected := SelectCredential(conns, "gpt-4", nil)
	if selected == nil {
		t.Fatal("expected a connection to be selected")
	}
	if selected.ID != "conn-2" {
		t.Errorf("expected conn-2, got %s", selected.ID)
	}

	// Exclude conn-2, should pick conn-4
	selected = SelectCredential(conns, "gpt-4", map[string]bool{"conn-2": true})
	if selected == nil {
		t.Fatal("expected a connection to be selected")
	}
	if selected.ID != "conn-4" {
		t.Errorf("expected conn-4, got %s", selected.ID)
	}

	// Exclude all available → nil
	selected = SelectCredential(conns, "gpt-4", map[string]bool{"conn-2": true, "conn-4": true})
	if selected != nil {
		t.Errorf("expected nil when all excluded, got %s", selected.ID)
	}
}

func TestSelectCredentialWithModelLock(t *testing.T) {
	future := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)

	conns := []model.ProviderConnection{
		{ID: "conn-1", Provider: "openai", IsActive: true, Priority: 1,
			Data: model.ConnectionData{
				ProviderSpecificData: map[string]any{
					"modelLock_gpt-4": future,
				},
			}},
		{ID: "conn-2", Provider: "openai", IsActive: true, Priority: 2,
			Data: model.ConnectionData{}},
	}

	// Model lock active for gpt-4 on conn-1 → should pick conn-2
	selected := SelectCredential(conns, "gpt-4", nil)
	if selected == nil {
		t.Fatal("expected a connection")
	}
	if selected.ID != "conn-2" {
		t.Errorf("expected conn-2 (model-locked conn-1 skipped), got %s", selected.ID)
	}

	// Different model → conn-1 is fine
	selected = SelectCredential(conns, "gpt-3.5-turbo", nil)
	if selected == nil {
		t.Fatal("expected a connection")
	}
	if selected.ID != "conn-1" {
		t.Errorf("expected conn-1 for different model, got %s", selected.ID)
	}
}

func TestApplyAndResetErrorState(t *testing.T) {
	conn := &model.ProviderConnection{
		ID:       "test",
		Provider: "openai",
		IsActive: true,
		Data:     model.ConnectionData{},
	}

	// Apply error
	ApplyErrorState(conn, 429, "rate limit exceeded")
	if conn.Data.RateLimitedUntil == "" {
		t.Error("expected RateLimitedUntil to be set")
	}
	if conn.Data.BackoffLevel != 1 {
		t.Errorf("expected BackoffLevel=1, got %d", conn.Data.BackoffLevel)
	}
	if conn.Data.TestStatus != "error" {
		t.Errorf("expected status 'error', got %s", conn.Data.TestStatus)
	}

	// Reset
	ResetAccountState(conn)
	if conn.Data.RateLimitedUntil != "" {
		t.Error("expected RateLimitedUntil to be cleared")
	}
	if conn.Data.BackoffLevel != 0 {
		t.Errorf("expected BackoffLevel=0, got %d", conn.Data.BackoffLevel)
	}
	if conn.Data.TestStatus != "active" {
		t.Errorf("expected status 'active', got %s", conn.Data.TestStatus)
	}
}

func TestFormatRetryAfter(t *testing.T) {
	// Empty
	if got := FormatRetryAfter(""); got != "" {
		t.Errorf("expected empty for empty input, got %q", got)
	}

	// Past → "reset after 0s"
	past := time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339)
	if got := FormatRetryAfter(past); got != "reset after 0s" {
		t.Errorf("expected 'reset after 0s' for past time, got %q", got)
	}

	// Future ~90s → "reset after 1m 30s" (approximately)
	future := time.Now().Add(90 * time.Second).UTC().Format(time.RFC3339)
	got := FormatRetryAfter(future)
	if got == "" {
		t.Error("expected non-empty for future time")
	}
}
