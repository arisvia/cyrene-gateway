package provider

// Unified CLI & IDE client fingerprint resolution.
//
// Upstream coding endpoints (Anthropic OAuth, Grok Build, Kimi Code, CodeBuddy,
// Qoder CLI, OpenCode) gate newly released models or reject stale clients by
// User-Agent and version headers. Instead of duplicating background refreshers
// or letting hardcoded constants rot, every npm-distributed CLI shares one
// npmReleaseTracker:
//   - Reading the version is lock-free and never blocks on the network.
//   - The bundled *Floor guarantees the advertised release never drops below a
//     verified baseline, even if the registry is unreachable or stale.
//   - Operator env pins (CYRENE_*_VERSION) take immediate precedence.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	npmRegistryBase      = "https://registry.npmjs.org/"
	cliVersionTimeout    = 3 * time.Second
	cliVersionRefreshTTL = 6 * time.Hour
	cliVersionRetryDelay = 10 * time.Minute
)

// Verified baseline releases (2026-11).
const (
	ClaudeCodeVersionFloor = "2.1.331"
	GrokCLIVersionFloor    = "1.0.77"
	KimiCLIVersionFloor    = "2.6.2"
	CodebuddyVersionFloor  = "2.186.0"
	QoderCLIVersionFloor   = "1.1.94"
	OpenCodeVersionFloor   = "1.19.6"

	CopilotChatVersionDefault   = "0.38.0"
	CopilotVSCodeVersionDefault = "1.110.0"
	CopilotAPIVersionDefault    = "2025-04-01"
)

const (
	claudeCLIVersionPinEnv   = "CYRENE_CLAUDE_CLI_VERSION"
	grokCLIVersionPinEnv     = "CYRENE_GROK_CLI_VERSION"
	kimiCLIVersionPinEnv     = "CYRENE_KIMI_CLI_VERSION"
	codebuddyVersionPinEnv   = "CYRENE_CODEBUDDY_VERSION"
	qoderCLIVersionPinEnv    = "CYRENE_QODER_CLI_VERSION"
	opencodeVersionPinEnv    = "CYRENE_OPENCODE_VERSION"
	copilotChatVersionPinEnv = "CYRENE_COPILOT_CHAT_VERSION"
	copilotVSCodePinEnv      = "CYRENE_COPILOT_VSCODE_VERSION"
	copilotAPIVersionPinEnv  = "CYRENE_COPILOT_API_VERSION"
)

type npmReleaseTracker struct {
	name   string
	pkg    string
	floor  string
	envPin string
	live   atomic.Pointer[string]
	boot   sync.Once
}

var (
	claudeCLITracker = npmReleaseTracker{
		name: "claude", pkg: "@anthropic-ai/claude-code", floor: ClaudeCodeVersionFloor, envPin: claudeCLIVersionPinEnv,
	}
	grokCLITracker = npmReleaseTracker{
		name: "grok-cli", pkg: "@xai-official/grok", floor: GrokCLIVersionFloor, envPin: grokCLIVersionPinEnv,
	}
	kimiCLITracker = npmReleaseTracker{
		name: "kimi", pkg: "@moonshot-ai/kimi-code", floor: KimiCLIVersionFloor, envPin: kimiCLIVersionPinEnv,
	}
	codebuddyCLITracker = npmReleaseTracker{
		name: "codebuddy", pkg: "@tencent-ai/codebuddy-code", floor: CodebuddyVersionFloor, envPin: codebuddyVersionPinEnv,
	}
	qoderCLITracker = npmReleaseTracker{
		name: "qoder", pkg: "@qoder-ai/qodercli", floor: QoderCLIVersionFloor, envPin: qoderCLIVersionPinEnv,
	}
	opencodeCLITracker = npmReleaseTracker{
		name: "opencode", pkg: "opencode-ai", floor: OpenCodeVersionFloor, envPin: opencodeVersionPinEnv,
	}
)

// Version returns the pinned env override, else the latest valid npm release at
// or above floor, else the bundled floor. Calling it arms one background loop.
func (t *npmReleaseTracker) Version() string {
	if pin := strings.TrimSpace(os.Getenv(t.envPin)); pin != "" {
		return pin
	}
	t.boot.Do(func() { go t.refreshLoop() })
	if v := t.live.Load(); v != nil && *v != "" {
		return *v
	}
	return t.floor
}

func (t *npmReleaseTracker) refreshLoop() {
	for {
		interval := cliVersionRefreshTTL
		if !t.apply(fetchNpmLatestVersion(SafeHTTPClient(cliVersionTimeout, false), npmRegistryBase+t.pkg+"/latest")) {
			interval = cliVersionRetryDelay
		}
		timer := time.NewTimer(interval)
		<-timer.C
		timer.Stop()
	}
}

