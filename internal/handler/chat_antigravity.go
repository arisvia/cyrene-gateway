package handler

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
	"github.com/arisvia/cyrene-gateway/internal/translator"
	"github.com/arisvia/cyrene-gateway/internal/usage"
)

// handleAntigravityChat handles inference requests via Google Cloud Code Assist (Antigravity).
// It returns retryable=true when the account itself failed in a way another
// account could survive (quota exhausted, rate limited). In that case nothing has
// been written to w yet, so the caller may retry with the next connection. A false
// return means the response is already complete.
func (s *Server) handleAntigravityChat(
	w http.ResponseWriter,
	r *http.Request,
	req ChatCompletionRequest,
	rawBody []byte,
	modelInfo model.ModelInfo,
	conn *model.ProviderConnection,
	providerInfo provider.ProviderInfo,
) (retryable bool) {
	token := conn.Data.AccessToken
	if token == "" && conn.Data.APIKey != "" {
		token = conn.Data.APIKey
	}
	if token == "" {
		// No usable credential on this account: another one may have one.
		return true
	}

	clientTimeout := 3 * time.Minute
	if req.Stream {
		clientTimeout = 0
	}
	client := s.getHTTPClient(clientTimeout)
	projectID := ""
	if conn.Data.ProviderSpecificData != nil {
		if pid, ok := conn.Data.ProviderSpecificData["projectId"].(string); ok && pid != "" {
			projectID = pid
		}
	}
	if projectID == "" {
		pid, err := s.EnsureAntigravityProject(r.Context(), conn, client)
		if err != nil {
			if conn.Data.BaseURL == "" && providerInfo.BaseURL == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": fmt.Sprintf("antigravity project not configured and discovery failed: %v", err),
				})
				return
			}
		} else {
			projectID = pid
		}
	}

	// Transform OpenAI messages into Gemini contents format
	contents, systemInstruction := convertOpenAIToGeminiContents(req.Messages, modelInfo.Model)

	genConfig := map[string]any{}
	// Gemini 4 removes external sampling params (temperature/topP); forwarding them causes 400 INVALID_ARGUMENT (9router #4802 parity)
	if !isGemini4Model(modelInfo.Model) {
		if req.Temperature != nil {
			genConfig["temperature"] = *req.Temperature
		}
	}
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		genConfig["maxOutputTokens"] = *req.MaxTokens
	}
	sessionID := fmt.Sprintf("-%d", time.Now().UnixNano()%9000000000000000000)
	innerRequest := map[string]any{
		"contents":  contents,
		"sessionId": sessionID,
	}
	if len(systemInstruction) > 0 {
		innerRequest["systemInstruction"] = map[string]any{
			"parts": systemInstruction,
		}
	}
	// Forward tool definitions if present
	if len(req.Tools) > 0 {
		var rawTools []map[string]any
		if err := json.Unmarshal(req.Tools, &rawTools); err == nil && len(rawTools) > 0 {
			var declarations []any
			for _, t := range rawTools {
				var name, description string
				var params map[string]any

				if fn, ok := t["function"].(map[string]any); ok {
					name, _ = fn["name"].(string)
					description, _ = fn["description"].(string)
					params, _ = fn["parameters"].(map[string]any)
				}
				if name == "" {
					name, _ = t["name"].(string)
					description, _ = t["description"].(string)
					if p, ok := t["parameters"].(map[string]any); ok {
						params = p
					} else if s, ok := t["input_schema"].(map[string]any); ok {
						params = s
					}
				}
				if name != "" {
					if params != nil {
						params = translator.CleanJSONSchemaForGemini(params)
					}
					declarations = append(declarations, map[string]any{
						"name":        name,
						"description": description,
						"parameters":  params,
					})
				}
			}
			if len(declarations) > 0 {
				innerRequest["tools"] = []any{map[string]any{"functionDeclarations": declarations}}
			}
		}
	}

	availableModels := s.getAntigravityAvailableModels()
	targetModel, tier, shouldInjectThinking, isImage := resolveAntigravityModel(modelInfo.Model, req.ReasoningEffort, availableModels)

	if !isImage {
		s.applyLoopGuard(innerRequest, rawBody, "gemini", req.Tools != nil)
		s.applyTokenSaver(innerRequest, "gemini", modelInfo.Provider, r)
	}
	if isImage {
		genConfig["maxOutputTokens"] = 8192
		genConfig["imageConfig"] = map[string]any{"aspectRatio": "1:1"}
	} else if shouldInjectThinking && genConfig != nil && (tier == "high" || tier == "medium" || tier == "low") {
		genConfig["thinkingConfig"] = map[string]any{
			"thinkingLevel":   tier,
			"includeThoughts": true,
		}
		// Ensure maxOutputTokens strictly exceeds thinking budget to prevent 400 INVALID_ARGUMENT (9router#3981)
		minBudget := 4096
		if tier == "high" {
			minBudget = 16384
		} else if tier == "medium" {
			minBudget = 8192
		}
		if mt, ok := genConfig["maxOutputTokens"].(int); ok && mt <= minBudget {
			genConfig["maxOutputTokens"] = minBudget + 8192
		}
	}
	if len(genConfig) > 0 {
		innerRequest["generationConfig"] = genConfig
	}

	upstreamAction := "streamGenerateContent?alt=sse"
	if isImage {
		upstreamAction = "generateContent"
	}

	reqID := fmt.Sprintf("agent-%d-%s", time.Now().UnixMilli(), randomHex(4))
	envelope := map[string]any{
		"model":     targetModel,
		"request":   innerRequest,
		"userAgent": "antigravity",
		"requestId": reqID,
	}
	if projectID != "" {
		envelope["project"] = projectID
	}
	if isImage {
		envelope["requestType"] = "image_gen"
	}

	envelopeBytes, err := json.Marshal(envelope)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encode antigravity request"})
		return
	}

	baseURL := provider.AntigravityBaseURL
	if conn.Data.BaseURL != "" {
		baseURL = strings.TrimRight(conn.Data.BaseURL, "/")
	} else if providerInfo.BaseURL != "" {
		baseURL = strings.TrimRight(providerInfo.BaseURL, "/")
	}
	upstreamURL := fmt.Sprintf("%s/v1internal:%s", baseURL, upstreamAction)
	upReq, err := http.NewRequestWithContext(r.Context(), "POST", upstreamURL, bytes.NewReader(envelopeBytes))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upstream request"})
		return
	}

	upReq.Header.Set("Authorization", "Bearer "+token)
	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("Accept", "text/event-stream")
	upReq.Header.Set("User-Agent", provider.AntigravityUserAgent)

	endpoint := resolveRequestEndpoint(r, "/v1/chat/completions")
	start := time.Now()
	s.publishRequestEvent(r, RequestEvent{Type: "routing", Provider: conn.Provider, Model: modelInfo.Model, Endpoint: endpoint, Status: "routing"})
	resp, err := client.Do(upReq)
	if err != nil {
		if r.Context().Err() != nil {
			return false
		}
		// Transport failures are per-account as often as per-network; let the
		// caller try the next account before surfacing a 502.
		slog.Warn("Antigravity upstream connection failed", "error", err, "connection", conn.ID)
		return true
	}
	defer resp.Body.Close()

	// Phase 9: On-401 retry with token refresh for OAuth connections
	if resp.StatusCode == http.StatusUnauthorized && conn.Data.RefreshToken != "" {
		resp.Body.Close()
		slog.Warn("Antigravity upstream returned 401, attempting token refresh and retry",
			slog.String("provider", conn.Provider),
			slog.String("connection_id", conn.ID))
		refreshResult, refreshErr := provider.RefreshCredentials(conn.Provider, conn, s.getHTTPClient(30*time.Second))
		if refreshErr == nil {
			if err := s.persistRefreshedCredentials(conn, refreshResult); err != nil {
				slog.Error("Failed to persist refreshed Antigravity credentials", "error", err)
				return true
			}
			token = conn.Data.AccessToken
			retryReq, retryErr := http.NewRequestWithContext(r.Context(), "POST", upstreamURL, bytes.NewReader(envelopeBytes))
			if retryErr == nil {
				retryReq.Header.Set("Authorization", "Bearer "+token)
				retryReq.Header.Set("Content-Type", "application/json")
				retryReq.Header.Set("Accept", "text/event-stream")
				retryReq.Header.Set("User-Agent", provider.AntigravityUserAgent)
				if newResp, newErr := client.Do(retryReq); newErr == nil {
					resp = newResp
				}
			}
		}
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		provider.ApplyErrorState(conn, resp.StatusCode, string(respBody))
		if s.DB != nil {
			_ = s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
		}
		latencyMs := time.Since(start).Milliseconds()
		if latencyMs < 0 {
			latencyMs = 0
		}
		statusStr := fmt.Sprintf("%d", resp.StatusCode)
		rdID := fmt.Sprintf("%d-%s", time.Now().UnixNano(), modelInfo.Model)
		rdData := map[string]any{
			"id":           rdID,
			"timestamp":    time.Now().UTC().Format(time.RFC3339),
			"provider":     conn.Provider,
			"model":        modelInfo.Model,
			"connectionId": conn.ID,
			"status":       statusStr,
			"latencyMs":    latencyMs,
			"endpoint":     endpoint,
			"error":        string(respBody),
			"input":        extractPromptSummary(req.Messages),
		}
		rdBytes, _ := json.Marshal(rdData)
		if s.DB != nil {
			_ = s.DB.SaveRequestDetail(&db.RequestDetail{
				ID:           rdID,
				Timestamp:    rdData["timestamp"].(string),
				Provider:     conn.Provider,
				Model:        modelInfo.Model,
				ConnectionID: conn.ID,
				Status:       statusStr,
				Data:         string(rdBytes),
			})
		}
		if s.Metrics != nil {
			s.Metrics.ObserveRequest(conn.Provider, modelInfo.Model, endpoint, resp.StatusCode, time.Since(start).Seconds(), nil)
		}
		s.publishRequestEvent(r, RequestEvent{Type: "request", Provider: conn.Provider, Model: modelInfo.Model, Endpoint: endpoint, Status: statusStr, LatencyMs: latencyMs})

		// The account itself is out of quota or throttled: another account in the
		// pool may still serve this request, so let the caller retry before the
		// client sees the failure.
		if provider.CheckFallbackError(resp.StatusCode, string(respBody), conn.Data.BackoffLevel).ShouldFallback {
			return true
		}
		writeJSON(w, resp.StatusCode, map[string]any{
			"error": fmt.Sprintf("antigravity upstream error (HTTP %d): %s", resp.StatusCode, string(respBody)),
		})
		return false
	}

	uc := &usageContext{
		StartedAt:    start,
		Provider:     conn.Provider,
		Model:        modelInfo.Model,
		ConnectionID: conn.ID,
		APIKey:       extractRequestAPIKey(r),
		Endpoint:     endpoint,
		Status:       http.StatusOK,
		Prompt:       extractPromptSummary(req.Messages),
	}

	if isImage || !req.Stream {
		s.proxyAntigravityNonStreaming(w, resp, modelInfo.Model, uc)
	} else {
		s.proxyAntigravityStreaming(w, r, resp, modelInfo.Model, uc)
	}
	return false
}

