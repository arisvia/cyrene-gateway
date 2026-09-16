package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/system"
	"github.com/arisvia/cyrene-gateway/internal/updater"
)

// handleSystemStats handles GET /api/system/stats
func (s *Server) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	dbPath := ""
	if s.Config != nil {
		dbPath = s.Config.DBPath
	}

	stats, err := system.CollectStats(Version(), s.DB, dbPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// MaintenanceRequest represents action for /api/system/maintenance
type MaintenanceRequest struct {
	Action        string `json:"action"`                  // "checkpoint", "vacuum", "prune_logs"
	RetentionDays int    `json:"retentionDays,omitempty"` // for prune_logs
}

// handleSystemMaintenance handles POST /api/system/maintenance
func (s *Server) handleSystemMaintenance(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database unavailable"})
		return
	}

	var req MaintenanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json payload"})
		return
	}

	switch req.Action {
	case "checkpoint":
		if err := s.DB.CheckpointWAL(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"action":  "checkpoint",
			"message": "WAL checkpoint truncate completed successfully",
		})

	case "vacuum":
		if err := s.DB.Vacuum(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"action":  "vacuum",
			"message": "Database vacuum completed successfully",
		})

	case "prune_logs":
		days := req.RetentionDays
		if days <= 0 {
			days = 30 // default 30 days
		}
		histCount, detailCount, err := s.DB.PruneLogs(days)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"success":       true,
			"action":        "prune_logs",
			"retentionDays": days,
			"prunedHistory": histCount,
			"prunedDetails": detailCount,
			"message":       "Old logs pruned successfully",
		})

	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "unknown action: must be 'checkpoint', 'vacuum', or 'prune_logs'",
		})
	}
}

// handleSystemUpdateCheck handles GET /api/system/update/check
func (s *Server) handleSystemUpdateCheck(w http.ResponseWriter, r *http.Request) {
	client := s.getHTTPClient(15 * time.Second)
	result, err := updater.CheckForUpdate(Version(), client, "")
	if err != nil {
		slog.Error("Failed to check for updates", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// SystemUpdateRequest represents payload for POST /api/system/update
type SystemUpdateRequest struct {
	AssetURL    string `json:"assetUrl,omitempty"`
	ChecksumURL string `json:"checksumUrl,omitempty"`
	AssetName   string `json:"assetName,omitempty"`
}

// handleSystemUpdate handles POST /api/system/update
func (s *Server) handleSystemUpdate(w http.ResponseWriter, r *http.Request) {
	if system.IsRunningInDocker() {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "in-place binary update is disabled in containerized environments; please update the Docker image instead",
		})
		return
	}

	client := s.getHTTPClient(120 * time.Second)

	// Always resolve asset from pinned official GitHub Releases repository to prevent injection
	check, err := updater.CheckForUpdate(Version(), client, updater.DefaultGitHubRepo)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "check update: " + err.Error()})
		return
	}
	if check.AssetURL == "" || check.ChecksumURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "no matching release asset found for platform " + updater.ExpectedAssetName(),
		})
		return
	}

	targetExe, err := os.Executable()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "locate executable: " + err.Error()})
		return
	}
	targetDir := filepath.Dir(targetExe)

	// Download & verify SHA-256 against release checksums.txt
	tmpPath, err := updater.DownloadAndVerify(check.AssetURL, check.ChecksumURL, check.AssetName, targetDir, client)
	if err != nil {
		slog.Error("Failed to download or verify update", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "download verify: " + err.Error()})
		return
	}

	// Apply atomic replace (.old backup created on disk)
	if err := updater.ApplyUpdate(tmpPath, targetExe); err != nil {
		slog.Error("Failed to apply update", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "apply update: " + err.Error()})
		return
	}

	slog.Info("Update applied successfully, restart required", "asset", check.AssetName, "version", check.LatestVersion)
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"version": check.LatestVersion,
		"message": "Binary updated successfully. Service restart is required to take effect.",
	})
}

// handleSystemRestart handles POST /api/system/restart
func (s *Server) handleSystemRestart(w http.ResponseWriter, r *http.Request) {
	if system.IsRunningInDocker() {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "process restart is disabled in containerized environments; please use container restart policies instead",
		})
		return
	}

	// 1. Respond 200 OK immediately and flush buffer so client receives response
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Gateway is restarting now. Reconnecting shortly...",
	})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// 2. Perform port handover & successor spawn in background
	go func() {
		time.Sleep(100 * time.Millisecond)

		// Flush SQLite WAL
		if s.DB != nil {
			_ = s.DB.CheckpointWAL()
		}

		if s.onRestart != nil {
			s.onRestart()
			return
		}

		// Close listener first to release the TCP port
		if s.ShutdownFunc != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = s.ShutdownFunc(ctx)
			cancel()
		}

		// Spawn new server process with CYRENE_RESTART=1 retry-bind
		if err := updater.RestartServer(); err != nil {
			slog.Error("Failed to spawn new server process", "error", err)
			return
		}

		slog.Info("Gracefully terminating parent gateway for restart handover")
		os.Exit(0)
	}()
}

// handleSystemRollback handles POST /api/system/rollback
func (s *Server) handleSystemRollback(w http.ResponseWriter, r *http.Request) {
	if err := updater.Rollback(""); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "rollback: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Rolled back to previous binary. Please restart the service.",
	})
}
