package handler

import (
	"net/http"
	"strings"
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
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
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
	orgName := ""
	if conn.Provider == "antigravity" {
		projectID, _ = s.EnsureAntigravityProject(r.Context(), conn, s.getHTTPClient(15*time.Second))
	} else if conn.Data.ProviderSpecificData != nil {
		if pid, ok := conn.Data.ProviderSpecificData["projectId"].(string); ok {
			projectID = pid
		}
		if org, ok := conn.Data.ProviderSpecificData["organizationName"].(string); ok && org != "" {
			orgName = org
		} else if org, ok := conn.Data.ProviderSpecificData["organization_name"].(string); ok && org != "" {
			orgName = org
		}
	}

	// Auto-repair missing profile info (mobile, username, organization) for Qoder and CodeBuddy
	if (conn.Provider == "qoder" || conn.Provider == "qoder-cn") && conn.Data.AccessToken != "" {
		if conn.Email == "" || orgName == "" || conn.Name == "" || conn.Name == conn.Email {
			profile := provider.FetchQoderUserProfileFor(conn.Provider, conn.Data.AccessToken, s.getHTTPClient(15*time.Second))
			if profile != nil {
				changed := false
				if conn.Email == "" && profile.EffectiveEmail() != "" {
					conn.Email = profile.EffectiveEmail()
					changed = true
				}
				if (conn.Name == "" || conn.Name == conn.Email) && profile.EffectiveName() != "" {
					conn.Name = profile.EffectiveName()
					changed = true
				}
				if orgName == "" && profile.OrganizationName != "" {
					orgName = profile.OrganizationName
					if conn.Data.ProviderSpecificData == nil {
						conn.Data.ProviderSpecificData = make(map[string]any)
					}
					conn.Data.ProviderSpecificData["organizationName"] = orgName
					changed = true
				}
				if changed {
					_ = s.DB.UpdateConnection(conn)
				}
			}
		}
	} else if strings.HasPrefix(conn.Provider, "codebuddy") && conn.Data.AccessToken != "" {
		if conn.Name == "" || conn.Name == conn.Email {
			parts := strings.Split(conn.Data.AccessToken, ".")
			if len(parts) >= 2 {
				if payload, err := provider.DecodeJWTPayload(parts[1]); err == nil {
					pref, _ := payload["preferred_username"].(string)
					if pref != "" && pref != conn.Email {
						conn.Name = pref
						_ = s.DB.UpdateConnection(conn)
					}
				}
			}
		}
	}

	res := usage.FetchQuota(r.Context(), s.getHTTPClient(15*time.Second), usage.QuotaCredentials{
		Provider:     conn.Provider,
		APIKey:       conn.Data.APIKey,
		AccessToken:  conn.Data.AccessToken,
		ProjectID:    projectID,
		Organization: orgName,
		BaseURL:      conn.Data.BaseURL,
	})
	if res.AccountEmail == "" && conn.Email != "" {
		res.AccountEmail = conn.Email
	}
	if res.AccountID == "" && conn.Data.ProviderSpecificData != nil {
		if uid, ok := conn.Data.ProviderSpecificData["userId"].(string); ok && uid != "" {
			res.AccountID = uid
		}
	}
	writeJSON(w, http.StatusOK, res)
}