// defaultThinkingAgSignature is the base64-encoded dummy thought signature ("skip_thought_signature_validator")
// officially recognized by Google Gemini and Vertex AI to bypass thought signature validation for prior turns.
const defaultThinkingAgSignature = "c2tpcF90aG91Z2h0X3NpZ25hdHVyZV92YWxpZGF0b3I="

func parseMessageContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return str
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "text" || p.Text != "" {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return string(raw)
}

func convertOpenAIToGeminiContents(messages []Message, targetModel ...string) ([]map[string]any, []map[string]any) {
	var contents []map[string]any
	var systemParts []map[string]any
	isGemini4 := len(targetModel) > 0 && isGemini4Model(targetModel[0])
	toolCallNames := make(map[string]string)
	for _, msg := range messages {
		if len(msg.ToolCalls) > 0 {
			var tcs []struct {
				ID       string `json:"id"`
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			}
			if err := json.Unmarshal(msg.ToolCalls, &tcs); err == nil {
				for _, tc := range tcs {
					if tc.ID != "" && tc.Function.Name != "" {
						toolCallNames[tc.ID] = tc.Function.Name
					}
				}
			}
		}
	}

	for _, msg := range messages {
		role := strings.ToLower(msg.Role)
		text := parseMessageContent(msg.Content)

		if role == "system" || role == "developer" {
			systemParts = append(systemParts, map[string]any{"text": text})
			continue
		}
		if role == "tool" {
			fnName := toolCallNames[msg.ToolCallID]
			if fnName == "" {
				fnName = msg.ToolCallID
			}
			var resp any
			if err := json.Unmarshal([]byte(text), &resp); err != nil || resp == nil {
				resp = map[string]any{"result": text}
			}
			respMap, ok := resp.(map[string]any)
			if !ok {
				respMap = map[string]any{"result": resp}
			}
			contents = append(contents, map[string]any{
				"role": "user",
				"parts": []map[string]any{
					{
						"functionResponse": map[string]any{
							"name":     fnName,
							"response": respMap,
						},
					},
				},
			})
			continue
		}
		if role == "assistant" {
			role = "model"
		}

		parts := []map[string]any{
			{"text": text},
		}

		// Backfill thoughtSignature for tool calling history on Gemini 3+
		if len(msg.ToolCalls) > 0 {
			var toolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			}
			if err := json.Unmarshal(msg.ToolCalls, &toolCalls); err == nil && len(toolCalls) > 0 {
				for i, tc := range toolCalls {
					var args map[string]any
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
					if args == nil {
						args = map[string]any{}
					}
					fcPart := map[string]any{
						"functionCall": map[string]any{
							"name": tc.Function.Name,
							"args": args,
						},
					}
					if i == 0 {
						fcPart["thoughtSignature"] = defaultThinkingAgSignature
					}
					parts = append(parts, fcPart)
				}
			}
		}

		contents = append(contents, map[string]any{
			"role":  role,
			"parts": parts,
		})
	}

	// Normalization: merge adjacent same-role turns and ensure functionResponse is placed last in user turns for Gemini 4
	contents = normalizeAntigravityContents(contents, isGemini4)

	return contents, systemParts
}

