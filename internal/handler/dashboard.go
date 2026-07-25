package handler

import (
	"archive/zip"
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

const (
	maxPanelDownload = 20 * 1024 * 1024 // 20MB download limit
	maxPanelFiles    = 500              // max files in panel zip
	maxPanelFileSize = 5 * 1024 * 1024  // 5MB per extracted file
)

// DashboardHandler serves the panel with a four-tier fallback:
// 1. Local directory (-dashboard flag) — file server for dev (vite dist or dev output)
// 2. Downloaded panel from -panel-url:
//    a. dist.zip → extracted to version-scoped cache directory
//    b. single HTML → cached as file (legacy support)
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
		if d.serveFromDir(w, r, d.cfg.Dashboard, path) {
			return
		}
	}

	// Tier 2a: Downloaded + extracted panel directory (dist.zip)
	if dir := d.panelDir(); dir != "" {
		if d.serveFromDir(w, r, dir, path) {
			return
		}
	}

	// Tier 2b: Legacy single-HTML cache
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
	d.serveEmbedded(w, r, path)
}

// serveFromDir serves a static file from dir, falling back to dir/index.html (SPA).
// Returns false if the directory is unusable.
func (d *DashboardHandler) serveFromDir(w http.ResponseWriter, r *http.Request, dir, path string) bool {
	if path != "" {
		full := filepath.Join(dir, filepath.FromSlash(path))
		// Prevent path traversal outside the panel directory
		if !strings.HasPrefix(full, filepath.Clean(dir)+string(os.PathSeparator)) {
			return false
		}
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			if strings.HasPrefix(path, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.ServeFile(w, r, full)
			return true
		}
	}
	indexPath := filepath.Join(dir, "index.html")
	if data, err := os.ReadFile(indexPath); err == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Write(data)
		return true
	}
	return false
}

func (d *DashboardHandler) serveEmbedded(w http.ResponseWriter, r *http.Request, path string) {
	// Try exact file match (static assets)
	if path != "" {
		if f, err := d.embedded.Open(path); err == nil {
			defer f.Close()
			if stat, err := f.Stat(); err == nil && !stat.IsDir() {
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
// Supports two formats:
//   - *.zip → extracted to a version-scoped directory (third-party panel distribution)
//   - anything else → treated as single HTML file (legacy)
//
// Called at startup; non-fatal on failure.
func (d *DashboardHandler) TryDownload() {
	if d.cfg.PanelURL == "" {
		return
	}

	client := &http.Client{Timeout: 30 * time.Second}
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

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPanelDownload))
	if err != nil {
		slog.Warn("Failed to read panel response", "error", err)
		return
	}

	if d.isZipURL() || isZipData(data) {
		if err := d.extractPanel(data); err != nil {
			slog.Warn("Failed to extract panel zip", "error", err)
			return
		}
		slog.Info("Panel zip downloaded and extracted", "url", d.cfg.PanelURL, "dir", d.panelDir(), "size", len(data))
		return
	}

	// Legacy single-HTML path
	if err := d.writeCache(data); err != nil {
		slog.Warn("Failed to cache panel", "error", err)
		return
	}
	slog.Info("Panel downloaded and cached", "url", d.cfg.PanelURL, "size", len(data))
}

func (d *DashboardHandler) isZipURL() bool {
	u := strings.ToLower(d.cfg.PanelURL)
	return strings.HasSuffix(u, ".zip")
}

func isZipData(data []byte) bool {
	return len(data) > 4 && data[0] == 'P' && data[1] == 'K' && (data[2] == 3 || data[2] == 5)
}

// panelDir returns the version-scoped extraction directory, or "" if not present.
func (d *DashboardHandler) panelDir() string {
	dir := filepath.Join(d.cfg.DataDir, fmt.Sprintf("panel_dist_%s", Version()))
	if info, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !info.IsDir() {
		return dir
	}
	return ""
}

// extractPanel safely extracts a zip archive, guarding against zip-slip.
func (d *DashboardHandler) extractPanel(data []byte) error {
	tmp, err := os.CreateTemp(d.cfg.DataDir, "panel-*.zip")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	tmp.Close()

	zr, err := zip.OpenReader(tmpPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	if len(zr.File) > maxPanelFiles {
		return fmt.Errorf("zip contains too many files (%d > %d)", len(zr.File), maxPanelFiles)
	}

	dest := filepath.Join(d.cfg.DataDir, fmt.Sprintf("panel_dist_%s", Version()))
	// Remove stale extraction from same version (re-download)
	os.RemoveAll(dest)

	foundIndex := false
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}

		name := filepath.FromSlash(zf.Name)
		// Zip-slip protection: reject absolute paths and traversal
		if filepath.IsAbs(name) || strings.Contains(name, "..") {
			return fmt.Errorf("zip entry with unsafe path: %s", zf.Name)
		}

		// Strip single top-level directory (e.g. "dist/index.html" → "index.html")
		if parts := strings.SplitN(name, string(os.PathSeparator), 2); len(parts) == 2 && parts[0] == "dist" {
			name = parts[1]
		}

		target := filepath.Join(dest, name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("zip entry escapes target dir: %s", zf.Name)
		}

		if name == "index.html" {
			foundIndex = true
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}

		rc, err := zf.Open()
		if err != nil {
			return fmt.Errorf("open entry %s: %w", zf.Name, err)
		}

		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return fmt.Errorf("create %s: %w", target, err)
		}

		_, err = io.Copy(out, io.LimitReader(rc, maxPanelFileSize))
		rc.Close()
		out.Close()
		if err != nil {
			return fmt.Errorf("extract %s: %w", zf.Name, err)
		}
	}

	if !foundIndex {
		os.RemoveAll(dest)
		return fmt.Errorf("zip does not contain index.html")
	}

	// Clean up stale panel dirs from previous versions
	d.cleanStaleCaches()
	return nil
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
	d.cleanStaleCaches()
	return os.WriteFile(d.cachePath(), data, 0o644)
}

// cleanStaleCaches removes panel caches and extractions from previous versions.
func (d *DashboardHandler) cleanStaleCaches() {
	entries, _ := os.ReadDir(d.cfg.DataDir)
	currentCache := filepath.Base(d.cachePath())
	currentDist := filepath.Base(d.panelDir())
	for _, e := range entries {
		name := e.Name()
		if name == currentCache || name == currentDist {
			continue
		}
		if strings.HasPrefix(name, "panel_cache_") {
			os.Remove(filepath.Join(d.cfg.DataDir, name))
		}
		if strings.HasPrefix(name, "panel_dist_") {
			os.RemoveAll(filepath.Join(d.cfg.DataDir, name))
		}
	}
}
