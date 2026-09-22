package handler

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/config"
	"github.com/arisvia/cyrene-gateway/internal/provider"
	"github.com/arisvia/cyrene-gateway/webui"
)

const (
	maxPanelDownload = 20 * 1024 * 1024 // 20MB download limit
	maxPanelFiles    = 500              // max files in panel zip
	maxPanelFileSize = 5 * 1024 * 1024  // 5MB per extracted file
)

// DashboardHandler serves the panel with a four-tier fallback:
//  1. Local directory (-dashboard flag) — file server for dev (vite dist or dev output)
//  2. Downloaded panel from -panel-url:
//     a. dist.zip → extracted to version-scoped cache directory
//     b. single HTML → cached as file (legacy support)
//  3. Embedded webui/dist (Vue 3 + Vite build output)
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
	addDashboardVary(w.Header())
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
			if !slices.Contains(dashboardEncodings(r.Header), "identity") {
				http.Error(w, "no acceptable content encoding", http.StatusNotAcceptable)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(cached))
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
	return serveDashboardFS(w, r, os.DirFS(dir), path)
}

func (d *DashboardHandler) serveEmbedded(w http.ResponseWriter, r *http.Request, path string) {
	if !serveDashboardFS(w, r, d.embedded, path) {
		http.Error(w, "dashboard unavailable", http.StatusInternalServerError)
	}
}

func serveDashboardFS(w http.ResponseWriter, r *http.Request, fsys fs.FS, path string) bool {
	if path != "" && serveDashboardFile(w, r, fsys, path) {
		return true
	}
	// Missing hashed assets must never be masked by the SPA fallback.
	if strings.HasPrefix(path, "assets/") {
		http.NotFound(w, r)
		return true
	}
	return serveDashboardFile(w, r, fsys, "index.html")
}

func serveDashboardFile(w http.ResponseWriter, r *http.Request, fsys fs.FS, name string) bool {
	// Validate before using os.DirFS, including Windows path separators.
	if !fs.ValidPath(name) || strings.Contains(name, `\`) {
		return false
	}
	info, err := fs.Stat(fsys, name)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}

	var file http.File
	selected, encoding := false, ""
	for _, coding := range dashboardEncodings(r.Header) {
		if coding == "identity" {
			selected = true
			break
		}
		suffix := ".br"
		if coding == "gzip" {
			suffix = ".gz"
		}
		f, err := http.FS(fsys).Open(name + suffix)
		if err != nil {
			continue
		}
		stat, err := f.Stat()
		if err != nil || !stat.Mode().IsRegular() {
			f.Close()
			continue
		}
		file, info, selected, encoding = f, stat, true, coding
		break
	}
	if !selected {
		http.Error(w, "no acceptable content encoding", http.StatusNotAcceptable)
		return true
	}

	// Open index entries directly to avoid redirects and allow tier fallback.
	isIndex := filepath.Base(name) == "index.html"
	if file == nil && (isIndex || strings.HasSuffix(r.URL.Path, "/index.html")) {
		file, err = http.FS(fsys).Open(name)
		if err != nil {
			return false
		}
	}
	if file != nil {
		defer file.Close()
	}
	if encoding != "" {
		// Sniff the original, never the compressed bytes, for unknown extensions.
		contentType := mime.TypeByExtension(filepath.Ext(name))
		if contentType == "" {
			f, err := fsys.Open(name)
			if err != nil {
				return false
			}
			var buf [512]byte
			n, _ := io.ReadFull(f, buf[:])
			f.Close()
			contentType = http.DetectContentType(buf[:n])
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Encoding", encoding)
	}
	if isIndex {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	} else if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}

	// Reuse the open sidecar/entry without reopening or buffering its contents.
	if file != nil {
		http.ServeContent(w, r, name, info.ModTime(), file)
	} else {
		http.ServeFileFS(w, r, fsys, name)
	}
	return true
}

func addDashboardVary(h http.Header) {
	for _, line := range h.Values("Vary") {
		for token := range strings.SplitSeq(line, ",") {
			token = strings.TrimSpace(token)
			if token == "*" || strings.EqualFold(token, "Accept-Encoding") {
				return
			}
		}
	}
	h.Add("Vary", "Accept-Encoding")
}

// Unspecified identity is a last resort unless *;q=0 excludes it.
func dashboardEncodings(h http.Header) []string {
	weights := make(map[string]int, 4)
	for _, line := range h.Values("Accept-Encoding") {
		for item := range strings.SplitSeq(line, ",") {
			token, params, hasParams := strings.Cut(item, ";")
			token = strings.ToLower(strings.TrimSpace(token))
			switch token {
			case "br", "gzip", "identity", "*":
			default:
				continue
			}
			q := 1000
			if hasParams {
				key, value, ok := strings.Cut(params, "=")
				q = 0
				if ok && strings.EqualFold(strings.TrimSpace(key), "q") {
					q = dashboardEncodingQuality(strings.TrimSpace(value))
				}
			}
			// Conflicting duplicates must not resurrect an explicit prohibition.
			if previous, ok := weights[token]; ok {
				q = min(q, previous)
			}
			weights[token] = q
		}
	}
	var accepted []string
	for _, coding := range []string{"br", "gzip", "identity"} {
		q, explicit := weights[coding]
		if !explicit {
			wildcard, present := weights["*"]
			q = wildcard
			if coding == "identity" && (!present || wildcard != 0) {
				q = -1 // Allowed, but no explicit preference over compression.
			}
		}
		weights[coding] = q
		if q != 0 {
			accepted = append(accepted, coding)
		}
	}
	slices.SortStableFunc(accepted, func(a, b string) int { return weights[b] - weights[a] })
	return accepted
}

// Parse RFC 9110 qvalues; invalid weights disable the coding.
func dashboardEncodingQuality(value string) int {
	whole, fraction, _ := strings.Cut(value, ".")
	if (whole != "0" && whole != "1") || len(fraction) > 3 {
		return 0
	}
	q, place := 0, 100
	for _, digit := range fraction {
		if digit < '0' || digit > '9' || (whole == "1" && digit != '0') {
			return 0
		}
		q += int(digit-'0') * place
		place /= 10
	}
	if whole == "1" {
		return 1000
	}
	return q
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
	allowPrivate := d.cfg.AllowPrivateNetworks
	if _, err := provider.ValidateUpstreamURL(d.cfg.PanelURL, allowPrivate); err != nil {
		slog.Warn("Panel URL failed SSRF validation, using embedded fallback", "url", d.cfg.PanelURL, "error", err)
		return
	}

	client := provider.SafeHTTPClient(30*time.Second, allowPrivate)
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