// isGemini4Model reports whether a model name refers to Google Gemini 4 (Argon architecture).
func isGemini4Model(model string) bool {
	m := strings.ToLower(model)
	if slash := strings.LastIndex(m, "/"); slash >= 0 {
		m = m[slash+1:]
	}
	return strings.HasPrefix(m, "gemini-4") || strings.Contains(m, "/gemini-4")
}

// normalizeAntigravityContents merges adjacent turns of the same role and places functionResponse last for Gemini 4
func normalizeAntigravityContents(contents []map[string]any, isGemini4 bool) []map[string]any {
	if len(contents) == 0 {
		return contents
	}
	var merged []map[string]any
	for _, c := range contents {
		role, _ := c["role"].(string)
		parts, _ := c["parts"].([]map[string]any)
		if len(parts) == 0 {
			continue
		}
		if len(merged) > 0 && merged[len(merged)-1]["role"] == role {
			lastParts, _ := merged[len(merged)-1]["parts"].([]map[string]any)
			lastParts = append(lastParts, parts...)
			if isGemini4 && role == "user" {
				lastParts = moveFunctionResponsesLast(lastParts)
			}
			merged[len(merged)-1]["parts"] = lastParts
			continue
		}
		if isGemini4 && role == "user" {
			parts = moveFunctionResponsesLast(parts)
		}
		merged = append(merged, map[string]any{
			"role":  role,
			"parts": parts,
		})
	}
	return merged
}

