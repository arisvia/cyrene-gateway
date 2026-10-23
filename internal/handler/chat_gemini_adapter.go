package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/rtk"
	"github.com/arisvia/cyrene-gateway/internal/translator"
)

// geminiResponseAdapter intercepts an OpenAI ChatCompletion response (stream or non-stream)
// and translates it into a Google Gemini generateContent response.
type geminiResponseAdapter struct {
	w             http.ResponseWriter
	flusher       http.Flusher
	sseTranslator *translator.OpenAIToGeminiSSETranslator
	model         string
	scanBuf       []byte
	buf           bytes.Buffer
	statusCode    int
	isStream      bool
	headerWritten bool
}

func newGeminiResponseAdapter(w http.ResponseWriter, isStream bool, model string) *geminiResponseAdapter {
	var flusher http.Flusher
	if f, ok := w.(http.Flusher); ok {
		flusher = f
	}
	return &geminiResponseAdapter{
		w:             w,
		isStream:      isStream,
		model:         model,
		flusher:       flusher,
		sseTranslator: translator.NewOpenAIToGeminiSSETranslator(model),
		statusCode:    http.StatusOK,
	}
}

func (a *geminiResponseAdapter) Header() http.Header {
	return a.w.Header()
}

func (a *geminiResponseAdapter) WriteHeader(statusCode int) {
	a.statusCode = statusCode
	if statusCode < 200 || statusCode >= 300 {
		a.w.WriteHeader(statusCode)
		a.headerWritten = true
		return
	}
	if a.isStream {
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

func (a *geminiResponseAdapter) Write(p []byte) (int, error) {
	if a.statusCode < 200 || a.statusCode >= 300 {
		return a.w.Write(p)
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
				translatedEvents, _, err := a.sseTranslator.TranslateChunk(chunk)
				if err == nil && len(translatedEvents) > 0 {
					a.w.Write(translatedEvents)
					if a.flusher != nil {
						a.flusher.Flush()
					}
				}
				continue
			}

			// SSE comment frames (": ping") keep the connection alive while the
			// upstream queues. Dropping them makes the edge see an idle origin
			// and answer 524 before the first token arrives.
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

func (a *geminiResponseAdapter) Flush() {
	if a.flusher != nil {
		a.flusher.Flush()
	}
}

func (a *geminiResponseAdapter) Finish() {
	if a.statusCode < 200 || a.statusCode >= 300 {
		return
	}

	if a.isStream {
		if !a.headerWritten {
			a.WriteHeader(http.StatusOK)
		}
		if len(a.scanBuf) > 0 {
			if data, isDone, ok := translator.ParseSSEDataLine(a.scanBuf); ok || isDone {
				var chunk []byte
				if isDone {
					chunk = []byte("[DONE]")
				} else {
					chunk = data
				}
				translatedEvents, _, _ := a.sseTranslator.TranslateChunk(chunk)
				if len(translatedEvents) > 0 {
					a.w.Write(translatedEvents)
				}
			}
		}
		// Send final [DONE] translation
		doneEvents, _, _ := a.sseTranslator.TranslateChunk([]byte("[DONE]"))
		if len(doneEvents) > 0 {
			a.w.Write(doneEvents)
		}
		if a.flusher != nil {
			a.flusher.Flush()
		}
		return
	}

	raw := a.buf.Bytes()
	if len(raw) == 0 {
		return
	}

	converted, err := translator.OpenAIToGeminiResponse(raw, a.model)
	if err != nil {
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

// handleGeminiGenerateContent processes Google Gemini generateContent requests.
// Route: POST /v1beta/models/{modelAction...}
func (s *Server) handleGeminiGenerateContent(w http.ResponseWriter, r *http.Request) {
	modelAction := r.PathValue("modelAction")
	if strings.HasPrefix(modelAction, "models/") {
		modelAction = strings.TrimPrefix(modelAction, "models/")
	}

	parts := strings.Split(modelAction, ":")
	modelStr := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	if modelStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model is required in path"})
		return
	}

	isStream := action == "streamGenerateContent" || r.URL.Query().Get("alt") == "sse" || r.Header.Get("Accept") == "text/event-stream"

	rawBody, err := readRequestBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	var reqMap map[string]any
	if len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, &reqMap); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
	} else {
		reqMap = make(map[string]any)
	}

	// Gemini exposes several operations under the same model path. Only the
	// generateContent family is a chat completion; the others have their own
	// response shapes and must not be forwarded as generation requests.
	switch action {
	case "countTokens":
		s.handleGeminiCountTokens(w, r, reqMap, modelStr)
		return
	case "embedContent", "batchEmbedContents":
		s.handleGeminiEmbedContent(w, r, reqMap, modelStr, action == "batchEmbedContents")
		return
	case "", "generateContent", "streamGenerateContent":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("unsupported method %q for model %s", action, modelStr),
		})
		return
	}

	chatReqMap, err := translator.GeminiToOpenAIRequest(reqMap, modelStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("failed to parse Gemini request: %v", err)})
		return
	}
	if isStream {
		chatReqMap["stream"] = true
		chatReqMap["stream_options"] = map[string]any{"include_usage": true}
	}

	chatBodyBytes, err := json.Marshal(chatReqMap)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encode translated request"})
		return
	}

	adapter := newGeminiResponseAdapter(w, isStream, modelStr)
	newReq, err := http.NewRequestWithContext(r.Context(), "POST", "/v1/chat/completions", bytes.NewReader(chatBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create proxy request"})
		return
	}
	newReq.Header = r.Header.Clone()
	// Preserve the inbound query string (e.g. Gemini REST "?key=<k>") so the
	// shared extractRequestAPIKey can attribute usage for ?key= callers.
	newReq.URL.RawQuery = r.URL.RawQuery
	newReq.Header.Set("Content-Type", "application/json")
	endpointPrefix := "/v1beta/models/"
	if strings.HasPrefix(r.URL.Path, "/v1/") {
		endpointPrefix = "/v1/models/"
	}
	newReq.Header.Set("X-Cyrene-Original-Endpoint", endpointPrefix+modelAction)
	s.handleChatCompletions(adapter, newReq)
	adapter.Finish()
}

