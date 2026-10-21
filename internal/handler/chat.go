package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/cache"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
	"github.com/arisvia/cyrene-gateway/internal/rtk"
	"github.com/arisvia/cyrene-gateway/internal/translator"
	"github.com/arisvia/cyrene-gateway/internal/usage"
)

func isHopByHopHeader(h string) bool {
	h = strings.ToLower(h)
	switch h {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
		"te", "trailers", "transfer-encoding", "upgrade":
		return true
	}
	return false
}

// usageContext carries metadata for recording usage after a proxied response.
type usageContext struct {
	RequestID    string
	StartedAt    time.Time
	Provider     string
	Model        string
	ConnectionID string
	APIKey       string
	Endpoint     string
	Status       int
	Prompt       string
	Response     string
}

func capPromptText(s string) string {
	const maxPromptChars = 16384
	if len(s) > maxPromptChars {
		return s[:maxPromptChars] + "..."
	}
	return s
}

func extractPromptSummary(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.ToLower(messages[i].Role) == "user" {
			txt := parseMessageContent(messages[i].Content)
			if txt != "" {
				return capPromptText(txt)
			}
		}
	}
	return capPromptText(parseMessageContent(messages[len(messages)-1].Content))
}

func extractAnthropicPrompt(reqBody map[string]any) string {
	rawMsgs, ok := reqBody["messages"].([]any)
	if !ok || len(rawMsgs) == 0 {
		return ""
	}
	extractText := func(content any) string {
		switch c := content.(type) {
		case string:
			return c
		case []any:
			var sb strings.Builder
			for _, item := range c {
				if block, ok := item.(map[string]any); ok {
					if t, ok := block["text"].(string); ok && t != "" {
						if sb.Len() > 0 {
							sb.WriteString("\n")
						}
						sb.WriteString(t)
					}
				}
			}
			return sb.String()
		}
		return ""
	}
	for i := len(rawMsgs) - 1; i >= 0; i-- {
		msg, ok := rawMsgs[i].(map[string]any)
		if !ok {
			continue
		}
		if role, _ := msg["role"].(string); strings.ToLower(role) == "user" {
			txt := extractText(msg["content"])
			if txt != "" {
				return capPromptText(txt)
			}
		}
	}
	if last, ok := rawMsgs[len(rawMsgs)-1].(map[string]any); ok {
		return capPromptText(extractText(last["content"]))
	}
	return ""
}

