package provider

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// Backoff configuration for rate limits (mirrors 9router errorConfig.js)
const (
	BackoffBase     = 2 * time.Second
	BackoffMax      = 5 * time.Minute
	BackoffMaxLevel = 15

	TransientCooldown = 30 * time.Second

	CooldownLong  = 2 * time.Minute
	CooldownShort = 5 * time.Second
)

// ErrorRule defines a classification rule for upstream errors.
type ErrorRule struct {
	Text       string        // substring match (case-insensitive) on error message
	Status     int           // HTTP status code match
	CooldownMs time.Duration // fixed cooldown
	Backoff    bool          // use exponential backoff
}

// ErrorRules are checked top-to-bottom: text rules first, then status rules.
var ErrorRules = []ErrorRule{
	// Non-fallback deterministic errors
	{Status: 400, CooldownMs: 0},
	{Status: 422, CooldownMs: 0},

	// Text-based rules
	{Text: "no credentials", CooldownMs: CooldownLong},
	{Text: "request not allowed", CooldownMs: CooldownShort},
	{Text: "improperly formed request", CooldownMs: 0}, // Non-fallback
	{Text: "rate limit", Backoff: true},
	{Text: "too many requests", Backoff: true},
	{Text: "quota exceeded", Backoff: true},
	{Text: "capacity", Backoff: true},
	{Text: "overloaded", Backoff: true},

	// Status-based rules
	{Status: 401, CooldownMs: CooldownLong},
	{Status: 402, CooldownMs: CooldownLong},
	{Status: 403, CooldownMs: CooldownLong},
	{Status: 404, CooldownMs: CooldownLong},
	{Status: 429, Backoff: true},
}

// FallbackResult is the outcome of error classification.
type FallbackResult struct {
	ShouldFallback  bool
	Cooldown        time.Duration
	NewBackoffLevel int
}

// GetQuotaCooldown calculates exponential backoff cooldown.
// Level 1: 2s, Level 2: 4s, Level 3: 8s... → max 5min
func GetQuotaCooldown(backoffLevel int) time.Duration {
	level := max(backoffLevel-1, 0)
	cooldown := BackoffBase
	for i := 0; i < level; i++ {
		cooldown *= 2
		if cooldown >= BackoffMax {
			return BackoffMax
		}
	}
	return cooldown
}

// CheckFallbackError classifies an upstream error and determines cooldown.
func CheckFallbackError(status int, errorText string, backoffLevel int) FallbackResult {
	// Explicit 400 Bad Request / 422 Unprocessable Entity should never trigger fallback across other connections/models
	if status == 400 || status == 422 {
		return FallbackResult{ShouldFallback: false}
	}

	lowerErr := strings.ToLower(errorText)

	for _, rule := range ErrorRules {
		// Text-based rule
		if rule.Text != "" && lowerErr != "" && strings.Contains(lowerErr, rule.Text) {
			if rule.CooldownMs == 0 && !rule.Backoff {
				return FallbackResult{ShouldFallback: false}
			}
			if rule.Backoff {
				newLevel := min(backoffLevel+1, BackoffMaxLevel)
				return FallbackResult{
					ShouldFallback:  true,
					Cooldown:        GetQuotaCooldown(newLevel),
					NewBackoffLevel: newLevel,
				}
			}
			return FallbackResult{ShouldFallback: true, Cooldown: rule.CooldownMs}
		}

		// Status-based rule
		if rule.Status != 0 && rule.Status == status {
			if rule.CooldownMs == 0 && !rule.Backoff {
				return FallbackResult{ShouldFallback: false}
			}
			if rule.Backoff {
				newLevel := min(backoffLevel+1, BackoffMaxLevel)
				return FallbackResult{
					ShouldFallback:  true,
					Cooldown:        GetQuotaCooldown(newLevel),
					NewBackoffLevel: newLevel,
				}
			}
			return FallbackResult{ShouldFallback: true, Cooldown: rule.CooldownMs}
		}
	}

	// Default: transient cooldown for any unmatched error
	return FallbackResult{ShouldFallback: true, Cooldown: TransientCooldown}
}

// IsAccountUnavailable checks if a connection is currently in cooldown.
func IsAccountUnavailable(rateLimitedUntil string) bool {
	if rateLimitedUntil == "" {
		return false
	}
	until, err := time.Parse(time.RFC3339, rateLimitedUntil)
	if err != nil {
		return false
	}
	return until.After(time.Now())
}

// IsModelLockActive checks if a model-specific lock is still active.
func IsModelLockActive(conn *model.ProviderConnection, modelName string) bool {
	if conn.Data.ProviderSpecificData == nil {
		return false
	}

	key := "modelLock_" + modelName
	if expiry, ok := conn.Data.ProviderSpecificData[key]; ok {
		if expiryStr, ok := expiry.(string); ok && expiryStr != "" {
			if t, err := time.Parse(time.RFC3339, expiryStr); err == nil && t.After(time.Now()) {
				return true
			}
		}
	}

	// Check account-level lock
	if expiry, ok := conn.Data.ProviderSpecificData["modelLock___all"]; ok {
		if expiryStr, ok := expiry.(string); ok && expiryStr != "" {
			if t, err := time.Parse(time.RFC3339, expiryStr); err == nil && t.After(time.Now()) {
				return true
			}
		}
	}

	return false
}

