package handler

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/config"
	"github.com/arisvia/cyrene-gateway/webui"
)

// DashboardHandler serves the panel with a three-tier fallback:
// 1. Local directory (-dashboard flag) — file server for dev (vite dist or dev output)
// 2. Downloaded cache from -panel-url (cached in data dir, legacy single-HTML support)
// 3. Embedded webui/dist (Vue 3 + Vite build output)
type DashboardHandler struct {
	cfg      *config.Config
	embedded fs.FS
}

func NewDashboardHandler(cfg *config.Config) *DashboardHandler {
	sub, err := fs.Sub(webui.FS, "dist")
	if err != nil {
		slog.Error("Failed to open embedded webui", "error", err)
		sub = nil
	}
	return &DashboardHandler{cfg: cfg, embedded: sub}
}

func (d *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	// Tier 1: Local dashboard directory (dev mode)
	if d.cfg.Dashboard != "" {
		full := filepath.Join(d.cfg.Dashboard, path)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			http.ServeFile(w, r, full)
			return
		}
		// SPA fallback for local dir
		indexPath := filepath.Join(d.cfg.Dashboard, "index.html")
		if data, err := os.ReadFile(indexPath); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
			return
		}
	}

	// Tier 2: Downloaded cache (legacy single-HTML panel-url)
	if path == "" || path == "index.html" {
		if cached := d.readCache(); cached != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Write(cached)
			return
		}
	}

	// Tier 3: Embedded SPA
	if d.embedded == nil {
		http.Error(w, "dashboard unavailable", http.StatusInternalServerError)
		return
	}

	// Try exact file match (static assets)
	if path != "" {
		if f, err := d.embedded.Open(path); err == nil {
			defer f.Close()
			if stat, err := f.Stat(); err == nil && !stat.IsDir() {
				// Hashed assets are immutable
				if strings.HasPrefix(path, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				http.ServeFileFS(w, r, d.embedded, path)
				return
			}
		}
	}

	// SPA fallback: serve index.html for all unmatched routes
	data, err := fs.ReadFile(d.embedded, "index.html")
	if err != nil {
		http.Error(w, "dashboard unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
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