// ChatCompletionRequest represents an OpenAI-compatible chat request
type ChatCompletionRequest struct {
	Temperature     *float64        `json:"temperature,omitempty"`
	MaxTokens       *int            `json:"max_tokens,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
	Model           string          `json:"model"`
	Messages        []Message       `json:"messages"`
	Tools           json.RawMessage `json:"tools,omitempty"`
	ToolChoice      json.RawMessage `json:"tool_choice,omitempty"`
	Stream          bool            `json:"stream,omitempty"`
}

type Message struct {
	Role       string          `json:"role"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	rawBody, err := readRequestBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}

	var req ChatCompletionRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if req.Model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing model"})
		return
	}
	requestID := ensureRequestID(r)
	w.Header().Set(requestIDHeader, requestID)
	s.publishRequestEvent(r, RequestEvent{Type: "accepted", Model: req.Model, Status: "accepted"})

	// Strip reasoning effort suffix from model name (e.g. "claude-3-7-sonnet(high)" -> effort="high")
	if cleanModel, effort := provider.ExtractModelReasoningEffort(req.Model); effort != "" {
		req.Model = cleanModel
		if req.ReasoningEffort == "" {
			req.ReasoningEffort = effort
		}
		var bodyMap map[string]any
		if err := json.Unmarshal(rawBody, &bodyMap); err == nil {
			bodyMap["model"] = cleanModel
			if _, hasEffort := bodyMap["reasoning_effort"]; !hasEffort {
				bodyMap["reasoning_effort"] = effort
			}
			if updated, err := json.Marshal(bodyMap); err == nil {
				rawBody = updated
			}
		}
	}
	keyObj := auth.APIKeyFromContext(r.Context())
	if keyObj != nil && !keyObj.IsModelAllowed(req.Model) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": fmt.Sprintf("model '%s' is not allowed for this API key", req.Model),
		})
		return
	}

	// Check if model is disabled
	if provider.IsModelDisabled(req.Model, s.DB) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": fmt.Sprintf("model is disabled: %s", req.Model)})
		return
	}

	// Inject per-key system prompt context pre-cache
	if keyObj != nil && keyObj.SystemPrompt != "" {
		var bodyMap map[string]any
		if err := json.Unmarshal(rawBody, &bodyMap); err == nil {
			rtk.InjectSystemPrompt(bodyMap, "openai", keyObj.SystemPrompt)
			if updated, err := json.Marshal(bodyMap); err == nil {
				rawBody = updated
				_ = json.Unmarshal(rawBody, &req)
			}
		}
	}

	// Phase: Response Cache check
	var settings *db.Settings
	if s.DB != nil {
		settings, _ = s.DB.GetSettings()
	}
	cacheEnabled := settings != nil && settings.ResponseCacheEnabled && s.Cache != nil
	var cacheKey string
	if cacheEnabled && !cache.ShouldBypass(r.Header) {
		eligible, key, err := cache.IsEligible("chat", rawBody, settings.ResponseCacheAll)
		if err == nil && eligible {
			cacheKey = key
			if entry, hit := s.Cache.Get(key); hit {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Cyrene-Cache", "HIT")
				if entry.ServedModel != "" {
					w.Header().Set("X-Cyrene-Served-Model", entry.ServedModel)
				}
				w.WriteHeader(entry.StatusCode)
				w.Write(entry.Body)
				return
			}
		}
	}

	if cacheKey != "" {
		w.Header().Set("X-Cyrene-Cache", "MISS")
		rec := cache.NewRecorder(w)
		defer func() {
			if r := recover(); r != nil {
				panic(r)
			}
			if rec.ShouldCache() {
				ttl := time.Duration(settings.ResponseCacheTTL) * time.Second
				if ttl <= 0 {
					ttl = time.Hour
				}
				servedModel := rec.Header().Get("X-Cyrene-Served-Model")
				tokens := rec.ExtractTokens()
				s.Cache.Set(cacheKey, rec.StatusCode(), rec.HeaderMap(), rec.BodyBytes(), ttl, servedModel, tokens)
			}
		}()
		w = rec
	}

	// Check if model string is a combo
	if combo, ok := provider.ResolveCombo(req.Model, s.DB); ok {
		s.handleComboChat(w, r, req, rawBody, combo)
		return
	}

	// Single model path
	s.handleSingleModelChat(w, r, req, rawBody)
}

// getHTTPClient returns an HTTP client, using proxy rotation if available.
func (s *Server) getHTTPClient(timeout time.Duration) *http.Client {
	allowPrivate := false
	if s.Config != nil && s.Config.AllowPrivateNetworks {
		allowPrivate = true
	}
	if s.Proxies != nil {
		client := s.Proxies.GetHTTPClient(allowPrivate)
		client.Timeout = timeout
		return client
	}
	return provider.SafeHTTPClient(timeout, allowPrivate)
}