func moveFunctionResponsesLast(parts []map[string]any) []map[string]any {
	var nonFR []map[string]any
	var fr []map[string]any
	for _, p := range parts {
		if _, ok := p["functionResponse"]; ok {
			fr = append(fr, p)
		} else {
			nonFR = append(nonFR, p)
		}
	}
	if len(fr) == 0 {
		return parts
	}
	return append(nonFR, fr...)
}

type antigravityCandidatePart struct {
	Text         string `json:"text"`
	Thought      bool   `json:"thought"`
	FunctionCall *struct {
		Name string         `json:"name"`
		Args map[string]any `json:"args"`
	} `json:"functionCall,omitempty"`
}

type antigravityCandidateItem struct {
	Content struct {
		Parts []antigravityCandidatePart `json:"parts"`
	} `json:"content"`
	FinishReason string `json:"finishReason"`
}

type antigravityChunkPayload struct {
	Response struct {
		Candidates []antigravityCandidateItem `json:"candidates"`
	} `json:"response"`
	Candidates []antigravityCandidateItem `json:"candidates"`
}

func (p *antigravityChunkPayload) getCandidates() []antigravityCandidateItem {
	if len(p.Response.Candidates) > 0 {
		return p.Response.Candidates
	}
	return p.Candidates
}

// Antigravity may wrap Gemini's response, or put usageMetadata at the root.
func extractAntigravityUsage(data []byte) usage.Usage {
	// Detect whether usageMetadata is actually present (not absent/null).
	rootHas := hasNonNullField(data, "usageMetadata")

	var envelopeHas bool
	var responseRaw json.RawMessage
	var env struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(data, &env); err == nil && len(env.Response) > 0 && string(env.Response) != "null" {
		responseRaw = env.Response
		envelopeHas = hasNonNullField(responseRaw, "usageMetadata")
	}

	var u usage.Usage
	if envelopeHas {
		u = usage.ExtractFromGemini(responseRaw)
	} else if rootHas {
		u = usage.ExtractFromGemini(data)
	}
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	if envelopeHas || rootHas {
		u.Known = true
	}
	return u
}

