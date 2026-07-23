package provider

import (
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestClampMaxTokens_NVIDIA(t *testing.T) {
	body := map[string]any{
		"max_tokens": float64(16384),
	}
	clamped := ClampMaxTokens("nvidia", "some-model", body)
	if !clamped {
		t.Error("expected clamping for nvidia provider")
	}
	if body["max_tokens"] != float64(4096) {
		t.Errorf("expected max_tokens=4096, got %v", body["max_tokens"])
	}
}

func TestClampMaxTokens_NVIDIA_NoClampNeeded(t *testing.T) {
	body := map[string]any{
		"max_tokens": float64(2048),
	}
	clamped := ClampMaxTokens("nvidia", "some-model", body)
	if clamped {
		t.Error("should not clamp when already under limit")
	}
	if body["max_tokens"] != float64(2048) {
		t.Errorf("expected max_tokens unchanged, got %v", body["max_tokens"])
	}
}

func TestClampMaxTokens_Kimi(t *testing.T) {
	body := map[string]any{
		"max_tokens": float64(65536),
	}
	clamped := ClampMaxTokens("volcengine-ark", "kimi-k2-code", body)
	if !clamped {
		t.Error("expected clamping for kimi model on volcengine-ark")
	}
	if body["max_tokens"] != float64(32768) {
		t.Errorf("expected max_tokens=32768, got %v", body["max_tokens"])
	}
}

func TestClampMaxTokens_NoMatch(t *testing.T) {
	body := map[string]any{
		"max_tokens": float64(128000),
	}
	clamped := ClampMaxTokens("openai", "gpt-4o", body)
	if clamped {
		t.Error("should not clamp for unmatched provider")
	}
}

func TestShouldRefresh_NoRefreshToken(t *testing.T) {
	conn := &mockConn{}
	conn.data.RefreshToken = ""
	conn.data.ExpiresAt = "2020-01-01T00:00:00Z"
	if ShouldRefresh(conn.toModel()) {
		t.Error("should not refresh without refresh token")
	}
}

func TestShouldRefresh_Expired(t *testing.T) {
	conn := &mockConn{}
	conn.data.RefreshToken = "rt_123"
	conn.data.ExpiresAt = "2020-01-01T00:00:00Z"
	if !ShouldRefresh(conn.toModel()) {
		t.Error("should refresh when token is expired")
	}
}

func TestShouldRefresh_NotExpired(t *testing.T) {
	conn := &mockConn{}
	conn.data.RefreshToken = "rt_123"
	conn.data.ExpiresAt = "2099-01-01T00:00:00Z"
	if ShouldRefresh(conn.toModel()) {
		t.Error("should not refresh when token is far from expiry")
	}
}

type mockConn struct {
	data struct {
		RefreshToken string
		ExpiresAt    string
	}
}

func (m *mockConn) toModel() *model.ProviderConnection {
	return &model.ProviderConnection{
		Data: model.ConnectionData{
			RefreshToken: m.data.RefreshToken,
			ExpiresAt:    m.data.ExpiresAt,
		},
	}
}
