package provider

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// Grok Build (Grok CLI) identity, from a wire capture of @xai-official/grok.
// cli-chat-proxy rejects old client identities with HTTP 426, so the version
// here is a gate, not cosmetic: keep it on a current grok release.
const (
	GrokCLIVersion          = "1.0.44"
	GrokCLIBaseURL          = "https://cli-chat-proxy.grok.com/v1"
	GrokCLIClientIdentifier = "grok-shell"
	GrokCLIUserAgent        = "grok-shell/" + GrokCLIVersion + " (linux; x86_64)"
	GrokCLIModel            = "grok-build"
)

// grokCliHeadersHook stamps the per-request CLI identity. cli-chat-proxy keys
// conversation state off x-grok-session-id/conv-id and rejects turns whose
// x-grok-turn-idx regresses, so these cannot be omitted even though they are
// not part of the registry's static header block.
func grokCliHeadersHook(h http.Header, c Credentials) {
	if h.Get("User-Agent") == "" {
		h.Set("User-Agent", GrokCLIUserAgent)
	}
	h.Set("x-grok-client-identifier", GrokCLIClientIdentifier)
	h.Set("x-grok-client-version", GrokCLIVersion)

	sessionID := strings.TrimSpace(h.Get("x-grok-session-id"))
	if sessionID == "" {
		if c.ProviderSpecificData != nil {
			sessionID, _ = c.ProviderSpecificData["sessionId"].(string)
		}
	}
	if sessionID == "" {
		sessionID = uuid.NewString()
	}
	h.Set("x-grok-session-id", sessionID)
	h.Set("x-grok-conv-id", sessionID)
	if h.Get("x-grok-req-id") == "" {
		h.Set("x-grok-req-id", uuid.NewString())
	}
	if h.Get("x-grok-turn-idx") == "" {
		h.Set("x-grok-turn-idx", "1")
	}
	if c.ProviderSpecificData != nil && h.Get("x-email") == "" {
		if email, ok := c.ProviderSpecificData["email"].(string); ok && email != "" {
			h.Set("x-email", email)
		}
	}
}

func init() {
	authHooks["grokCliHeaders"] = grokCliHeadersHook
}

// Grok CLI OAuth identity: the CLI requests conversation scopes beyond the
// plain API token and identifies itself as grok-build on the authorize leg.
const (
	GrokCLIOAuthScope = "openid profile email offline_access grok-cli:access api:access conversations:read conversations:write"
	GrokCLIReferrer   = "grok-build"
)
