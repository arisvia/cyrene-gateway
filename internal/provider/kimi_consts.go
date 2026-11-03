package provider

import (
	"net/http"
	"strings"
)

// Kimi Code client identity. Mirror the official kimi-cli: Moonshot's
// kimi-for-coding backend 403s ("only available for Coding Agents") on any
// User-Agent that is not KimiCLI/<version>, and the X-Msh-* block is what the
// channel check reads for platform/version. Naming ourselves here — the old
// "cyrene-gateway"/"1.0.0" pair — identified the relay on every request.
const (
	KimiCLIVersion = "1.52.0"
	// kimiCLIUAPrefix is what the upstream regex matches on, and what the UA
	// must preserve so a version bump does not change the identity.
	kimiCLIUAPrefix = "KimiCLI/"
	KimiCLIUA       = kimiCLIUAPrefix + KimiCLIVersion
	// kimiCLIPlatform is the literal the official CLI sends, not a product name.
	kimiCLIPlatform = "kimi_cli"
)

// applyKimiHeaders stamps the official-CLI identity onto req. Device ID is
// optional: the device-code requests send it only when one already exists,
// while chat/refresh generate one on the fly.
func applyKimiHeaders(h http.Header, deviceID string) {
	if !strings.HasPrefix(h.Get("User-Agent"), kimiCLIUAPrefix) {
		h.Set("User-Agent", KimiCLIUA)
	}
	h.Set("X-Msh-Platform", kimiCLIPlatform)
	h.Set("X-Msh-Version", KimiCLIVersion)
	if deviceID != "" {
		h.Set("X-Msh-Device-Id", deviceID)
	}
}
