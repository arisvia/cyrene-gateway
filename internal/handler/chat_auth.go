package handler

import (
	"log/slog"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// tryRefreshToken attempts to refresh OAuth credentials if needed.
// Uses dedup lock to prevent concurrent refreshes for the same token.
// Returns true if refresh was attempted (regardless of success).
func (s *Server) tryRefreshToken(conn *model.ProviderConnection) bool {
	// GitHub Copilot: multi-step exchange (GitHub OAuth token → Copilot API token).
	// The stored access_token is the GitHub OAuth token; we exchange it for a
	// short-lived Copilot token before each request when the cached one expires.
	if conn.Provider == "github" {
		return s.tryRefreshCopilotToken(conn)
	}

	if !provider.ShouldRefresh(conn) {
		return false
	}
	oldToken := conn.Data.AccessToken
	result, err := provider.DedupRefresh(conn.Provider, oldToken, func() (*provider.RefreshResult, error) {
		return provider.RefreshCredentials(conn.Provider, conn, s.getHTTPClient(30*time.Second))
	})
	if err != nil {
		slog.Warn("Token refresh failed", slog.String("provider", conn.Provider), "error", err)
		if provider.IsUnrecoverableRefreshError(err) {
			conn.Data.TestStatus = "expired"
			conn.Data.LastError = err.Error()
			if s.DB != nil {
				if errUpdate := s.DB.UpdateConnectionHealth(conn.ID, conn.Data); errUpdate != nil {
					slog.Error("Failed to persist expired connection status", "provider", conn.Provider, "error", errUpdate)
				}
			}
		}
		return true
	}
	if errUpdate := s.persistRefreshedCredentials(conn, result); errUpdate != nil {
		slog.Error("Failed to persist refreshed connection token", "provider", conn.Provider, "error", errUpdate)
	}
	return true
}

// persistRefreshedCredentials writes only refreshed fields, conditional on the
// credentials that produced the refresh. Administrator configuration stays intact.
func (s *Server) persistRefreshedCredentials(conn *model.ProviderConnection, result *provider.RefreshResult) error {
	oldAccess, oldRefresh := conn.Data.AccessToken, conn.Data.RefreshToken
	provider.ApplyRefreshResult(conn, result)
	if s.DB == nil {
		return nil
	}
	return s.DB.UpdateConnectionTokens(conn.ID, oldAccess, oldRefresh, conn.Data, result.Extra)
}

// tryRefreshCopilotToken exchanges the stored GitHub OAuth token for a fresh
// Copilot API token. The GitHub token is long-lived; the Copilot token is
// short-lived (~30 min) and cached in ProviderSpecificData.copilotToken.
func (s *Server) tryRefreshCopilotToken(conn *model.ProviderConnection) bool {
	psd := conn.Data.ProviderSpecificData
	if psd == nil {
		psd = make(map[string]any)
		conn.Data.ProviderSpecificData = psd
	}

	// Check if cached copilot token is still valid.
	if expAt, ok := psd["copilotTokenExpiresAt"].(string); ok {
		if t, err := time.Parse(time.RFC3339, expAt); err == nil && time.Now().Before(t.Add(-2*time.Minute)) {
			if token, ok := psd["copilotToken"].(string); ok && token != "" {
				conn.Data.AccessToken = token
				return false
			}
		}
	}

	githubToken := conn.Data.AccessToken
	if psd["githubAccessToken"] != nil {
		if gt, ok := psd["githubAccessToken"].(string); ok && gt != "" {
			githubToken = gt
		}
	}

	result, err := provider.DedupRefresh("copilot", githubToken, func() (*provider.RefreshResult, error) {
		return provider.ExchangeCopilotToken(githubToken, s.getHTTPClient(30*time.Second))
	})
	if err != nil {
		slog.Warn("Copilot token exchange failed", "error", err)
		return true
	}

	// Cache the copilot token; preserve the GitHub token for future exchanges.
	oldAccess, oldRefresh := conn.Data.AccessToken, conn.Data.RefreshToken
	patch := map[string]any{"githubAccessToken": githubToken, "copilotToken": result.AccessToken}
	if result.ExpiresIn > 0 {
		patch["copilotTokenExpiresAt"] = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}
	for key, value := range patch {
		psd[key] = value
	}
	conn.Data.AccessToken = result.AccessToken
	if s.DB != nil {
		if errUpdate := s.DB.UpdateConnectionTokens(conn.ID, oldAccess, oldRefresh, conn.Data, patch); errUpdate != nil {
			slog.Error("Failed to persist copilot token", "provider", conn.Provider, "error", errUpdate)
		}
	}
	return true
}
