package handler

import (
	"context"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/provider"
	"github.com/arisvia/cyrene-gateway/internal/usage"
)

// QuotaDeadlineRefreshInterval is how stale a stored credit deadline may get
// before selection is working from a lie. Credits are counted in days, not
// minutes, and every miss only costs ordering preference — so an hourly sweep
// is already generous, and far cheaper than per-request fetches.
const QuotaDeadlineRefreshInterval = time.Hour

// StartBackgroundQuotaDeadlineSync refreshes the stored per-account credit
// deadline for providers whose quota API we can read. Without it the deadline
// only moves when someone opens the quota panel, which would silently disable
// expiry-aware selection for unattended deployments.
func (s *Server) StartBackgroundQuotaDeadlineSync(ctx context.Context, interval time.Duration) {
	go func() {
		// Let startup traffic settle first; this is housekeeping, not hot path.
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
		s.syncAllQuotaDeadlines(ctx)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.syncAllQuotaDeadlines(ctx)
			}
		}
	}()
}

// syncAllQuotaDeadlines walks every connection that exposes a quota API and
// stores its derived deadline. Providers without a quota endpoint are skipped
// entirely rather than probed and failed.
func (s *Server) syncAllQuotaDeadlines(ctx context.Context) {
	conns, err := s.DB.ListConnections()
	if err != nil {
		return
	}
	client := s.getHTTPClient(20 * time.Second)
	for i := range conns {
		if ctx.Err() != nil {
			return
		}
		conn := &conns[i]
		if !conn.IsActive || !usage.QuotaSupported(conn.Provider) {
			continue
		}
		if conn.Data.AccessToken == "" && conn.Data.APIKey == "" {
			continue
		}
		// Same precondition the panel path uses: a stale access token makes the
		// quota call fail, which would silently freeze the deadline at its old
		// value instead of reporting a fresh one.
		s.tryRefreshToken(conn)

		orgName := ""
		projectID := ""
		if conn.Data.ProviderSpecificData != nil {
			projectID, _ = conn.Data.ProviderSpecificData["projectId"].(string)
			orgName, _ = conn.Data.ProviderSpecificData["organizationName"].(string)
		}
		// Antigravity scopes its quota call to a project; without one the fetch
		// returns no buckets, which under the rule above leaves the deadline
		// untouched forever.
		if conn.Provider == "antigravity" && projectID == "" {
			projectID, _ = s.EnsureAntigravityProject(ctx, conn, client)
		}
		res := usage.FetchQuota(ctx, client, usage.QuotaCredentials{
			Provider:     conn.Provider,
			APIKey:       conn.Data.APIKey,
			AccessToken:  conn.Data.AccessToken,
			ProjectID:    projectID,
			Organization: orgName,
			BaseURL:      conn.Data.BaseURL,
		})
		s.persistQuotaDeadline(conn, res)
	}
}

// quotaBucketsOf adapts a fetched quota result to the selection layer's bucket
// shape. A zero Remaining with a positive Total means the provider reported only
// usage, not remainder, so the difference is used instead.
func quotaBucketsOf(res usage.QuotaResult) []provider.QuotaBucket {
	buckets := make([]provider.QuotaBucket, 0, len(res.Quotas))
	for _, q := range res.Quotas {
		remaining := q.Remaining
		if remaining == 0 && q.Total > 0 {
			remaining = q.Total - q.Used
		}
		if q.Unlimited {
			remaining = 1
		}
		buckets = append(buckets, provider.QuotaBucket{
			Remaining:           remaining,
			RemainingPercentage: q.RemainingPercentage,
			ResetAt:             q.ResetAt,
			Unlimited:           q.Unlimited,
		})
	}
	return buckets
}
