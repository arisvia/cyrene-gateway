package handler

import (
	"encoding/json"
	"net/http"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// handleTestCredentials tests a raw credential (provider + apiKey/baseUrl)
// before it is saved as a connection. This powers the "Test Connection"
// button in the panel's connection wizard.
// POST /api/providers/test-credentials
// Body: { "provider": "openrouter", "apiKey": "sk-...", "baseUrl": "https://..." }
func (s *Server) handleTestCredentials(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		APIKey   string `json:"apiKey"`
		BaseURL  string `json:"baseUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Provider == "" && req.BaseURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider or baseUrl required"})
		return
	}

	if _, ok := provider.GetProvider(req.Provider); !ok && req.BaseURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown provider: " + req.Provider})
		return
	}

	// Build an ephemeral (unsaved) connection for the existing test engine.
	conn := &model.ProviderConnection{
		ID:       "",
		Provider: req.Provider,
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  req.APIKey,
			BaseURL: req.BaseURL,
		},
	}
	if req.APIKey == "" {
		conn.AuthType = "none"
	}

	res := s.testConnection(r, conn)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        res.OK,
		"code":      res.Code,
		"error":     res.Error,
		"latency":   res.Latency,
		"latencyMs": res.LatencyMS,
	})
}
