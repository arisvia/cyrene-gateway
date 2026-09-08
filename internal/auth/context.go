package auth

import (
	"context"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

type apiKeyCtxKeyType struct{}

var apiKeyCtxKey = apiKeyCtxKeyType{}

// WithAPIKey injects the authenticated API key into context.
func WithAPIKey(ctx context.Context, key *model.APIKey) context.Context {
	if key == nil {
		return ctx
	}
	return context.WithValue(ctx, apiKeyCtxKey, key)
}

// APIKeyFromContext retrieves the API key from context, or nil if none.
func APIKeyFromContext(ctx context.Context) *model.APIKey {
	if ctx == nil {
		return nil
	}
	v, _ := ctx.Value(apiKeyCtxKey).(*model.APIKey)
	return v
}
