package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const probeTTL = 10 * time.Second
const probeTimeout = 3 * time.Second

// Status represents the current tunnel state.
type Status struct {
	Installed     bool   `json:"installed"`
	DaemonRunning bool   `json:"daemonRunning"`
	LoggedIn      bool   `json:"loggedIn"`
	FunnelRunning bool   `json:"funnelRunning"`
	TunnelURL     string `json:"tunnelUrl"`
	Platform      string `json:"platform"`
	BinPath       string `json:"binPath,omitempty"`
}

// Manager handles tailscale tunnel operations.
type Manager struct {
	mu       sync.Mutex
	dataDir  string
	port     int
	socket   string
	stateDir string

	// Cached probe results
	binCache    binCacheEntry
	statusCache statusCacheEntry
}

type binCacheEntry struct {
	path      string
	fetchedAt time.Time
	probed    bool
}

type statusCacheEntry struct {
	status    Status
	fetchedAt time.Time
}

func NewManager(dataDir string, port int) *Manager {
	tsDir := filepath.Join(dataDir, "tailscale")
	return &Manager{
		dataDir:  dataDir,
		port:     port,
		socket:   filepath.Join(tsDir, "tailscaled.sock"),
		stateDir: tsDir,
	}
}

// GetStatus returns the current tunnel status (cached with TTL).
func (m *Manager) GetStatus() Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	if time.Since(m.statusCache.fetchedAt) < probeTTL && m.statusCache.fetchedAt != (time.Time{}) {
		return m.statusCache.status
	}

	s := m.probeStatus()
	m.statusCache = statusCacheEntry{status: s, fetchedAt: time.Now()}
	return s
}

// InvalidateCache forces a fresh probe on next GetStatus call.
func (m *Manager) InvalidateCache() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusCache = statusCacheEntry{}
}

func (m *Manager) probeStatus() Status {
	s := Status{Platform: runtime.GOOS}

	bin := m.findBin()
	if bin == "" {
		return s
	}
	s.Installed = true
	s.BinPath = bin

	// Check daemon + login status
	if jsonOut, err := m.runTS(bin, "status", "--json"); err == nil {
		var tsStatus struct {
			BackendState string `json:"BackendState"`
			Self         struct {
				Online  bool   `json:"Online"`
				DNSName string `json:"DNSName"`
			} `json:"Self"`
		}
		if json.Unmarshal([]byte(jsonOut), &tsStatus) == nil {
			s.DaemonRunning = tsStatus.BackendState != "" && tsStatus.BackendState != "NoState"
			s.LoggedIn = tsStatus.BackendState == "Running" && tsStatus.Self.Online
			if s.LoggedIn && tsStatus.Self.DNSName != "" {
				dnsName := strings.TrimSuffix(tsStatus.Self.DNSName, ".")
				s.TunnelURL = "https://" + dnsName
			}
		}
	}

	// Check funnel status
	if s.LoggedIn {
		if jsonOut, err := m.runTS(bin, "funnel", "status", "--json"); err == nil {
			var funnelStatus struct {
				AllowFunnel map[string]bool `json:"AllowFunnel"`
			}
			if json.Unmarshal([]byte(jsonOut), &funnelStatus) == nil {
				s.FunnelRunning = len(funnelStatus.AllowFunnel) > 0
			}
		}
	}

	return s
}

func (m *Manager) findBin() string {
	if time.Since(m.binCache.fetchedAt) < probeTTL && m.binCache.probed {
		return m.binCache.path
	}

	var path string
	// Check data dir bin first
	localBin := filepath.Join(m.dataDir, "bin", "tailscale")
	if runtime.GOOS == "windows" {
		localBin += ".exe"
	}
	if fileExists(localBin) {
		path = localBin
	}

	// Probe common system paths
	if path == "" && runtime.GOOS != "windows" {
		candidates := []string{
			"/usr/local/bin/tailscale",
			"/opt/homebrew/bin/tailscale",
			"/usr/sbin/tailscale",
			"/usr/bin/tailscale",
			"/snap/bin/tailscale",
		}
		for _, c := range candidates {
			if fileExists(c) {
				path = c
				break
			}
		}
	}

	// Fallback: which
	if path == "" {
		if out, err := exec.Command("which", "tailscale").Output(); err == nil {
			path = strings.TrimSpace(string(out))
		}
	}

	m.binCache = binCacheEntry{path: path, fetchedAt: time.Now(), probed: true}
	return path
}