// handleGeminiCountTokens answers :countTokens with a local estimate. Gemini
// expects {"totalTokens": N}; routing it through generation would return a
// candidates payload the SDK cannot parse.
func (s *Server) handleGeminiCountTokens(w http.ResponseWriter, r *http.Request, reqMap map[string]any, modelStr string) {
	var sb strings.Builder
	if sys, ok := reqMap["systemInstruction"].(map[string]any); ok {
		sb.WriteString(geminiPartsText(sys["parts"]))
	}
	if contents, ok := reqMap["contents"].([]any); ok {
		for _, c := range contents {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			sb.WriteString(geminiPartsText(cm["parts"]))
		}
	}
	if tools, ok := reqMap["tools"]; ok {
		if b, err := json.Marshal(tools); err == nil {
			sb.Write(b)
		}
	}

	total := rtk.EstimateTextTokens(sb.String())
	if total < 1 {
		total = 1
	}
	writeJSON(w, http.StatusOK, map[string]any{"totalTokens": total})
}

// geminiPartsText concatenates the text of a Gemini parts array.
func geminiPartsText(raw any) string {
	parts, ok := raw.([]any)
	if !ok {
		return ""
	}
	var sb strings.Builder
	for _, p := range parts {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := pm["text"].(string); ok {
			sb.WriteString(t)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// handleGeminiEmbedContent answers :embedContent and :batchEmbedContents by
// translating to the OpenAI embeddings shape and translating the reply back.
func (s *Server) handleGeminiEmbedContent(w http.ResponseWriter, r *http.Request, reqMap map[string]any, modelStr string, batch bool) {
	var inputs []string
	if batch {
		reqs, _ := reqMap["requests"].([]any)
		for _, rq := range reqs {
			rm, ok := rq.(map[string]any)
			if !ok {
				continue
			}
			if cm, ok := rm["content"].(map[string]any); ok {
				inputs = append(inputs, strings.TrimSpace(geminiPartsText(cm["parts"])))
			}
		}
	} else {
		if cm, ok := reqMap["content"].(map[string]any); ok {
			inputs = append(inputs, strings.TrimSpace(geminiPartsText(cm["parts"])))
		}
	}
	if len(inputs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content.parts is required"})
		return
	}

	embReq := map[string]any{"model": modelStr, "input": inputs}
	body, err := json.Marshal(embReq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encode embeddings request"})
		return
	}

	rec := newCaptureResponseWriter()
	newReq, err := http.NewRequestWithContext(r.Context(), "POST", "/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create proxy request"})
		return
	}
	newReq.Header = r.Header.Clone()
	newReq.URL.RawQuery = r.URL.RawQuery
	newReq.Header.Set("Content-Type", "application/json")
	s.handleEmbeddings(rec, newReq)

	if status := rec.Status(); status < 200 || status >= 300 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(rec.body.Bytes())
		return
	}

	var parsed struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rec.body.Bytes(), &parsed); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid embeddings response"})
		return
	}

	embeddings := make([]any, 0, len(parsed.Data))
	for _, d := range parsed.Data {
		embeddings = append(embeddings, map[string]any{"values": d.Embedding})
	}

	if batch {
		writeJSON(w, http.StatusOK, map[string]any{"embeddings": embeddings})
		return
	}
	out := map[string]any{}
	if len(embeddings) > 0 {
		out["embedding"] = embeddings[0]
	}
	writeJSON(w, http.StatusOK, out)
}
