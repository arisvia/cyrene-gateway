package provider

// Claude Code CLI fingerprint resolution. Anthropic gates newly released
// Claude models on a minimum Claude Code version: a hardcoded claude-cli
// fingerprint goes stale with every model launch, and OAuth accounts then
// stop being served models their plan actually includes. The live release is
// resolved from npm's public registry in the background (the same finding
// that made 9router@43bc940ee auto-update it); the chat path only ever reads
// an in-memory value, so an unreachable registry costs no latency, and the
// bundled floor keeps the connection working until a refresh lands.

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

// claudeCLIVersionPinEnv freezes the fingerprint without a rebuild.
const claudeCLIVersionPinEnv = "CYRENE_CLAUDE_CLI_VERSION"

// ClaudeCodeVersionFloor is the bundled release this gateway is verified
// against; the advertised fingerprint never drops below it. 2.1.322 is the
// 2026-11 model-wave floor.
const ClaudeCodeVersionFloor = "2.1.322"

const (
	claudeCLIFetchTimeout = 3 * time.Second
	claudeCLIVersionTTL   = 6 * time.Hour
	claudeCLIVersionRetry = 10 * time.Minute
)

// claudeCLIVersionURL is a var so tests can point the resolver at a stub
// registry instead of npm.
var claudeCLIVersionURL = "https://registry.npmjs.org/@anthropic-ai/claude-code/latest"

var (
	claudeCLILive atomic.Pointer[string]
	claudeCLIBoot sync.Once
)

// ClaudeCodeVersion reports the Claude Code release currently advertised
// upstream: the operator pin, else the last registry value that passed the
// floor, else the bundled floor. Calling it on the first request arms the
// background refresh; it never blocks on the network itself.
func ClaudeCodeVersion() string {
	if pin := strings.TrimSpace(os.Getenv(claudeCLIVersionPinEnv)); pin != "" {
		return pin
	}
	claudeCLIBoot.Do(func() { go claudeCLIRefreshLoop() })
	if v := claudeCLILive.Load(); v != nil && *v != "" {
		return *v
	}
	return ClaudeCodeVersionFloor
}

// ClaudeCodeUserAgent renders the claude-cli fingerprint advertised on OAuth
// chat requests.
func ClaudeCodeUserAgent() string {
	return "claude-cli/" + ClaudeCodeVersion() + " (external, sdk-cli)"
}

func claudeCLIRefreshLoop() {
	for {
		interval := claudeCLIVersionTTL
		if !applyClaudeCLIVersion(fetchClaudeCLIVersion(SafeHTTPClient(claudeCLIFetchTimeout, false), claudeCLIVersionURL)) {
			interval = claudeCLIVersionRetry
		}
		timer := time.NewTimer(interval)
		<-timer.C
		timer.Stop()
	}
}

// fetchClaudeCLIVersion reads the package's latest dist-tag version from a
// registry document; "" means "no usable answer right now".
func fetchClaudeCLIVersion(client *http.Client, url string) string {
	ctx, cancel := context.WithTimeout(context.Background(), claudeCLIFetchTimeout)
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

// applyClaudeCLIVersion adopts v only when it is a numeric release at or
// above the bundled floor and newer than what is already live. It reports
// whether the registry gave a usable answer at all (success or floor veto);
// "" and malformed bodies return false so the loop retries early.
func applyClaudeCLIVersion(v string) bool {
	if !isClaudeCLIVersion(v) {
		return false
	}
	if versionLess(v, ClaudeCodeVersionFloor) {
		// A registry lagging the floor (mirror, cache skew) must not lower
		// the fingerprint back below a release we already advertise.
		return true
	}
	if cur := claudeCLILive.Load(); cur != nil && !versionLess(*cur, v) {
		return true
	}
	live := v
	claudeCLILive.Store(&live)
	slog.Info("Claude CLI fingerprint updated", slog.String("version", v))
	return true
}

// isClaudeCLIVersion accepts only dot-separated numeric releases; the
// official Claude Code version line has no prerelease suffix.
func isClaudeCLIVersion(v string) bool {
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

// versionLess compares dot-separated numeric releases segment by segment,
// reading missing segments as zero (2.1.322 < 2.2.0 < 2.10.0).
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
