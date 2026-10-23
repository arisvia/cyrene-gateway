package handler

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/google/uuid"
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
	id := uuid.New()
	return hex.EncodeToString(id[:])
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

// isSSEComment reports whether an SSE line is a comment/keepalive frame, i.e. it
// starts with ':' (": ping", ": keepalive"). Proxies must forward these so an
// idle-looking stream is not mistaken for a dead origin.
func isSSEComment(line []byte) bool {
	trimmed := bytes.TrimSpace(line)
	return len(trimmed) > 0 && trimmed[0] == ':'
}

// forwardSSEComment relays an SSE comment/keepalive frame to the client and
// flushes immediately. The terminating blank line is appended here because the
// adapter loops consume lines one at a time; without it the frame would be
// unterminated and buffered by the edge until the next real event.
func forwardSSEComment(w http.ResponseWriter, flusher http.Flusher, line []byte) error {
	if _, err := w.Write(append(append([]byte{}, bytes.TrimSpace(line)...), '\n', '\n')); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

// maxSSELineBytes caps a single SSE line. Upstreams occasionally emit one huge
// frame (a multi-megabyte tool argument or reasoning blob); a 1MB cap made the
// scanner abort silently and the stream just stopped, so allow more headroom.
const maxSSELineBytes = 8 * 1024 * 1024

// captureResponseWriter records a handler's status, headers and body in memory so
// one protocol surface can delegate to another in-process (Gemini embedContent →
// /v1/embeddings) without a loopback HTTP round trip.
type captureResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newCaptureResponseWriter() *captureResponseWriter {
	return &captureResponseWriter{header: make(http.Header)}
}

func (c *captureResponseWriter) Header() http.Header { return c.header }

func (c *captureResponseWriter) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

func (c *captureResponseWriter) Write(p []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return c.body.Write(p)
}

// Status returns the recorded status, defaulting to 200 like net/http.
func (c *captureResponseWriter) Status() int {
	if c.status == 0 {
		return http.StatusOK
	}
	return c.status
}
