package handler

import (
	"net/http"

	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// QuotaEntry represents per-connection quota status.
type QuotaEntry struct {
	ConnectionID     string `json:"connectionId"`
	Provider         string `json:"provider"`
	Name             string `json:"name,omitempty"`
	QuotaLimit       int    `json:"quotaLimit"`
	QuotaPeriod      string `json:"quotaPeriod"`
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