// hasNonNullField reports whether obj contains key with a non-null value.
func hasNonNullField(obj json.RawMessage, key string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(obj, &m); err != nil {
		return false
	}
	raw, ok := m[key]
	return ok && len(raw) > 0 && string(raw) != "null" && string(raw) != "{}"
}

func (s *Server) proxyAntigravityStreaming(w http.ResponseWriter, r *http.Request, resp *http.Response, model string, uc *usageContext) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Cyrene-Served-Model", model)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	stallReader := provider.NewStallTimeoutReader(resp.Body, provider.DefaultStreamStallTimeout)
	defer stallReader.Close()
	scanner := bufio.NewScanner(stallReader)
	scanner.Buffer(make([]byte, 1024*1024), 5*1024*1024)

	chatCmpleID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixMilli())
	var lastUsage usage.Usage
	var outBuilder strings.Builder
	var reasoningBuilder strings.Builder
	ctx := r.Context()
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			slog.Info("Client disconnected during Antigravity stream", slog.String("model", model))
			finalOutput := outBuilder.String()
			if finalOutput == "" && reasoningBuilder.Len() > 0 {
				finalOutput = reasoningBuilder.String()
			}
			if uc != nil {
				uc.Status = 499
				uc.Response = finalOutput
			}
			s.recordUsage(uc, lastUsage)
			return
		default:
		}
		line := scanner.Bytes()
		if isSSEComment(line) {
			if err := forwardSSEComment(w, flusher, line); err != nil {
				return
			}
			continue
		}
		data, isDone, ok := translator.ParseSSEDataLineString(scanner.Text())
		if !ok || isDone {
			continue
		}

		var payload antigravityChunkPayload
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}
		if u := extractAntigravityUsage([]byte(data)); u.Known || u.TotalTokens > 0 {
			lastUsage = u
		}

		textChunk := ""
		reasoningChunk := ""
		finishReason := ""
		var toolCallsDelta []any
		for _, cand := range payload.getCandidates() {
			if cand.FinishReason != "" {
				finishReason = strings.ToLower(cand.FinishReason)
			}
			for i, part := range cand.Content.Parts {
				if part.Thought {
					reasoningChunk += part.Text
				} else if part.FunctionCall != nil {
					argsBytes, _ := json.Marshal(part.FunctionCall.Args)
					toolCallsDelta = append(toolCallsDelta, map[string]any{
						"index": i,
						"id":    fmt.Sprintf("call_%s_%d", randomHex(8), i),
						"type":  "function",
						"function": map[string]any{
							"name":      part.FunctionCall.Name,
							"arguments": string(argsBytes),
						},
					})
				} else {
					textChunk += part.Text
				}
			}
		}

		if textChunk == "" && reasoningChunk == "" && len(toolCallsDelta) == 0 && finishReason == "" {
			continue
		}

		delta := map[string]any{}
		if textChunk != "" {
			delta["content"] = textChunk
			if outBuilder.Len() < 16384 {
				outBuilder.WriteString(textChunk)
			}
		}
		if reasoningChunk != "" {
			delta["reasoning_content"] = reasoningChunk
			if reasoningBuilder.Len() < 16384 {
				reasoningBuilder.WriteString(reasoningChunk)
			}
		}
		if len(toolCallsDelta) > 0 {
			delta["tool_calls"] = toolCallsDelta
		}

		var fr any = nil
		if finishReason != "" {
			if len(toolCallsDelta) > 0 || finishReason == "tool_calls" {
				fr = "tool_calls"
			} else if finishReason == "stop" || finishReason == "max_tokens" {
				fr = finishReason
			} else {
				fr = "stop"
			}
		} else if len(toolCallsDelta) > 0 {
			fr = "tool_calls"
		}
		chunk := map[string]any{
			"id":      chatCmpleID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{
				{
					"index":         0,
					"delta":         delta,
					"finish_reason": fr,
				},
			},
		}

		chunkBytes, _ := json.Marshal(chunk)
		if _, writeErr := fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes)); writeErr != nil {
			finalOutput := outBuilder.String()
			if finalOutput == "" && reasoningBuilder.Len() > 0 {
				finalOutput = reasoningBuilder.String()
			}
			if uc != nil {
				uc.Status = 499
				uc.Response = finalOutput
			}
			s.recordUsage(uc, lastUsage)
			return
		}
	}

	if lastUsage.Known || lastUsage.TotalTokens > 0 {
		chunk, _ := json.Marshal(map[string]any{
			"id": chatCmpleID, "object": "chat.completion.chunk", "created": time.Now().Unix(),
			"model": model, "choices": []any{}, "usage": lastUsage.OpenAI(),
		})
		fmt.Fprintf(w, "data: %s\n\n", chunk)
	}
	// Send final [DONE]
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	finalOutput := outBuilder.String()
	if finalOutput == "" && reasoningBuilder.Len() > 0 {
		finalOutput = reasoningBuilder.String()
	}
	if uc != nil {
		uc.Response = finalOutput
	}
	s.recordUsage(uc, lastUsage)
}

