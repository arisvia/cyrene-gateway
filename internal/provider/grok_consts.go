package provider

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/google/uuid"
)

// Grok Build (Grok CLI) identity, from a wire capture of @xai-official/grok.
// cli-chat-proxy rejects old client identities with HTTP 426, so the version
// lives in grokCLITracker (cli_version.go) rather than a rotting constant.
const (
	GrokCLIBaseURL          = "https://cli-chat-proxy.grok.com/v1"
	GrokCLIClientIdentifier = "grok-shell"
	GrokCLIModel            = "grok-build"
)

// applyGrokConversationHeaders populates stable session ID and dynamic user-turn
// count for cli-chat-proxy before authHooks runs.
func applyGrokConversationHeaders(req *http.Request, bodyMap map[string]any, conn *model.ProviderConnection) {
	if req.Header.Get("x-grok-session-id") == "" {
		var sessionID string
		if conn != nil && conn.Data.ProviderSpecificData != nil {
			sessionID, _ = conn.Data.ProviderSpecificData["sessionId"].(string)
			sessionID = strings.TrimSpace(sessionID)
		}
		if sessionID == "" {
			conversationKey := SessionKeyFromBodyMap(bodyMap)
			if conversationKey == "" {
				conversationKey = FirstUserText(bodyMap)
			}
			if conversationKey != "" {
				sessionID = stableNamespaceUUID("grok:" + conversationKey)
			} else {
				sessionID = uuid.NewString()
			}
		}
		req.Header.Set("x-grok-session-id", sessionID)
		req.Header.Set("x-grok-conv-id", sessionID)
	}
	if req.Header.Get("x-grok-turn-idx") == "" {
		turns := 0
		if rawMsgs, ok := bodyMap["messages"].([]any); ok {
			for _, m := range rawMsgs {
				if msg, ok := m.(map[string]any); ok {
					if role, _ := msg["role"].(string); role == "user" {
						turns++
					}
				}
			}
		}
		if turns < 1 {
			turns = 1
		}
		req.Header.Set("x-grok-turn-idx", strconv.Itoa(turns))
	}
}

// grokCliHeadersHook stamps the per-request CLI identity. cli-chat-proxy keys
// conversation state off x-grok-session-id/conv-id and rejects turns whose
// x-grok-turn-idx regresses, so these cannot be omitted even though they are
// not part of the registry's static header block.
func grokCliHeadersHook(h http.Header, c Credentials) {
	h.Set("User-Agent", GrokCLIUserAgent())
	h.Set("x-grok-client-identifier", GrokCLIClientIdentifier)
	h.Set("x-grok-client-version", GrokCLIVersion())

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
	if h.Get("x-grok-conv-id") == "" {
		h.Set("x-grok-conv-id", sessionID)
	}
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
