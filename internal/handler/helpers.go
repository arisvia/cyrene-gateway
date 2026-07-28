package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"runtime/debug"
	"strings"
)

// version is set via ldflags at build time: -ldflags "-X .../handler.version=v0.3.0"
var version string

// Version returns the build version from ldflags, git tag (via build info), or "dev".
// The leading "v" prefix (from git tags) is stripped so the UI can add its own.
func Version() string {
	v := version
	if v == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			if info.Main.Version != "" && info.Main.Version != "(devel)" {
				v = info.Main.Version
			}
		}
	}
	if v == "" {
		return "dev"
	}
	return strings.TrimPrefix(v, "v")
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// wrapErrorWithHint enriches upstream error responses with actionable hints (Phase 26 Error UX).
func wrapErrorWithHint(statusCode int, errBody []byte, providerID string) []byte {
	hint := ""
	switch statusCode {
	case 401:
		hint = "Authentication failed. Reconnect your account or update the API key for " + providerID + "."
	case 403:
		hint = "Access denied. This may be a geo/IP restriction or insufficient permissions for " + providerID + "."
	case 429:
		hint = "Rate limited. The connection is in cooldown and will retry automatically."
	case 529:
		hint = "Provider overloaded. Try again in a moment or switch to a different provider."
	}

	if hint == "" {
		return errBody
	}

	// Try to inject hint into existing JSON error object
	var obj map[string]any
	if err := json.Unmarshal(errBody, &obj); err == nil {
		if errObj, ok := obj["error"].(map[string]any); ok {
			errObj["hint"] = hint
		} else {
			obj["hint"] = hint
		}
		out, _ := json.Marshal(obj)
		return out
	}

	// Fallback: wrap raw body
	out, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": string(errBody),
			"code":    statusCode,
			"hint":    hint,
		},
	})
	return out
}