func (s *Server) proxyAntigravityNonStreaming(w http.ResponseWriter, resp *http.Response, model string, uc *usageContext) {
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to read antigravity response: " + err.Error()})
		return
	}

	fullText := ""
	reasoningText := ""
	var toolCalls []any
	var lastUsage usage.Usage

	extractParts := func(data []byte) {
		var payload antigravityChunkPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return
		}
		if u := extractAntigravityUsage(data); u.Known || u.TotalTokens > 0 {
			lastUsage = u
		}
		for _, cand := range payload.getCandidates() {
			for i, part := range cand.Content.Parts {
				if part.Thought {
					reasoningText += part.Text
				} else if part.FunctionCall != nil {
					argsBytes, _ := json.Marshal(part.FunctionCall.Args)
					toolCalls = append(toolCalls, map[string]any{
						"id":   fmt.Sprintf("call_%s_%d", randomHex(8), i),
						"type": "function",
						"function": map[string]any{
							"name":      part.FunctionCall.Name,
							"arguments": string(argsBytes),
						},
					})
				} else {
					fullText += part.Text
				}
			}
		}
	}

	trimmed := bytes.TrimSpace(respBytes)
	if bytes.HasPrefix(trimmed, []byte("{")) {
		extractParts(trimmed)
	} else {
		scanner := bufio.NewScanner(bytes.NewReader(respBytes))
		scanner.Buffer(make([]byte, 1024*1024), 5*1024*1024)
		for scanner.Scan() {
			data, isDone, ok := translator.ParseSSEDataLineString(scanner.Text())
			if !ok || isDone {
				continue
			}
			extractParts([]byte(data))
		}
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	respObj := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixMilli()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": func() map[string]any {
					m := map[string]any{
						"role":    "assistant",
						"content": fullText,
					}
					if reasoningText != "" {
						m["reasoning_content"] = reasoningText
					}
					if len(toolCalls) > 0 {
						m["tool_calls"] = toolCalls
					}
					return m
				}(),
				"finish_reason": finishReason,
			},
		},
	}

	if lastUsage.Known || lastUsage.TotalTokens > 0 {
		respObj["usage"] = lastUsage.OpenAI()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cyrene-Served-Model", model)
	json.NewEncoder(w).Encode(respObj)
	finalResp := fullText
	if finalResp == "" && reasoningText != "" {
		finalResp = reasoningText
	}
	if uc != nil {
		uc.Response = finalResp
	}
	s.recordUsage(uc, lastUsage)
}
func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func (s *Server) getAntigravityAvailableModels() []model.ModelMetadata {
	if s.DB == nil {
		return nil
	}
	if raw, err := s.DB.KVGet("providerModelCache", "antigravity"); err == nil && raw != "" {
		var cached model.CachedModels
		if err := json.Unmarshal([]byte(raw), &cached); err == nil && len(cached.Models) > 0 {
			return cached.Models
		}
	}
	return nil
}

