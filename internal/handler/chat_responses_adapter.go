package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/arisvia/cyrene-gateway/internal/translator"
)

// responsesResponseAdapter intercepts an OpenAI ChatCompletion response (stream or non-stream)
// and translates it into an OpenAI Responses API (/v1/responses) format response.
type responsesResponseAdapter struct {
	w             http.ResponseWriter
	flusher       http.Flusher
	sseTranslator *translator.OpenAIToResponsesSSETranslator
	model         string
	scanBuf       []byte
	buf           bytes.Buffer
	statusCode    int
	isStream      bool
	headerWritten bool
	finished      bool
}

func newResponsesResponseAdapter(w http.ResponseWriter, isStream bool, model string) *responsesResponseAdapter {
	var flusher http.Flusher
	if f, ok := w.(http.Flusher); ok {
		flusher = f
	}
	return &responsesResponseAdapter{
		w:             w,
		isStream:      isStream,
		model:         model,
		flusher:       flusher,
		sseTranslator: translator.NewOpenAIToResponsesSSETranslator(model),
		statusCode:    http.StatusOK,
	}
}

func (a *responsesResponseAdapter) Header() http.Header {
	return a.w.Header()
}

func (a *responsesResponseAdapter) WriteHeader(statusCode int) {
	a.statusCode = statusCode
	if statusCode < 200 || statusCode >= 300 {
		a.w.WriteHeader(statusCode)
		a.headerWritten = true
		return
	}
	if a.isStream {
		a.w.Header().Set("Content-Type", "text/event-stream")
		a.w.Header().Set("Cache-Control", "no-cache")
		a.w.Header().Set("Connection", "keep-alive")
		a.w.Header().Set("X-Accel-Buffering", "no")
		a.w.WriteHeader(statusCode)
		a.headerWritten = true
	}
}

func (a *responsesResponseAdapter) Write(p []byte) (int, error) {
	if a.statusCode < 200 || a.statusCode >= 300 {
		return a.w.Write(p)
	}
	if a.finished {
		return len(p), nil
	}

	if a.isStream {
		if !a.headerWritten {
			a.WriteHeader(http.StatusOK)
		}
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
				translatedEvents, isFinished, err := a.sseTranslator.TranslateChunk(chunk)
				if err != nil {
					return 0, err
				}
				if len(translatedEvents) > 0 {
					if _, err := a.w.Write(translatedEvents); err != nil {
						return 0, err
					}
					if a.flusher != nil {
						a.flusher.Flush()
					}
				}
				if isFinished {
					a.finished = true
					a.scanBuf = nil
					return len(p), nil
				}
			}
		}
		return len(p), nil
	}

	return a.buf.Write(p)
}

func (a *responsesResponseAdapter) Flush() {
	if a.flusher != nil {
		a.flusher.Flush()
	}
}

func (a *responsesResponseAdapter) Finish() {
	if a.finished || a.statusCode < 200 || a.statusCode >= 300 {
		return
	}

	if a.isStream {
		if len(a.scanBuf) > 0 {
			if _, err := a.Write([]byte("\n")); err != nil {
				return
			}
		}
		if !a.finished {
			_, _ = a.Write([]byte("data: [DONE]\n\n"))
		}
		return
	}

	a.finished = true
	// Non-streaming response conversion
	raw := a.buf.Bytes()
	if len(raw) == 0 {
		return
	}

	converted, err := translator.OpenAIToResponsesResponse(raw, a.model)
	if err != nil {
		// Fallback to raw response if translation fails
		a.w.Header().Set("Content-Type", "application/json")
		if !a.headerWritten {
			a.w.WriteHeader(a.statusCode)
			a.headerWritten = true
		}
		a.w.Write(raw)
		return
	}

	a.w.Header().Set("Content-Type", "application/json")
	if !a.headerWritten {
		a.w.WriteHeader(a.statusCode)
		a.headerWritten = true
	}
	a.w.Write(converted)
}

// handleResponses processes OpenAI Responses API requests (/v1/responses).
// It converts Responses API payload (supporting string or array input, instructions, tools, etc.)
// into Chat Completions format, routes through the universal gateway core, and converts
// the result back into Responses API format (both non-stream and SSE stream).
func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	var reqMap map[string]any
	if err := json.Unmarshal(rawBody, &reqMap); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	modelStr, _ := reqMap["model"].(string)
	if modelStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model is required"})
		return
	}

	stream, _ := reqMap["stream"].(bool)

	// Convert Responses API payload to Chat Completions payload
	chatReqMap, err := translator.ResponsesToOpenAIRequest(reqMap)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("failed to parse responses request: %v", err)})
		return
	}

	chatBodyBytes, err := json.Marshal(chatReqMap)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encode translated request"})
		return
	}

	adapter := newResponsesResponseAdapter(w, stream, modelStr)
	newReq, err := http.NewRequestWithContext(r.Context(), "POST", "/v1/chat/completions", bytes.NewReader(chatBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create proxy request"})
		return
	}
	newReq.Header = r.Header.Clone()
	newReq.Header.Set("Content-Type", "application/json")
	newReq.Header.Set("X-Cyrene-Original-Endpoint", "/v1/responses")
	s.handleChatCompletions(adapter, newReq)
	adapter.Finish()
}