func (t *npmReleaseTracker) apply(v string) bool {
	if !isNumericRelease(v) {
		return false
	}
	if versionLess(v, t.floor) {
		// Lagging registries or cache skew must never lower the advertised floor.
		return true
	}
	if cur := t.live.Load(); cur != nil && !versionLess(*cur, v) {
		return true
	}
	live := v
	t.live.Store(&live)
	slog.Info("CLI fingerprint updated", slog.String("provider", t.name), slog.String("version", v))
	return true
}

func fetchNpmLatestVersion(client *http.Client, url string) string {
	ctx, cancel := context.WithTimeout(context.Background(), cliVersionTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var doc struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&doc); err != nil {
		return ""
	}
	return strings.TrimSpace(doc.Version)
}

func isNumericRelease(v string) bool {
	if v == "" {
		return false
	}
	for _, seg := range strings.Split(v, ".") {
		if _, err := strconv.Atoi(seg); err != nil {
			return false
		}
	}
	return true
}

func versionLess(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		x, y := 0, 0
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			return x < y
		}
	}
	return false
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// ClaudeCodeVersion reports the active Claude Code CLI version.
func ClaudeCodeVersion() string { return claudeCLITracker.Version() }

// ClaudeCodeUserAgent renders the claude-cli User-Agent for OAuth chat requests.
func ClaudeCodeUserAgent() string {
	return "claude-cli/" + ClaudeCodeVersion() + " (external, sdk-cli)"
}

// GrokCLIVersion reports the active Grok Build CLI version.
func GrokCLIVersion() string { return grokCLITracker.Version() }

// GrokCLIUserAgent renders the grok-shell User-Agent.
func GrokCLIUserAgent() string {
	return GrokCLIClientIdentifier + "/" + GrokCLIVersion() + " (linux; x86_64)"
}

// KimiCLIVersion reports the active Kimi Code CLI version.
func KimiCLIVersion() string { return kimiCLITracker.Version() }

// KimiCLIUserAgent renders the KimiCLI User-Agent.
func KimiCLIUserAgent() string { return kimiCLIUAPrefix + KimiCLIVersion() }

// CodebuddyVersion reports the active CodeBuddy CLI/IDE version.
func CodebuddyVersion() string { return codebuddyCLITracker.Version() }

// CodebuddyIDEUA renders the CodeBuddy IDE User-Agent (used by Intl chat/auth).
func CodebuddyIDEUA() string {
	v := CodebuddyVersion()
	return "IDE/" + v + " CodeBuddy/" + v
}

// CodebuddyCLIUA renders the CodeBuddy CLI User-Agent (used by CN and model catalog).
func CodebuddyCLIUA() string {
	v := CodebuddyVersion()
	return "CLI/" + v + " CodeBuddy/" + v
}

// CodebuddyUserAgent returns the product-matched User-Agent for CodeBuddy Intl vs CN.
func CodebuddyUserAgent(providerID string) string {
	if providerID == "codebuddy" {
		return CodebuddyIDEUA()
	}
	return CodebuddyCLIUA()
}

// QoderCLIVersion reports the active qodercli version.
func QoderCLIVersion() string { return qoderCLITracker.Version() }

// QoderCLIUserAgent renders the qodercli User-Agent.
func QoderCLIUserAgent() string { return "qodercli/" + QoderCLIVersion() }

// OpenCodeVersion reports the active OpenCode CLI version.
func OpenCodeVersion() string { return opencodeCLITracker.Version() }

// OpenCodeUserAgent renders the opencode User-Agent.
func OpenCodeUserAgent() string { return "opencode/" + OpenCodeVersion() }

// CopilotChatVersion returns the GitHub Copilot Chat plugin version.
func CopilotChatVersion() string {
	return envOrDefault(copilotChatVersionPinEnv, CopilotChatVersionDefault)
}

// CopilotVSCodeVersion returns the VS Code host version advertised to Copilot.
func CopilotVSCodeVersion() string {
	return envOrDefault(copilotVSCodePinEnv, CopilotVSCodeVersionDefault)
}

// CopilotAPIVersion returns the GitHub API version header for Copilot requests.
func CopilotAPIVersion() string {
	return envOrDefault(copilotAPIVersionPinEnv, CopilotAPIVersionDefault)
}

// CopilotUserAgent renders the GitHubCopilotChat User-Agent.
func CopilotUserAgent() string { return "GitHubCopilotChat/" + CopilotChatVersion() }

// CopilotEditorVersion renders the Editor-Version header for Copilot.
func CopilotEditorVersion() string { return "vscode/" + CopilotVSCodeVersion() }

// CopilotEditorPluginVersion renders the Editor-Plugin-Version header for Copilot.
func CopilotEditorPluginVersion() string { return "copilot-chat/" + CopilotChatVersion() }
