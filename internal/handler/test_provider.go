package handler

import (
	"net/http"

	"log/slog"
)

// handleTestProvider tests a single provider connection.
func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conn, err := s.DB.GetConnection(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	res := s.testConnection(r, conn)

	if res.OK {
		slog.Info("Provider test passed",
			slog.String("provider", conn.Provider),
			slog.String("connection", conn.ID),
			slog.Int("status", res.Code),
		)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"status":  "active",
			"code":    res.Code,
			"latency": res.Latency,
		})
	} else {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"status":  "error",
			"code":    res.Code,
			"error":   res.Error,
			"latency": res.Latency,
		})
	}
}
