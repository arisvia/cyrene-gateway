package handler

import (
	"net/http"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/provider"
	"github.com/arisvia/cyrene-gateway/internal/usage"
)

// QuotaEntry represents per-connection quota status.
type QuotaEntry struct {
	ConnectionID     string `json:"connectionId"`
	Provider         string `json:"provider"`
	Name             string `json:"name,omitempty"`
	QuotaPeriod      string `json:"quotaPeriod"`
	QuotaLimit       int    `json:"quotaLimit"`
	UsedRequests     int    `json:"usedRequests"`
	UsedPromptTokens int    `json:"usedPromptTokens"`
	UsedCompTokens   int    `json:"usedCompletionTokens"`
	OverQuota        bool   `json:"overQuota"`
}

func (s *Server) handleQuota(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "daily"
	}

	conns, err := s.DB.ListConnections()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	var entries []QuotaEntry
	for _, c := range conns {
		if c.Data.QuotaLimit <= 0 {
			continue
		}
		connPeriod := c.Data.QuotaPeriod
		if connPeriod == "" {
			connPeriod = "daily"
		}
		// Use the connection's own period for its quota check
		used := s.DB.GetConnectionUsageCount(c.ID, connPeriod)
		prompt, comp := s.DB.GetConnectionUsageTokens(c.ID, connPeriod)

		entries = append(entries, QuotaEntry{
			ConnectionID:     c.ID,
			Provider:         c.Provider,
			Name:             c.Name,
			QuotaLimit:       c.Data.QuotaLimit,
			QuotaPeriod:      connPeriod,
			UsedRequests:     used,
			UsedPromptTokens: prompt,
			UsedCompTokens:   comp,
			OverQuota:        used >= c.Data.QuotaLimit,
		})
	}

	if entries == nil {
		entries = []QuotaEntry{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"period":  period,
		"entries": entries,
	})
}

// quotaChecker returns a provider.QuotaChecker backed by the DB.
func (s *Server) quotaChecker() provider.QuotaChecker {
	return func(connectionID string, period string) int {
		return s.DB.GetConnectionUsageCount(connectionID, period)
	}
}

// handleConnectionUsage fetches real quota/usage data from a provider's own
// API (Phase 31). Port of 9router GET /api/usage/[connectionId]. Providers
// without an upstream quota endpoint return an informational message.
func (s *Server) handleConnectionUsage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conn, err := s.DB.GetConnection(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	if !usage.QuotaSupported(conn.Provider) {
		writeJSON(w, http.StatusOK, usage.QuotaResult{
			Message: "Usage API not implemented for " + conn.Provider,
		})
		return
	}

	s.tryRefreshToken(conn)
	projectID := ""
	if conn.Data.ProviderSpecificData != nil {
		if pid, ok := conn.Data.ProviderSpecificData["projectId"].(string); ok {
			projectID = pid
		}
	}
	// Lazy Antigravity project discovery if missing
	if projectID == "" && conn.Provider == "antigravity" && conn.Data.AccessToken != "" {
		if discovered, err := DiscoverAntigravityProject(r.Context(), s.getHTTPClient(15*time.Second), conn.Data.AccessToken); err == nil && discovered != "" {
			projectID = discovered
			if conn.Data.ProviderSpecificData == nil {
				conn.Data.ProviderSpecificData = make(map[string]any)
			}
			conn.Data.ProviderSpecificData["projectId"] = projectID
			s.DB.UpdateConnection(conn)
		}
	}

	res := usage.FetchQuota(r.Context(), s.getHTTPClient(15*time.Second), usage.QuotaCredentials{
		Provider:    conn.Provider,
		APIKey:      conn.Data.APIKey,
		AccessToken: conn.Data.AccessToken,
		ProjectID:   projectID,
	})
	writeJSON(w, http.StatusOK, res)
}
