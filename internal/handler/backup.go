package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/db"
)

const maxRestorePayloadSize = 100 * 1024 * 1024 // 100 MB max to prevent memory exhaustion DoS

// handleExportBackup handles GET /api/system/backup
func (s *Server) handleExportBackup(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database unavailable"})
		return
	}

	q := r.URL.Query()
	includeSecrets := q.Get("include_secrets") == "true" || q.Get("include_secrets") == "1"
	includeUsage := q.Get("include_usage") == "true" || q.Get("include_usage") == "1"

	payload, err := s.DB.ExportData(includeSecrets, includeUsage)
	if err != nil {
		slog.Error("Failed to export database", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to export database"})
		return
	}

	// Only attach authSecret when explicit secret export is requested AND not overridden by CLI/env
	if includeSecrets && !auth.IsExplicitSecret() {
		payload.AuthSecret = auth.GetSecret()
	}

	filename := fmt.Sprintf("cyrene-backup-%s.cyrene.json", time.Now().UTC().Format("2006-01-02-150405"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		slog.Error("Failed to stream backup response", "error", err)
	}
}

// handleRestoreBackup handles POST /api/system/restore
func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database unavailable"})
		return
	}

	// Enforce 100MB bound before JSON decoding to prevent unbounded memory allocation
	r.Body = http.MaxBytesReader(w, r.Body, maxRestorePayloadSize)

	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = db.RestoreModeReplace
	} else if mode != db.RestoreModeReplace && mode != db.RestoreModeMerge {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("invalid restore mode %q: must be %q or %q", mode, db.RestoreModeReplace, db.RestoreModeMerge),
		})
		return
	}
	var payload db.ExportPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid backup JSON: %v", err)})
		return
	}

	if payload.Version <= 0 || payload.Version > db.CurrentExportVersion {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("unsupported backup schema version %d (max supported is %d)", payload.Version, db.CurrentExportVersion),
		})
		return
	}

	if err := s.DB.ImportData(&payload, mode); err != nil {
		slog.Error("Failed to restore database", "error", err, "mode", mode)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to restore database: %v", err)})
		return
	}

	// Post-restore hydration & hot-reload
	if payload.AuthSecret != "" && payload.IncludesSecrets && !auth.IsExplicitSecret() {
		dataDir := ""
		if s.Config != nil {
			dataDir = s.Config.DataDir
		}
		if err := auth.PersistSecret(dataDir, payload.AuthSecret); err != nil {
			slog.Warn("Failed to persist restored authSecret", "error", err)
		}
	}

	// Synchronize runtime ProxyManager with fresh DB proxy pools
	s.refreshProxies()

	// Invalidate response cache if enabled
	if s.Cache != nil {
		s.Cache.Clear()
	}

	slog.Info("Database restored successfully",
		slog.String("mode", mode),
		slog.Int("version", payload.Version),
		slog.Int("conns", len(payload.Data.ProviderConnections)),
		slog.Int("keys", len(payload.Data.APIKeys)),
	)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Database restored successfully",
		"version": payload.Version,
		"mode":    mode,
	})
}