func (m *Manager) runTS(bin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	fullArgs := append([]string{"--socket", m.socket}, args...)
	cmd := exec.CommandContext(ctx, bin, fullArgs...)
	out, err := cmd.Output()
	if err != nil {
		// Try system socket
		sysArgs := append([]string{"--socket", "/var/run/tailscale/tailscaled.sock"}, args...)
		cmd2 := exec.CommandContext(ctx, bin, sysArgs...)
		out2, err2 := cmd2.Output()
		if err2 != nil {
			return "", err
		}
		return string(out2), nil
	}
	return string(out), nil
}

// EnableResult is returned from Enable().
type EnableResult struct {
	TunnelURL        string `json:"tunnelUrl,omitempty"`
	AuthURL          string `json:"authUrl,omitempty"`
	EnableURL        string `json:"enableUrl,omitempty"`
	Error            string `json:"error,omitempty"`
	Success          bool   `json:"success"`
	NeedsLogin       bool   `json:"needsLogin,omitempty"`
	FunnelNotEnabled bool   `json:"funnelNotEnabled,omitempty"`
}

// Enable starts the tailscale daemon, logs in if needed, and starts funnel.
func (m *Manager) Enable() (*EnableResult, error) {
	bin := m.findBin()
	if bin == "" {
		return nil, fmt.Errorf("tailscale is not installed")
	}

	// Ensure state dir exists
	os.MkdirAll(m.stateDir, 0o755)

	// Start daemon (best-effort, userspace networking for non-root)
	m.startDaemon(bin)

	// Check login status
	status := m.probeStatus()
	if !status.LoggedIn {
		authURL, err := m.startLogin(bin)
		if err != nil {
			return nil, fmt.Errorf("login failed: %w", err)
		}
		if authURL != "" {
			return &EnableResult{NeedsLogin: true, AuthURL: authURL}, nil
		}
	}

	// Start funnel
	tunnelURL, err := m.startFunnel(bin)
	if err != nil {
		return nil, fmt.Errorf("funnel failed: %w", err)
	}

	m.InvalidateCache()
	return &EnableResult{Success: true, TunnelURL: tunnelURL}, nil
}

// Disable stops the funnel.
func (m *Manager) Disable() error {
	bin := m.findBin()
	if bin == "" {
		return nil // Nothing to disable
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []string{"--socket", m.socket, "funnel", "--bg", "reset"}
	exec.CommandContext(ctx, bin, args...).Run()

	m.InvalidateCache()
	return nil
}

func (m *Manager) startDaemon(bin string) {
	// Check if daemon is already responsive
	if _, err := m.runTS(bin, "status", "--json"); err == nil {
		return
	}

	os.MkdirAll(m.stateDir, 0o755)

	// Find tailscaled binary
	daemonBin := "tailscaled"
	if runtime.GOOS == "darwin" {
		if fileExists("/usr/local/bin/tailscaled") {
			daemonBin = "/usr/local/bin/tailscaled"
		}
	}

	// Start in userspace-networking mode (no root required)
	cmd := exec.Command(daemonBin,
		"--socket="+m.socket,
		"--statedir="+m.stateDir,
		"--tun=userspace-networking",
	)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		slog.Warn("Failed to start tailscaled", "error", err)
		return
	}
	// Detach: don't wait for the daemon
	go cmd.Wait()

	// Wait for socket to become ready
	time.Sleep(2 * time.Second)
}

func (m *Manager) startLogin(bin string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	args := []string{"--socket", m.socket, "up", "--accept-routes"}
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	output := string(out)

	// Parse auth URL from output
	if url := parseAuthURL(output); url != "" {
		return url, nil
	}

	if err != nil {
		// Check if already logged in despite error
		if m.isLoggedIn(bin) {
			return "", nil
		}
		return "", fmt.Errorf("tailscale up failed: %s", strings.TrimSpace(output))
	}

	return "", nil
}

