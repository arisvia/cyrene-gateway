package cache

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
)

// ResponseRecorder wraps an http.ResponseWriter to capture status code, headers,
// and body for response caching without breaking streaming or underlying behavior.
type ResponseRecorder struct {
	underlying  http.ResponseWriter
	statusCode  int
	body        bytes.Buffer
	wroteHeader bool
}

// NewRecorder creates a ResponseRecorder wrapping w.
func NewRecorder(w http.ResponseWriter) *ResponseRecorder {
	return &ResponseRecorder{
		underlying: w,
		statusCode: http.StatusOK,
	}
}

// Header returns the header map from the underlying ResponseWriter.
func (r *ResponseRecorder) Header() http.Header {
	return r.underlying.Header()
}

// WriteHeader records the status code and delegates to underlying ResponseWriter.
func (r *ResponseRecorder) WriteHeader(statusCode int) {
	if !r.wroteHeader {
		r.statusCode = statusCode
		r.wroteHeader = true
		r.underlying.WriteHeader(statusCode)
	}
}

// Write delegates to the underlying ResponseWriter, buffering 2xx responses.
func (r *ResponseRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.underlying.Write(b)
	if err == nil && r.statusCode >= 200 && r.statusCode < 300 {
		ct := r.underlying.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/event-stream") {
			r.body.Write(b[:n])
		}
	}
	return n, err
}

// Flush forwards to the underlying ResponseWriter if it implements http.Flusher.
func (r *ResponseRecorder) Flush() {
	if flusher, ok := r.underlying.(http.Flusher); ok {
		flusher.Flush()
	}
}

// ShouldCache checks if the recorded response satisfies caching conditions (2xx, non-SSE, non-empty).
func (r *ResponseRecorder) ShouldCache() bool {
	if r.statusCode < 200 || r.statusCode >= 300 {
		return false
	}
	ct := r.underlying.Header().Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		return false
	}
	return r.body.Len() > 0
}

// BodyBytes returns a copy of the captured body.
func (r *ResponseRecorder) BodyBytes() []byte {
	return r.body.Bytes()
}

// StatusCode returns the captured status code.
func (r *ResponseRecorder) StatusCode() int {
	return r.statusCode
}

// HeaderMap returns a simplified header map suitable for caching.
func (r *ResponseRecorder) HeaderMap() map[string]string {
	m := make(map[string]string)
	h := r.underlying.Header()
	if ct := h.Get("Content-Type"); ct != "" {
		m["Content-Type"] = ct
	}
	if sm := h.Get("X-Cyrene-Served-Model"); sm != "" {
		m["X-Cyrene-Served-Model"] = sm
	}
	return m
}

// ExtractTokens parses token counts from the response body (OpenAI, Gemini, Anthropic shapes).
func (r *ResponseRecorder) ExtractTokens() int {
	body := r.body.Bytes()
	if len(body) == 0 {
		return 0
	}

	// OpenAI / Gemini / general format: {"usage": {"total_tokens": N}}
	var openAIUsage struct {
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &openAIUsage); err == nil && openAIUsage.Usage.TotalTokens > 0 {
		return openAIUsage.Usage.TotalTokens
	}

	// Anthropic format: {"usage": {"input_tokens": N, "output_tokens": M}}
	var anthropicUsage struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &anthropicUsage); err == nil {
		total := anthropicUsage.Usage.InputTokens + anthropicUsage.Usage.OutputTokens
		if total > 0 {
			return total
		}
	}

	return 0
}
