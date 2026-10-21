package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPrimeSSEBypassesProxyBuffering pins the SSE handshake contract: the
// response must be text/event-stream, padded past the 4-8KB threshold that
// Cloudflare and Nginx buffers hold before forwarding bytes.
func TestPrimeSSEBypassesProxyBuffering(t *testing.T) {
	w := httptest.NewRecorder()
	primeSSE(w, w)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	// RFC 9110 §8.4 removed identity from Content-Encoding: sending it makes
	// browsers/proxies fail with ERR_CONTENT_DECODING_FAILED. no-transform is
	// the correct way to keep proxies from compressing the pad.
	if enc := w.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("Content-Encoding = %q, want empty (identity is invalid in responses)", enc)
	}
	if accel := w.Header().Get("X-Accel-Buffering"); accel != "no" {
		t.Errorf("X-Accel-Buffering = %q, want no", accel)
	}

	body := w.Body.String()
	if len(body) < ssePrimeBytes {
		t.Errorf("padding = %d bytes, want at least %d", len(body), ssePrimeBytes)
	}
	// Every line must be an SSE comment so EventSource discards it silently.
	for _, line := range strings.Split(body, "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, ":") {
			t.Fatalf("non-comment line leaked into the SSE pad: %q", line)
		}
	}
}