func (m *Manager) isLoggedIn(bin string) bool {
	out, err := m.runTS(bin, "status", "--json")
	if err != nil {
		return false
	}
	var tsStatus struct {
		BackendState string `json:"BackendState"`
		Self         struct {
			Online bool `json:"Online"`
		} `json:"Self"`
	}
	if json.Unmarshal([]byte(out), &tsStatus) != nil {
		return false
	}
	return tsStatus.BackendState == "Running" && tsStatus.Self.Online
}

func (m *Manager) startFunnel(bin string) (string, error) {
	// Reset existing funnel first
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	exec.CommandContext(ctx, bin, "--socket", m.socket, "funnel", "--bg", "reset").Run()
	cancel()

	// Start funnel
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()

	args := []string{"--socket", m.socket, "funnel", "--bg", fmt.Sprintf("%d", m.port)}
	cmd := exec.CommandContext(ctx2, bin, args...)
	out, err := cmd.CombinedOutput()
	output := string(out)

	if err != nil {
		if strings.Contains(output, "Funnel is not enabled") {
			if url := parseEnableURL(output); url != "" {
				return "", fmt.Errorf("funnel not enabled, visit: %s", url)
			}
		}
		return "", fmt.Errorf("funnel failed: %s", strings.TrimSpace(output))
	}

	// Get actual funnel URL from status
	url := m.getFunnelURL(bin)
	if url == "" {
		return "", fmt.Errorf("funnel started but could not determine URL")
	}
	return url, nil
}

func (m *Manager) getFunnelURL(bin string) string {
	out, err := m.runTS(bin, "status", "--json")
	if err != nil {
		return ""
	}
	var tsStatus struct {
		Self struct {
			DNSName string `json:"DNSName"`
		} `json:"Self"`
	}
	if json.Unmarshal([]byte(out), &tsStatus) != nil {
		return ""
	}
	dnsName := strings.TrimSuffix(tsStatus.Self.DNSName, ".")
	if dnsName == "" {
		return ""
	}
	return "https://" + dnsName
}

// Install installs tailscale on Linux. Progress is sent via the callback.
func (m *Manager) Install(ctx context.Context, onProgress func(string)) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("automatic install is only supported on Linux; please install Tailscale manually")
	}

	onProgress("Downloading install script...")

	// Download the install script
	dlCtx, dlCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dlCancel()

	cmd := exec.CommandContext(dlCtx, "curl", "-fsSL", "https://tailscale.com/install.sh")
	script, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to download install script: %w", err)
	}

	onProgress("Running install script...")

	// Write script to temp file and execute
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("tailscale-install-%d.sh", time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, script, 0o700); err != nil {
		return fmt.Errorf("failed to write install script: %w", err)
	}
	defer os.Remove(tmpFile)

	installCtx, installCancel := context.WithTimeout(ctx, 120*time.Second)
	defer installCancel()

	installCmd := exec.CommandContext(installCtx, "sh", tmpFile)
	installCmd.Env = append(os.Environ(), "PATH=/usr/local/bin:/usr/sbin:/usr/bin:/bin:/snap/bin:"+os.Getenv("PATH"))
	if out, err := installCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("install failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	onProgress("Installation complete.")

	// Invalidate bin cache so next probe finds the new binary
	m.mu.Lock()
	m.binCache = binCacheEntry{}
	m.mu.Unlock()

	return nil
}

func parseAuthURL(output string) string {
	// Look for https://login.tailscale.com/a/...
	for line := range strings.SplitSeq(output, "\n") {
		if idx := strings.Index(line, "https://login.tailscale.com/a/"); idx >= 0 {
			url := line[idx:]
			// Trim trailing whitespace/control chars
			url = strings.TrimSpace(url)
			return url
		}
	}
	return ""
}

func parseEnableURL(output string) string {
	for line := range strings.SplitSeq(output, "\n") {
		if idx := strings.Index(line, "https://login.tailscale.com/"); idx >= 0 {
			url := strings.TrimSpace(line[idx:])
			return url
		}
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