// handleSingleModelChat processes a single model request (non-combo).
func (s *Server) handleSingleModelChat(w http.ResponseWriter, r *http.Request, req ChatCompletionRequest, rawBody []byte) {
	// Resolve model to provider + model name
	modelInfo, err := provider.ResolveModel(req.Model, s.DB)
	if err != nil || modelInfo.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("cannot resolve model: %s", req.Model)})
		return
	}

	// Get provider info
	providerInfo, ok := provider.GetProvider(modelInfo.Provider)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider: %s", modelInfo.Provider)})
		return
	}

	// Get credentials for this provider
	conns, err := s.DB.ListConnectionsByProvider(modelInfo.Provider)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("failed to list credentials for provider: %s", modelInfo.Provider),
		})
		return
	}

	if len(conns) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("no active credentials for provider: %s", modelInfo.Provider),
		})
		return
	}

	// Select best available connection (or respect specific connection if requested)
	var conn *model.ProviderConnection
	if targetConnID := r.Header.Get("X-Cyrene-Connection-ID"); targetConnID != "" {
		for i := range conns {
			if conns[i].ID == targetConnID && conns[i].IsActive {
				conn = &conns[i]
				break
			}
		}
	}
	if conn == nil {
		conn = s.selectAvailableConnection(conns, modelInfo.Model, nil)
	}
	if conn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("all accounts rate-limited for provider: %s", modelInfo.Provider),
		})
		return
	}

	// Phase 9: Pre-check OAuth token refresh
	s.tryRefreshToken(conn)

	// --- Special protocol paths: Qoder (COSY) & Antigravity (Cloud Code) ---
	if modelInfo.Provider == "qoder" || modelInfo.Provider == "qoder-cn" {
		s.handleQoderChat(w, r, req, rawBody, modelInfo, conn, providerInfo)
		return
	}
	if modelInfo.Provider == "antigravity" {
		s.handleAntigravityChat(w, r, req, rawBody, modelInfo, conn, providerInfo)
		return
	}

	upstreamStream := req.Stream || providerInfo.ForceStream
	loopGuard := s.loopGuardEnabled()
	upstreamReq, targetFormat, err := provider.PrepareUpstreamRequest(r.Context(),
		provider.ExecutionRequest{RawBody: rawBody, Stream: req.Stream, HasTools: req.Tools != nil, LoopGuardEnabled: loopGuard, AutoPromptCache: s.autoPromptCacheEnabled()},
		provider.ExecutionCandidate{Connection: conn, ModelInfo: modelInfo, ProviderInfo: providerInfo},
		func(body map[string]any, format string) { s.applyTokenSaver(body, format, modelInfo.Provider, r) },
	)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	slog.Info("Proxying request",
		slog.String("model", modelInfo.Model),
		slog.String("provider", modelInfo.Provider),
		slog.String("format", string(targetFormat)),
		slog.String("connection", conn.ID),
		slog.Bool("stream", req.Stream),
	)

	endpoint := resolveRequestEndpoint(r, "/v1/chat/completions")
	s.publishRequestEvent(r, RequestEvent{Type: "routing", Provider: modelInfo.Provider, Model: modelInfo.Model, Endpoint: endpoint, Status: "routing"})

	// Phase 9: Use proxy-aware HTTP client
	start := time.Now()
	clientTimeout := 5 * time.Minute
	if upstreamStream {
		clientTimeout = 0
	}
	client := s.getHTTPClient(clientTimeout)
	resp, err := client.Do(upstreamReq)
	if err != nil {
		slog.Error("Upstream request failed", "error", err, "provider", modelInfo.Provider)
		latencyMs := time.Since(start).Milliseconds()
		s.publishRequestEvent(r, RequestEvent{Type: "failed", Provider: modelInfo.Provider, Model: modelInfo.Model, Status: "502", Endpoint: endpoint, LatencyMs: latencyMs})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream request failed"})
		return
	}
	defer resp.Body.Close()

	// Phase 9: On-401 retry with token refresh
	if resp.StatusCode == http.StatusUnauthorized && conn.Data.RefreshToken != "" {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(errBody))

		result, refreshErr := provider.RefreshCredentials(conn.Provider, conn, s.getHTTPClient(30*time.Second))
		if refreshErr == nil {
			if errUpdate := s.persistRefreshedCredentials(conn, result); errUpdate != nil {
				slog.Error("Failed to persist connection after 401 refresh", "provider", conn.Provider, "error", errUpdate)
			}

			retryReq, _, retryErr := provider.PrepareUpstreamRequest(r.Context(),
				provider.ExecutionRequest{RawBody: rawBody, Stream: req.Stream, HasTools: req.Tools != nil, LoopGuardEnabled: loopGuard, AutoPromptCache: s.autoPromptCacheEnabled()},
				provider.ExecutionCandidate{Connection: conn, ModelInfo: modelInfo, ProviderInfo: providerInfo},
				func(body map[string]any, format string) { s.applyTokenSaver(body, format, modelInfo.Provider, r) },
			)
			if retryErr == nil {
				slog.Info("Retrying after token refresh", slog.String("provider", modelInfo.Provider))
				resp, err = client.Do(retryReq)
				if err != nil {
					latencyMs := time.Since(start).Milliseconds()
					s.publishRequestEvent(r, RequestEvent{Type: "failed", Provider: modelInfo.Provider, Model: modelInfo.Model, Status: "502", Endpoint: endpoint, LatencyMs: latencyMs})
					writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream request failed after refresh"})
					return
				}
				defer resp.Body.Close()
			}
		}
	}

	// Handle upstream errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		slog.Warn("Upstream request returned error status",
			slog.String("provider", modelInfo.Provider),
			slog.String("model", modelInfo.Model),
			slog.Int("status", resp.StatusCode),
			slog.String("body", string(errBody)),
		)
		provider.ApplyErrorState(conn, resp.StatusCode, string(errBody))
		s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
		// Record failed request detail for observability in "请求明细"
		latencyMs := int(time.Since(start).Milliseconds())
		if latencyMs < 0 {
			latencyMs = 0
		}
		statusStr := fmt.Sprintf("%d", resp.StatusCode)
		rdID := fmt.Sprintf("%d-%s", time.Now().UnixNano(), modelInfo.Model)
		rdData := map[string]any{
			"id":           rdID,
			"timestamp":    time.Now().UTC().Format(time.RFC3339),
			"provider":     modelInfo.Provider,
			"model":        modelInfo.Model,
			"connectionId": conn.ID,
			"status":       statusStr,
			"latencyMs":    latencyMs,
			"endpoint":     endpoint,
			"error":        string(errBody),
			"input":        extractPromptSummary(req.Messages),
		}
		rdBytes, _ := json.Marshal(rdData)
		_ = s.DB.SaveRequestDetail(&db.RequestDetail{
			ID:           rdID,
			Timestamp:    rdData["timestamp"].(string),
			Provider:     modelInfo.Provider,
			Model:        modelInfo.Model,
			ConnectionID: conn.ID,
			Status:       statusStr,
			Data:         string(rdBytes),
		})
		if s.Metrics != nil {
			s.Metrics.ObserveRequest(modelInfo.Provider, modelInfo.Model, endpoint, resp.StatusCode, time.Since(start).Seconds(), nil)
		}
		s.publishRequestEvent(r, RequestEvent{Type: "request", Provider: modelInfo.Provider, Model: modelInfo.Model, Status: statusStr, Endpoint: endpoint, LatencyMs: int64(latencyMs)})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		// Wrap error with actionable hint (Phase 26 Error UX)
		hinted := wrapErrorWithHint(resp.StatusCode, errBody, modelInfo.Provider)
		w.Write(hinted)
		return
	}

	// Reset error state on success
	provider.ResetAccountState(conn)
	s.DB.UpdateConnectionHealth(conn.ID, conn.Data)

	// Response headers arrived; the selected upstream is reachable.
	s.publishRequestEvent(r, RequestEvent{Type: "upstream_connected", Provider: modelInfo.Provider, Model: modelInfo.Model, Status: "upstream_connected", Endpoint: endpoint})

	// Proxy the response with format translation
	uc := &usageContext{
		RequestID:    ensureRequestID(r),
		Provider:     modelInfo.Provider,
		Model:        modelInfo.Model,
		ConnectionID: conn.ID,
		APIKey:       extractRequestAPIKey(r),
		Endpoint:     endpoint,
		StartedAt:    start,
		Status:       resp.StatusCode,
		Prompt:       extractPromptSummary(req.Messages),
	}
	s.proxyResponse(w, r, resp, req.Stream, upstreamStream, targetFormat, modelInfo.Model, uc)
}

