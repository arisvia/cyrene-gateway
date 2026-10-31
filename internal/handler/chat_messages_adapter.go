package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/translator"
)

// anthropicResponseAdapter intercepts an OpenAI ChatCompletion response (stream or non-stream)
// and translates it into an Anthropic Messages format response.
type anthropicResponseAdapter struct {
	w             http.ResponseWriter
	flusher       http.Flusher
	sseTranslator *translator.OpenAIToClaudeSSETranslator
	model         string
	scanBuf       []byte
	buf           bytes.Buffer
	statusCode    int
	isStream      bool
	headerWritten bool
	isSSE         bool
}

func newAnthropicResponseAdapter(w http.ResponseWriter, isStream bool, model string) *anthropicResponseAdapter {
	var flusher http.Flusher
	if f, ok := w.(http.Flusher); ok {
		flusher = f
	}
	return &anthropicResponseAdapter{
		w:             w,
		isStream:      isStream,
		model:         model,
		flusher:       flusher,
		sseTranslator: translator.NewOpenAIToClaudeSSETranslator(model),
		statusCode:    http.StatusOK,
	}
}

func (a *anthropicResponseAdapter) Header() http.Header {
	return a.w.Header()
}

func (a *anthropicResponseAdapter) WriteHeader(statusCode int) {
	a.statusCode = statusCode
	if statusCode < 200 || statusCode >= 300 {
		a.w.WriteHeader(statusCode)
		a.headerWritten = true
		return
	}

	ct := a.w.Header().Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") || a.isStream {
		a.isSSE = true
		a.w.Header().Set("Content-Type", "text/event-stream")
		a.w.Header().Set("Cache-Control", "no-cache, no-transform")
		a.w.Header().Set("Connection", "keep-alive")
		a.w.Header().Set("X-Accel-Buffering", "no")
		a.w.WriteHeader(statusCode)
		a.headerWritten = true
		if a.flusher != nil {
			a.flusher.Flush()
		}
	}
}

func (a *anthropicResponseAdapter) Write(p []byte) (int, error) {
	if !a.headerWritten {
		a.WriteHeader(a.statusCode)
	}
	if a.statusCode < 200 || a.statusCode >= 300 {
		return a.w.Write(anthropicErrorEnvelope(a.statusCode, p))
	}

	if a.isSSE {
		a.scanBuf = append(a.scanBuf, p...)
		for {
			idx := bytes.IndexByte(a.scanBuf, '\n')
			if idx < 0 {
				break
			}
			line := a.scanBuf[:idx]
			a.scanBuf = a.scanBuf[idx+1:]

			if data, isDone, ok := translator.ParseSSEDataLine(line); ok || isDone {
				var chunk []byte
				if isDone {
					chunk = []byte("[DONE]")
				} else {
					chunk = data
				}
				events, _, err := a.sseTranslator.TranslateChunk(chunk)
				if err == nil && len(events) > 0 {
					a.w.Write(events)
					if a.flusher != nil {
						a.flusher.Flush()
					}
				}
				continue
			}

			// SSE comment frames (": ping") keep the stream alive while the
			// upstream queues. Drop them and Cloudflare answers 524 on the
			// idle-looking origin, so forward them verbatim.
			if isSSEComment(line) {
				if err := forwardSSEComment(a.w, a.flusher, line); err != nil {
					return 0, err
				}
			}
		}
		return len(p), nil
	}

	return a.buf.Write(p)
}

func (a *anthropicResponseAdapter) Flush() {
	if a.flusher != nil {
		a.flusher.Flush()
	}
}

func (a *anthropicResponseAdapter) Finish() {
	if a.statusCode < 200 || a.statusCode >= 300 {
		return
	}

	if a.isSSE {
		if len(a.scanBuf) > 0 {
			if data, isDone, ok := translator.ParseSSEDataLine(a.scanBuf); ok || isDone {
				var chunk []byte
				if isDone {
					chunk = []byte("[DONE]")
				} else {
					chunk = data
				}
				events, _, err := a.sseTranslator.TranslateChunk(chunk)
				if err == nil && len(events) > 0 {
					a.w.Write(events)
				}
			}
		}
		events, _, _ := a.sseTranslator.TranslateChunk([]byte("[DONE]"))
		if len(events) > 0 {
			a.w.Write(events)
		}
		if a.flusher != nil {
			a.flusher.Flush()
		}
		return
	}

	// Non-streaming response translation
	claudeResp, err := translator.OpenAIToClaudeResponse(a.buf.Bytes(), a.model)
	if err != nil {
		if !a.headerWritten {
			a.w.WriteHeader(a.statusCode)
			a.headerWritten = true
		}
		a.w.Write(a.buf.Bytes())
		return
	}

	a.w.Header().Set("Content-Type", "application/json")
	if !a.headerWritten {
		a.w.WriteHeader(a.statusCode)
		a.headerWritten = true
	}
	a.w.Write(claudeResp)
}

// handleMessagesViaChat translates an Anthropic /v1/messages request to OpenAI format,
// routes it through the gateway's full Chat Completions pipeline, and translates the response back.
func (s *Server) handleMessagesViaChat(w http.ResponseWriter, r *http.Request, reqBody map[string]any, modelStr string, stream bool) {
	openAIBodyMap, err := translator.ClaudeToOpenAIRequest(reqBody)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, fmt.Sprintf("failed to translate Anthropic request: %v", err))
		return
	}
	openAIBytes, err := json.Marshal(openAIBodyMap)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "failed to encode translated request")
		return
	}

	adapter := newAnthropicResponseAdapter(w, stream, modelStr)
	newReq, err := http.NewRequestWithContext(r.Context(), "POST", "/v1/chat/completions", bytes.NewReader(openAIBytes))
	if err != nil {
		writeAnthropicError(w, http.StatusInternalServerError, "failed to create proxy request")
		return
	}
	newReq.Header = r.Header.Clone()
	// Keep client identity intact for tenant scoping (session affinity fallback).
	newReq.RemoteAddr = r.RemoteAddr
	newReq.Header.Set("Content-Type", "application/json")
	newReq.Header.Set("X-Cyrene-Original-Endpoint", "/v1/messages")
	s.handleChatCompletions(adapter, newReq)
	adapter.Finish()
}

// writeAnthropicError emits an Anthropic-shaped error body directly on w for
// failures raised before the response adapter exists.
func writeAnthropicError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body, _ := json.Marshal(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    anthropicErrorTypeForStatus(status),
			"message": message,
		},
	})
	w.Write(body)
}

// anthropicErrorEnvelope rewrites gateway/OpenAI error bodies ({\"error\": \"...\"}
// flat strings or nested objects) into the {"type":"error","error":{type,message}}
// shape Anthropic SDKs parse, so clients can classify and back off instead of
// surfacing an empty message. Already-shaped bodies pass through untouched.
func anthropicErrorEnvelope(status int, body []byte) []byte {
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err == nil {
		if parsed["type"] == "error" {
			return body
		}
	}
	message := string(bytes.TrimSpace(body))
	if err := json.Unmarshal(body, &parsed); err == nil {
		switch e := parsed["error"].(type) {
		case string:
			if e != "" {
				message = e
			}
		case map[string]any:
			if m, ok := e["message"].(string); ok && m != "" {
				message = m
			}
		}
	}
	out, err := json.Marshal(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    anthropicErrorTypeForStatus(status),
			"message": message,
		},
	})
	if err != nil {
		return body
	}
	return out
}

func anthropicErrorTypeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusRequestEntityTooLarge:
		return "request_too_large"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusServiceUnavailable:
		return "overloaded_error"
	default:
		return "api_error"
	}
}
