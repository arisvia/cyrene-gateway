// quota.go fetches real quota/usage data from provider APIs. Ported from
// 9router open-sse/services/usage/ (Phase 31). Each fetcher returns a
// QuotaResult whose Quotas map is rendered by the panel QuotaTable; Message is
// surfaced as a friendly fallback when a provider exposes no machine-readable
// quota.

package usage

import (
	"context"
	"net/http"
	"time"
)

// Quota is a single quota bucket (e.g. "Balance (CNY)", "M-series (5h)").
// Field semantics mirror 9router usage handlers so the panel renders identically.
type Quota struct {
	Used                float64 `json:"used"`
	Total               float64 `json:"total"`
	Remaining           float64 `json:"remaining,omitempty"`
	RemainingPercentage float64 `json:"remainingPercentage"`
	ResetAt             string  `json:"resetAt,omitempty"`
	Unlimited           bool    `json:"unlimited,omitempty"`
	Unit                string  `json:"unit,omitempty"`
	DisplayName         string  `json:"displayName,omitempty"`
}

// QuotaResult is the usage payload for one connection.
type QuotaResult struct {
	Plan    string           `json:"plan,omitempty"`
	Message string           `json:"message,omitempty"`
	Quotas  map[string]Quota `json:"quotas,omitempty"`
}

// QuotaCredentials is the subset of connection data a quota fetcher needs.
type QuotaCredentials struct {
	Provider    string
	APIKey      string
	AccessToken string
	// BaseURL optionally overrides the provider's upstream host. Used by tests
	// to point fetchers at a mock server; empty means the real endpoint.
	BaseURL string
}

// QuotaFetcher retrieves usage for a provider connection.
type QuotaFetcher func(ctx context.Context, client *http.Client, c QuotaCredentials) QuotaResult

// quotaHTTPClient returns a client with a bounded timeout for upstream calls.
func quotaHTTPClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// quotaFetchers maps provider id → fetcher. Only apikey-path providers with a
// real upstream quota endpoint are wired here (Phase 31); OAuth-only quota
// (gemini CLI, claude, codex, kimi) lands with the OAuth framework (Phase 33-34).
var quotaFetchers = map[string]QuotaFetcher{
	"deepseek":   fetchDeepseek,
	"glm":        fetchGLM,
	"glm-cn":     fetchGLM,
	"minimax":    fetchMiniMax,
	"minimax-cn": fetchMiniMax,
	"qoder":      fetchQoder,
}

// QuotaSupported reports whether a provider has a real usage fetcher.
func QuotaSupported(provider string) bool {
	_, ok := quotaFetchers[provider]
	return ok
}

// FetchQuota dispatches to the provider's fetcher, or returns an informational
// message when none is registered (matches 9router getUsageForProvider).
func FetchQuota(ctx context.Context, client *http.Client, c QuotaCredentials) QuotaResult {
	f, ok := quotaFetchers[c.Provider]
	if !ok {
		return QuotaResult{Message: "Usage API not implemented for " + c.Provider}
	}
	return f(ctx, quotaHTTPClient(client), c)
}