// proxyResponse handles streaming and non-streaming response proxying with format translation.
func (s *Server) proxyResponse(w http.ResponseWriter, r *http.Request, resp *http.Response, clientStream, upstreamStream bool, format translator.Format, model string, uc *usageContext) {
	isSSE := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
	if isSSE && !clientStream {
		s.aggregateSSEToNonStreaming(w, r, resp, format, model, uc)
		return
	}
	if !clientStream {
		s.proxyNonStreaming(w, resp, format, model, uc)
		return
	}
	s.proxyStreaming(w, r, resp, format, model, uc)
}

type chatStreamChunk struct {
	ID      string          `json:"id"`
	Created int64           `json:"created"`
	Error   json.RawMessage `json:"error"`
	Choices []struct {
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
		Delta        struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

func (s *Server) aggregateSSEToNonStreaming(w http.ResponseWriter, r *http.Request, resp *http.Response, format translator.Format, model string, uc *usageContext) {
	stallReader := provider.NewStallTimeoutReader(resp.Body, provider.DefaultStreamStallTimeout)
	defer stallReader.Close()
	reader := provider.NewSSEReader(stallReader)
	streamTranslator := translator.NewSSETranslator(format, model)
	var lastUsage usage.Usage
	var chunkID string
	created := time.Now().Unix()
	type toolState struct {
		id, kind        string
		name, arguments strings.Builder
	}
	type choiceState struct {
		content, reasoning strings.Builder
		finishReason       string
		tools              map[int]*toolState
	}
	choices := make(map[int]*choiceState)
	ctx := r.Context()
	streamingPublished := false

	for {
		event, err := reader.ReadEvent(ctx)
		if err != nil {
			if ctx.Err() != nil {
				if uc != nil {
					s.publishRequestEvent(r, RequestEvent{RequestID: uc.RequestID, Type: "cancelled", Provider: uc.Provider, Model: uc.Model, Status: "cancelled", Endpoint: uc.Endpoint})
					uc.Status = 499
				}
				s.recordUsage(uc, lastUsage)
				return
			}
			complete := len(choices) > 0
			for _, choice := range choices {
				complete = complete && choice.finishReason != ""
			}
			if !errors.Is(err, io.EOF) || !complete {
				if uc != nil {
					uc.Status = http.StatusBadGateway
				}
				s.recordUsage(uc, lastUsage)
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream stream ended before completion"})
				return
			}
			break
		}
		if len(event.Data) == 0 {
			continue
		}
		if strings.TrimSpace(string(event.Data)) == "[DONE]" {
			break
		}
		chunkData, done, err := streamTranslator.TranslateChunk(event.Data)
		if err != nil {
			if uc != nil {
				uc.Status = http.StatusBadGateway
			}
			s.recordUsage(uc, lastUsage)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid upstream stream: " + err.Error()})
			return
		}
		if len(chunkData) > 0 && string(chunkData) != "[DONE]" {
			if !streamingPublished && uc != nil {
				streamingPublished = true
				s.publishRequestEvent(r, RequestEvent{RequestID: uc.RequestID, Type: "streaming", Provider: uc.Provider, Model: uc.Model, Status: "streaming", Endpoint: uc.Endpoint})
			}
			var chunk chatStreamChunk
			if err := json.Unmarshal(chunkData, &chunk); err != nil {
				if uc != nil {
					uc.Status = http.StatusBadGateway
				}
				s.recordUsage(uc, lastUsage)
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid upstream stream payload"})
				return
			}
			if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
				// A 502 the client was handed must still show up in usage, or the
				// failed request is invisible in stats and in the detail list.
				if uc != nil {
					uc.Status = http.StatusBadGateway
				}
				s.recordUsage(uc, lastUsage)
				writeJSON(w, http.StatusBadGateway, map[string]json.RawMessage{"error": chunk.Error})
				return
			}
			if u := usage.ExtractFromSSELine(chunkData); u.TotalTokens > 0 {
				lastUsage = u
			}
			if chunk.ID != "" {
				chunkID = chunk.ID
			}
			if chunk.Created != 0 {
				created = chunk.Created
			}
			for _, delta := range chunk.Choices {
				choice := choices[delta.Index]
				if choice == nil {
					choice = &choiceState{tools: make(map[int]*toolState)}
					choices[delta.Index] = choice
				}
				choice.content.WriteString(delta.Delta.Content)
				choice.reasoning.WriteString(delta.Delta.ReasoningContent)
				if delta.FinishReason != "" {
					choice.finishReason = delta.FinishReason
				}
				for _, call := range delta.Delta.ToolCalls {
					tool := choice.tools[call.Index]
					if tool == nil {
						tool = &toolState{kind: "function"}
						choice.tools[call.Index] = tool
					}
					if call.ID != "" {
						tool.id = call.ID
					}
					if call.Type != "" {
						tool.kind = call.Type
					}
					tool.name.WriteString(call.Function.Name)
					tool.arguments.WriteString(call.Function.Arguments)
				}
			}
		}
		if done {
			break
		}
	}

	if len(choices) == 0 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream stream contained no choices"})
		return
	}
	if chunkID == "" {
		chunkID = fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	}
	var output []any
	for _, index := range slices.Sorted(maps.Keys(choices)) {
		choice := choices[index]
		message := map[string]any{"role": "assistant", "content": choice.content.String()}
		if choice.reasoning.Len() > 0 {
			message["reasoning_content"] = choice.reasoning.String()
		}
		if len(choice.tools) > 0 {
			var calls []any
			for _, toolIndex := range slices.Sorted(maps.Keys(choice.tools)) {
				call := choice.tools[toolIndex]
				calls = append(calls, map[string]any{
					"id": call.id, "type": call.kind,
					"function": map[string]any{"name": call.name.String(), "arguments": call.arguments.String()},
				})
			}
			message["tool_calls"] = calls
			if choice.content.Len() == 0 {
				message["content"] = nil
			}
		}
		if choice.finishReason == "" {
			choice.finishReason = "stop"
			if len(choice.tools) > 0 {
				choice.finishReason = "tool_calls"
			}
		}
		output = append(output, map[string]any{"index": index, "message": message, "finish_reason": choice.finishReason})
		if uc != nil && index == 0 {
			uc.Response = choice.content.String()
		}
	}
	respObj := map[string]any{"id": chunkID, "object": "chat.completion", "created": created, "model": model, "choices": output}
	if lastUsage.TotalTokens > 0 {
		respObj["usage"] = lastUsage.OpenAI()
	}
	s.recordUsage(uc, lastUsage)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cyrene-Served-Model", model)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(respObj)
}

