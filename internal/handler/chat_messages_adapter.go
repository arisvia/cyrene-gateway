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
	if a.statusCode < 200 || a.statusCode >= 300 {
		return a.w.Write(p)
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("failed to translate Anthropic request: %v", err)})
		return
	}
	openAIBytes, err := json.Marshal(openAIBodyMap)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to encode translated request"})
		return
	}

	adapter := newAnthropicResponseAdapter(w, stream, modelStr)
	newReq, err := http.NewRequestWithContext(r.Context(), "POST", "/v1/chat/completions", bytes.NewReader(openAIBytes))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create proxy request"})
		return
	}
	newReq.Header = r.Header.Clone()
	newReq.Header.Set("Content-Type", "application/json")

	s.handleChatCompletions(adapter, newReq)
	adapter.Finish()
}
