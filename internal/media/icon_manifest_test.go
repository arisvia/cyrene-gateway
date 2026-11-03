package media

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// brandAssetsDir holds the provider SVGs the panel bundles. Both copies must
// stay in step: webui/public is served at runtime, webui/src is inlined at
// build time.
const brandAssetsDir = "../../webui/src/assets/providers"

// monochromeOnly lists brands whose upstream icon set has no colour variant, so
// shipping the monochrome mark is the correct outcome rather than an oversight.
// Rendered badge providers that fall through to initials are not listed here.
var monochromeOnly = map[string]bool{
	"anthropic": true, "cline": true, "cursor": true, "deepgram": true,
	"elevenlabs": true, "github": true, "grok-cli": true, "groq": true,
	"kilocode": true, "openai": true, "opencode": true, "roo": true,
	"windsurf": true, "xai": true, "xiaomimimo": true,
}

// AGENTS.md rule 6: brand icons must use the colour variant when upstream
// publishes one. A monochrome asset outside the allowlist means someone pulled
// the wrong file from LobeHub, which is how this repository ended up with a
// split that nobody noticed.
func TestBrandIconsPreferColourVariants(t *testing.T) {
	entries, err := os.ReadDir(brandAssetsDir)
	if err != nil {
		t.Fatalf("read brand assets: %v", err)
	}
	hardColour := regexp.MustCompile(`#[\da-fA-F]{3,8}\b|rgba?\(|url\(`)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".svg") {
			continue
		}
		brand := strings.TrimSuffix(e.Name(), ".svg")
		raw, err := os.ReadFile(filepath.Join(brandAssetsDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		body := string(raw)
		isMonochrome := strings.Contains(body, "currentColor") && !hardColour.MatchString(body)
		if isMonochrome && !monochromeOnly[brand] {
			t.Errorf("%s ships the monochrome mark but upstream has a colour variant (AGENTS.md rule 6)", brand)
		}
		if !isMonochrome && monochromeOnly[brand] {
			t.Errorf("%s is expected to be monochrome-only; either the allowlist is stale or the asset changed", brand)
		}
	}
}

// The runtime copy and the build-time copy must not drift apart: the panel
// serves one path and inlines the other, so a difference means two different
// icons depending on load order.
func TestBrandIconCopiesMatch(t *testing.T) {
	pubDir := "../../webui/public/providers"
	entries, err := os.ReadDir(brandAssetsDir)
	if err != nil {
		t.Fatalf("read assets: %v", err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".svg") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(brandAssetsDir, e.Name()))
		if err != nil {
			t.Fatalf("read src %s: %v", e.Name(), err)
		}
		pub, err := os.ReadFile(filepath.Join(pubDir, e.Name()))
		if err != nil {
			t.Errorf("%s exists in src/ but not public/: %v", e.Name(), err)
			continue
		}
		if string(src) != string(pub) {
			t.Errorf("%s differs between src/ and public/", e.Name())
		}
	}
}