// resolveAntigravityModel dynamically resolves the target upstream model ID and tier from available models.
func resolveAntigravityModel(
	rawModel string,
	reasoningEffort string,
	availableModels []model.ModelMetadata,
) (targetModel string, tier string, shouldInjectThinking bool, isImage bool) {
	cleanModel := rawModel
	cleanModel = strings.TrimPrefix(cleanModel, "antigravity/")
	cleanModel = strings.TrimPrefix(cleanModel, "ag/")
	cleanModel = strings.TrimSpace(cleanModel)

	if strings.Contains(strings.ToLower(cleanModel), "image") {
		return cleanModel, "", false, true
	}

	lowerEffort := strings.ToLower(strings.TrimSpace(reasoningEffort))
	switch lowerEffort {
	case "high", "medium", "low", "extra-low":
		tier = lowerEffort
	}

	for _, t := range []string{"high", "medium", "low", "extra-low"} {
		if strings.HasSuffix(cleanModel, "-"+t) {
			tier = t
			break
		}
	}
	if tier == "" {
		tier = "medium"
	}
	availMap := make(map[string]model.ModelMetadata, len(availableModels))
	for _, m := range availableModels {
		availMap[m.ID] = m
	}

	// 1. Exact match
	if _, ok := availMap[cleanModel]; ok {
		targetModel = cleanModel
	}

	// 2. Effort suffix probing if not resolved yet
	if targetModel == "" {
		effectiveTier := tier
		if effectiveTier == "" {
			effectiveTier = "medium"
		}

		if cleanModel == "gemini-3.5-flash" {
			if effectiveTier == "high" {
				targetModel = "gemini-3.5-flash-high"
			} else if effectiveTier == "low" {
				targetModel = "gemini-3.5-flash-extra-low"
			} else {
				targetModel = "gemini-3.5-flash-low"
			}
		} else if cleanModel == "gemini-3.1-pro" {
			if effectiveTier == "low" {
				targetModel = "gemini-3.1-pro-low"
			} else {
				targetModel = "gemini-3.1-pro-high"
			}
		} else {
			probe := cleanModel + "-" + effectiveTier
			if _, ok := availMap[probe]; ok {
				targetModel = probe
			} else {
				for _, alt := range []string{"medium", "high", "low"} {
					probeAlt := cleanModel + "-" + alt
					if _, ok := availMap[probeAlt]; ok {
						targetModel = probeAlt
						if tier == "" {
							tier = alt
						}
						break
					}
				}
			}
		}
	}

	// 3. Prefix matching in available models
	if targetModel == "" {
		for id := range availMap {
			if strings.HasPrefix(id, cleanModel+"-") {
				targetModel = id
				break
			}
		}
	}

	// 4. Default / Passthrough fallback
	if targetModel == "" {
		if tier != "" && !strings.HasSuffix(cleanModel, "-"+tier) && strings.HasPrefix(cleanModel, "gemini-") {
			targetModel = cleanModel + "-" + tier
		} else {
			targetModel = cleanModel
		}
	}

	if tier == "" {
		for _, t := range []string{"high", "medium", "low", "extra-low"} {
			if strings.HasSuffix(targetModel, "-"+t) {
				tier = t
				break
			}
		}
	}
	if tier == "" {
		tier = "medium"
	}

	// Determine if thinkingConfig should be injected:
	// Only Gemini family models with thinking capability should receive thinkingConfig.
	// Claude, GPT-OSS, and non-thinking models must NEVER receive thinkingConfig.
	isGemini := strings.HasPrefix(targetModel, "gemini-")
	if meta, ok := availMap[targetModel]; ok {
		if meta.Family != "" && meta.Family != "gemini" {
			isGemini = false
		}
		hasReasoning := false
		for _, cap := range meta.Capabilities {
			if cap == "reasoning" {
				hasReasoning = true
				break
			}
		}
		shouldInjectThinking = isGemini && hasReasoning && (tier == "high" || tier == "medium" || tier == "low")
	} else {
		// Non-cached fallback heuristic: only gemini- models get thinkingConfig
		shouldInjectThinking = isGemini && (tier == "high" || tier == "medium" || tier == "low")
	}

	return targetModel, tier, shouldInjectThinking, false
}

// resolveAntigravityModelTier maps requested model and reasoning_effort to upstream tiered model (compat).
func resolveAntigravityModelTier(rawModel, reasoningEffort string) (targetModel string, tier string) {
	targetModel, tier, _, _ = resolveAntigravityModel(rawModel, reasoningEffort, nil)
	return targetModel, tier
}
