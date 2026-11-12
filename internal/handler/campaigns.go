package handler

import (
	"net/http"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/usage"
)

// handleConnectionCampaignClaim runs one check-then-claim round of the
// upstream's own activity API for a single connection: Qoder's daily
// "Claim 100 Credits" campaigns and CodeBuddy/WorkBuddy's daily check-in.
// Both upstreams dedupe per reward window, so repeated clicks are reported
// as "already" instead of double-granting. Body ignored; the account is
// addressed by path id.
func (s *Server) handleConnectionCampaignClaim(w http.ResponseWriter, r *http.Request) {
	conn, err := s.DB.GetConnection(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}
	if !usage.CampaignSupported(conn.Provider) {
		writeJSON(w, http.StatusOK, usage.ClaimSummary{
			Supported: false,
			Outcome:   usage.ClaimOutcomeNone,
			Message:   "No campaign API for " + conn.Provider,
		})
		return
	}
	s.tryRefreshToken(conn)

	creds := usage.QuotaCredentials{
		Provider:    conn.Provider,
		APIKey:      conn.Data.APIKey,
		AccessToken: conn.Data.AccessToken,
		BaseURL:     conn.Data.BaseURL,
	}
	if conn.Data.ProviderSpecificData != nil {
		creds.MachineID, _ = conn.Data.ProviderSpecificData["machineId"].(string)
	}

	writeJSON(w, http.StatusOK, usage.ClaimCampaigns(r.Context(), s.getHTTPClient(20*time.Second), creds))
}