// proxyNonStreaming reads the full response, translates if needed, and writes it.
func (s *Server) proxyNonStreaming(w http.ResponseWriter, resp *http.Response, format translator.Format, model string, uc *usageContext) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to read upstream response"})
		return
	}
	// Unwrap {success: true, data: {...}} envelope (Cline, ClinePass, proxy wrappers)
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err == nil && env.Success && len(env.Data) > 0 {
		body = env.Data
	}

	// Extract usage before translation
	var u usage.Usage
	switch format {
	case translator.FormatAnthropic:
		u = usage.ExtractFromClaude(body)
	case translator.FormatGemini:
		u = usage.ExtractFromGemini(body)
	case translator.FormatResponses:
		u = usage.ExtractFromResponses(body)
	default:
		u = usage.ExtractFromOpenAI(body)
	}
	if format != translator.FormatOpenAI {
		translated, err := translator.TranslateResponse(format, body, model)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid upstream response: " + err.Error()})
			return
		}
		body = translated
	}
	var protocolError struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &protocolError); err == nil && len(protocolError.Error) > 0 && string(protocolError.Error) != "null" {
		writeJSON(w, http.StatusBadGateway, map[string]json.RawMessage{"error": protocolError.Error})
		return
	}
	var respObj struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &respObj); err == nil {
		if len(respObj.Choices) > 0 {
			if str, ok := respObj.Choices[0].Message.Content.(string); ok {
				uc.Response = str
			}
		} else if len(respObj.Content) > 0 {
			uc.Response = respObj.Content[0].Text
		}
	}

	if u.TotalTokens > 0 {
		slog.Info("Usage extracted",
			slog.String("model", model),
			slog.Int("prompt_tokens", u.PromptTokens),
			slog.Int("completion_tokens", u.CompletionTokens),
		)
	}
	s.recordUsage(uc, u)

	w.Header().Set("Content-Type", "application/json")
	// Extract upstream served model if available
	var modelObj struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &modelObj); err == nil && modelObj.Model != "" {
		w.Header().Set("X-Cyrene-Served-Model", modelObj.Model)
	} else {
		w.Header().Set("X-Cyrene-Served-Model", model)
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

// proxyStreaming handles SSE streaming with disconnect awareness and [DONE] handling.
func (s *Server) proxyStreaming(w http.ResponseWriter, r *http.Request, resp *http.Response, format translator.Format, model string, uc *usageContext) {
	flush := func() {
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Cyrene-Served-Model", model)
	w.WriteHeader(http.StatusOK)
	flush()
	ctx := r.Context()
	stallReader := provider.NewStallTimeoutReader(resp.Body, provider.DefaultStreamStallTimeout)
	defer stallReader.Close()
	reader := provider.NewSSEReader(stallReader)
	streamTranslator := translator.NewSSETranslator(format, model)
	var lastUsage usage.Usage
	var respBuilder strings.Builder
	streamingPublished := false
	finished := make(map[int]bool)
	fail := func(message string) {
		errChunk, _ := json.Marshal(map[string]any{"error": map[string]any{
			"message": message, "type": "upstream_error", "code": "upstream_error",
		}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", errChunk)
		flush()
	}

	for {
		event, err := reader.ReadEvent(ctx)
		if err != nil {
			if ctx.Err() != nil {
				if uc != nil {
					uc.Status = 499
					uc.Response = respBuilder.String()
				}
				s.recordUsage(uc, lastUsage)
				return
			}
			complete := len(finished) > 0
			for _, done := range finished {
				complete = complete && done
			}
			if !errors.Is(err, io.EOF) || !complete {
				if uc != nil {
					uc.Status = http.StatusBadGateway
					uc.Response = respBuilder.String()
				}
				s.recordUsage(uc, lastUsage)
				fail("upstream stream ended before completion: " + err.Error())
				return
			}
			break
		}
		if len(event.Data) == 0 {
			continue
		}
		if strings.TrimSpace(string(event.Data)) == "[DONE]" {
			break
		}
		chunkData, done, err := streamTranslator.TranslateChunk(event.Data)
		if err != nil {
			if uc != nil {
				uc.Status = http.StatusBadGateway
				uc.Response = respBuilder.String()
			}
			s.recordUsage(uc, lastUsage)
			fail("invalid upstream stream: " + err.Error())
			return
		}
		if len(chunkData) > 0 && string(chunkData) != "[DONE]" {
			if !streamingPublished && uc != nil {
				streamingPublished = true
				s.publishRequestEvent(r, RequestEvent{RequestID: uc.RequestID, Type: "streaming", Provider: uc.Provider, Model: uc.Model, Status: "streaming", Endpoint: uc.Endpoint})
			}
			var chunk chatStreamChunk
			if err := json.Unmarshal(chunkData, &chunk); err != nil {
				if uc != nil {
					uc.Status = http.StatusBadGateway
					uc.Response = respBuilder.String()
				}
				s.recordUsage(uc, lastUsage)
				fail("invalid upstream stream payload")
				return
			}
			if u := usage.ExtractFromSSELine(chunkData); u.TotalTokens > 0 {
				lastUsage = u
			}
			for _, choice := range chunk.Choices {
				finished[choice.Index] = finished[choice.Index] || choice.FinishReason != ""
				if respBuilder.Len() < 16384 {
					if choice.Delta.Content != "" {
						respBuilder.WriteString(choice.Delta.Content)
					} else if choice.Delta.ReasoningContent != "" {
						respBuilder.WriteString(choice.Delta.ReasoningContent)
					}
				}
			}
			var writeErr error
			if format == translator.FormatOpenAI {
				_, writeErr = w.Write(provider.FormatSSEEvent(*event))
			} else {
				_, writeErr = fmt.Fprintf(w, "data: %s\n\n", chunkData)
			}
			if writeErr != nil {
				if uc != nil {
					uc.Status = 499
					uc.Response = respBuilder.String()
				}
				s.recordUsage(uc, lastUsage)
				return
			}
			if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
				// The upstream failed in-band: same status and bookkeeping as the
				// other upstream failures, so the row is not lost from usage.
				if uc != nil {
					uc.Status = http.StatusBadGateway
					uc.Response = respBuilder.String()
				}
				s.recordUsage(uc, lastUsage)
				fmt.Fprint(w, "data: [DONE]\n\n")
				flush()
				return
			}
			flush()
		}
		if done {
			break
		}
	}
	if uc != nil {
		uc.Response = respBuilder.String()
	}
	s.recordUsage(uc, lastUsage)
	fmt.Fprint(w, "data: [DONE]\n\n")
	flush()
}

// extractRequestAPIKey extracts the caller credential for usage attribution.
// Delegates to the shared auth.ExtractAPIKey so every inbound protocol surface
// (OpenAI, Anthropic x-api-key, Gemini x-goog-api-key / ?key=) attributes
// consistently from one site (AGENTS.md §7).
func extractRequestAPIKey(r *http.Request) string {
	return auth.ExtractAPIKey(r)
}

// resolveRequestEndpoint extracts the original request endpoint, respecting
// proxy adapter overrides (e.g. X-Cyrene-Original-Endpoint) or default fallback.
func resolveRequestEndpoint(r *http.Request, defaultEndpoint string) string {
	if r == nil {
		return defaultEndpoint
	}
	if orig := r.Header.Get("X-Cyrene-Original-Endpoint"); orig != "" {
		return orig
	}
	if r.URL != nil && r.URL.Path != "" {
		return r.URL.Path
	}
	return defaultEndpoint
}
