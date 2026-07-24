package handler

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/config"
	"github.com/arisvia/cyrene-gateway/templates"
)

// DashboardHandler serves the panel with a three-tier fallback:
// 1. Local directory (-dashboard flag)
// 2. Downloaded cache from -panel-url (cached in data dir)
// 3. Embedded templates/index.html
type DashboardHandler struct {
	cfg *config.Config
}

func NewDashboardHandler(cfg *config.Config) *DashboardHandler {
	return &DashboardHandler{cfg: cfg}
}

func (d *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	// Tier 1: Local dashboard directory
	if d.cfg.Dashboard != "" {
		indexPath := filepath.Join(d.cfg.Dashboard, "index.html")
		if data, err := os.ReadFile(indexPath); err == nil {
			w.Write(data)
			return
		}
	}

	// Tier 2: Downloaded cache
	if cached := d.readCache(); cached != nil {
		w.Write(cached)
		return
	}

	// Tier 3: Embedded fallback
	data, err := templates.FS.ReadFile("index.html")
	if err != nil {
		http.Error(w, "dashboard unavailable", http.StatusInternalServerError)
		return
	}
	w.Write(data)
}

// TryDownload fetches the panel from PanelURL and caches it locally.
// Called at startup; non-fatal on failure.
func (d *DashboardHandler) TryDownload() {
	if d.cfg.PanelURL == "" {
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(d.cfg.PanelURL)
	if err != nil {
		slog.Warn("Failed to download panel, using embedded fallback", "url", d.cfg.PanelURL, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("Panel download returned non-200", "url", d.cfg.PanelURL, "status", resp.StatusCode)
		return
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024)) // 5MB limit
	if err != nil {
		slog.Warn("Failed to read panel response", "error", err)
		return
	}

	if err := d.writeCache(data); err != nil {
		slog.Warn("Failed to cache panel", "error", err)
		return
	}

	slog.Info("Panel downloaded and cached", "url", d.cfg.PanelURL, "size", len(data))
}

// cachePath is version-scoped: a binary upgrade invalidates the old cache
// so users always get the panel matching their binary version.
func (d *DashboardHandler) cachePath() string {
	return filepath.Join(d.cfg.DataDir, fmt.Sprintf("panel_cache_%s.html", Version()))
}

func (d *DashboardHandler) readCache() []byte {
	data, err := os.ReadFile(d.cachePath())
	if err != nil {
		return nil
	}
	return data
}

func (d *DashboardHandler) writeCache(data []byte) error {
	if err := os.MkdirAll(d.cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	// Clean up stale caches from previous versions
	entries, _ := os.ReadDir(d.cfg.DataDir)
	for _, e := range entries {
		if name := e.Name(); name != filepath.Base(d.cachePath()) && len(name) > len("panel_cache_") && name[:len("panel_cache_")] == "panel_cache_" {
			os.Remove(filepath.Join(d.cfg.DataDir, name))
		}
	}
	return os.WriteFile(d.cachePath(), data, 0o644)
}
