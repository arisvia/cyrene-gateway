package handler

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestBroadcastLogHandler_Sanitization(t *testing.T) {
	base := slog.NewTextHandler(io.Discard, nil)
	h := NewBroadcastLogHandler(base)

	GlobalLogBuffer = NewLogRingBuffer(100)

	ctx := context.Background()
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "test log message", 0)
	r.AddAttrs(
		slog.String("access_token", "secret-token-123"),
		slog.String("api_key", "sk-proj-super-secret"),
		slog.Int("prompt_tokens", 150),
		slog.Int("completion_tokens", 45),
		slog.String("safe_attr", "normal_value"),
	)

	if err := h.Handle(ctx, r); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	records := GlobalLogBuffer.GetAll()
	if len(records) == 0 {
		t.Fatal("expected record in buffer")
	}

	attrs := records[len(records)-1].Attrs
	if attrs["access_token"] != "[REDACTED]" {
		t.Errorf("expected access_token to be [REDACTED], got: %v", attrs["access_token"])
	}
	if attrs["api_key"] != "[REDACTED]" {
		t.Errorf("expected api_key to be [REDACTED], got: %v", attrs["api_key"])
	}
	if attrs["prompt_tokens"] != int64(150) {
		t.Errorf("prompt_tokens must NOT be redacted, got: %v (%T)", attrs["prompt_tokens"], attrs["prompt_tokens"])
	}
	if attrs["completion_tokens"] != int64(45) {
		t.Errorf("completion_tokens must NOT be redacted, got: %v (%T)", attrs["completion_tokens"], attrs["completion_tokens"])
	}
	if attrs["safe_attr"] != "normal_value" {
		t.Errorf("expected safe_attr to stay unchanged, got: %v", attrs["safe_attr"])
	}
}