// QuotaChecker is a function that returns the current usage count for a connection
// within its quota period. If nil, quota checks are skipped.
type QuotaChecker func(connectionID string, period string) int

// SelectCredential picks the best available connection for a provider.
// It respects priority ordering, cooldown state, model locks, quota limits, and an exclude set.
func SelectCredential(conns []model.ProviderConnection, modelName string, excludeIDs map[string]bool) *model.ProviderConnection {
	return SelectCredentialWithQuota(conns, modelName, excludeIDs, nil)
}

// SelectCredentialWithQuota extends SelectCredential with quota awareness.
// For dual-auth providers (AuthModes includes both "oauth" and "apikey"),
// OAuth connections are preferred over API key connections (9router accountFallback parity).
func SelectCredentialWithQuota(conns []model.ProviderConnection, modelName string, excludeIDs map[string]bool, quotaFn QuotaChecker) *model.ProviderConnection {
	now := time.Now()

	// Sort: prefer OAuth connections over API key for dual-auth fallback.
	sorted := make([]model.ProviderConnection, len(conns))
	copy(sorted, conns)
	sort.SliceStable(sorted, func(i, j int) bool {
		iOAuth := sorted[i].AuthType == "oauth" || sorted[i].AuthType == "access_token"
		jOAuth := sorted[j].AuthType == "oauth" || sorted[j].AuthType == "access_token"
		if iOAuth != jOAuth {
			return iOAuth
		}
		return false
	})

	for i := range sorted {
		c := &sorted[i]

		// Skip excluded connections (already tried and failed)
		if excludeIDs != nil && excludeIDs[c.ID] {
			continue
		}

		// Skip inactive
		if !c.IsActive {
			continue
		}

		// Skip rate-limited (cooldown not expired)
		if c.Data.RateLimitedUntil != "" {
			until, err := time.Parse(time.RFC3339, c.Data.RateLimitedUntil)
			if err == nil && until.After(now) {
				continue
			}
		}

		// Skip model-locked connections
		if modelName != "" && IsModelLockActive(c, modelName) {
			continue
		}

		// Skip over-quota connections
		if c.Data.QuotaLimit > 0 && quotaFn != nil {
			period := c.Data.QuotaPeriod
			if period == "" {
				period = "daily"
			}
			used := quotaFn(c.ID, period)
			if used >= c.Data.QuotaLimit {
				continue
			}
		}

		return c
	}

	return nil
}

// ApplyErrorState updates a connection's data after an upstream error.
func ApplyErrorState(conn *model.ProviderConnection, status int, errorText string) {
	result := CheckFallbackError(status, errorText, conn.Data.BackoffLevel)

	if result.Cooldown > 0 {
		conn.Data.RateLimitedUntil = time.Now().Add(result.Cooldown).UTC().Format(time.RFC3339)
	}
	if result.NewBackoffLevel > 0 {
		conn.Data.BackoffLevel = result.NewBackoffLevel
	}
	conn.Data.LastError = errorText
	conn.Data.TestStatus = "error"
}

// ResetAccountState clears cooldown and backoff after a successful request.
func ResetAccountState(conn *model.ProviderConnection) {
	conn.Data.RateLimitedUntil = ""
	conn.Data.BackoffLevel = 0
	conn.Data.LastError = ""
	conn.Data.TestStatus = "active"
}

// SetModelLock sets a model-specific lock on a connection.
func SetModelLock(conn *model.ProviderConnection, modelName string, cooldown time.Duration) {
	if conn.Data.ProviderSpecificData == nil {
		conn.Data.ProviderSpecificData = make(map[string]any)
	}
	key := "modelLock_" + modelName
	conn.Data.ProviderSpecificData[key] = time.Now().Add(cooldown).UTC().Format(time.RFC3339)
}

// ClearModelLocks removes all model locks from a connection.
func ClearModelLocks(conn *model.ProviderConnection) {
	if conn.Data.ProviderSpecificData == nil {
		return
	}
	for key := range conn.Data.ProviderSpecificData {
		if strings.HasPrefix(key, "modelLock_") {
			delete(conn.Data.ProviderSpecificData, key)
		}
	}
}

// FormatRetryAfter formats a rateLimitedUntil timestamp to human-readable string.
func FormatRetryAfter(rateLimitedUntil string) string {
	if rateLimitedUntil == "" {
		return ""
	}
	until, err := time.Parse(time.RFC3339, rateLimitedUntil)
	if err != nil {
		return ""
	}
	diff := time.Until(until)
	if diff <= 0 {
		return "reset after 0s"
	}

	totalSec := int(diff.Seconds()) + 1
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	s := totalSec % 60

	var parts []string
	if h > 0 {
		parts = append(parts, formatDurationPart(h, "h"))
	}
	if m > 0 {
		parts = append(parts, formatDurationPart(m, "m"))
	}
	if s > 0 || len(parts) == 0 {
		parts = append(parts, formatDurationPart(s, "s"))
	}
	return "reset after " + strings.Join(parts, " ")
}

func formatDurationPart(v int, unit string) string {
	return strconv.Itoa(v) + unit
}
