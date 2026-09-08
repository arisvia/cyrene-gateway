package auth

import (
	"context"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestContextAPIKey(t *testing.T) {
	ctx := context.Background()
	if got := APIKeyFromContext(ctx); got != nil {
		t.Fatalf("expected nil APIKey from background context, got %+v", got)
	}

	key := &model.APIKey{
		ID:   "key-test",
		Name: "Test",
	}

	ctxWithKey := WithAPIKey(ctx, key)
	got := APIKeyFromContext(ctxWithKey)
	if got == nil || got.ID != key.ID {
		t.Fatalf("expected key-test, got %+v", got)
	}

	// nil key should return original context unchanged
	if gotNil := APIKeyFromContext(WithAPIKey(ctx, nil)); gotNil != nil {
		t.Fatalf("expected nil from nil key injection, got %+v", gotNil)
	}
}
