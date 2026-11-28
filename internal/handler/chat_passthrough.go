package handler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
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

// effectiveProviderTarget resolves a connection's effective base URL and API
// type (dual-auth routing plus user overrides) for native passthrough paths.
func effectiveProviderTarget(providerInfo provider.ProviderInfo, conn *model.ProviderConnection) (string, string) {
	baseURL, apiType := providerInfo.EffectiveBaseURL(conn.AuthType, conn.Data.APIKey != "")
	if conn.Data.BaseURL != "" {
		baseURL = conn.Data.BaseURL
		if apiType == "" {
			apiType = providerInfo.APIType
		}
	}
	return baseURL, apiType
}

func providerSupportsAPIType(p provider.ProviderInfo, target string) bool {
	return p.APIType == target || p.OAuthAPIType == target || p.ApiKeyAPIType == target
}

func (s *Server) resolvePinnedConnection(conns []model.ProviderConnection, r *http.Request) *model.ProviderConnection {
	targetConnID := r.Header.Get("X-Cyrene-Connection-ID")
	if targetConnID == "" {
		return nil
	}
	for i := range conns {
		if conns[i].ID == targetConnID && conns[i].IsActive {
			return &conns[i]
		}
	}
	return nil
}

func hasNativeTargetConnection(providerInfo provider.ProviderInfo, conns []model.ProviderConnection, targetAPIType string) bool {
	for i := range conns {
		if !conns[i].IsActive {
			continue
		}
		if base, api := effectiveProviderTarget(providerInfo, &conns[i]); api == targetAPIType && base != "" {
			return true
		}
	}
	return false
}

func (s *Server) recordPassthroughError(
	r *http.Request,
	modelInfo model.ModelInfo,
	connID, endpoint string,
	start time.Time,
	statusCode int,
	errBody []byte,
	turns int,
	prompt string,
) {
	latencyMs := time.Since(start).Milliseconds()
	if latencyMs < 0 {
		latencyMs = 0
	}
	statusStr := fmt.Sprintf("%d", statusCode)
	rdID := fmt.Sprintf("%d-%s", nextRequestDetailStamp(), modelInfo.Model)
	ts := time.Now().UTC().Format(time.RFC3339)
	rdData := map[string]any{
		"id":           rdID,
		"timestamp":    ts,
		"provider":     modelInfo.Provider,
		"model":        modelInfo.Model,
		"connectionId": connID,
		"status":       statusStr,
		"latencyMs":    latencyMs,
		"endpoint":     endpoint,
		"client":       detectClientAgent(r),
		"error":        string(errBody),
		"turns":        turns,
		"input":        prompt,
	}
	rdBytes, _ := json.Marshal(rdData)
	if s.DB != nil {
		_ = s.DB.SaveRequestDetail(&db.RequestDetail{
			ID:           rdID,
			Timestamp:    ts,
			Provider:     modelInfo.Provider,
			Model:        modelInfo.Model,
			ConnectionID: connID,
			Status:       statusStr,
			Data:         string(rdBytes),
		})
	}
	if s.Metrics != nil {
		s.Metrics.ObserveRequest(modelInfo.Provider, modelInfo.Model, endpoint, statusCode, time.Since(start).Seconds(), nil)
	}
	s.publishRequestEvent(r, RequestEvent{
		Type:      "request",
		Provider:  modelInfo.Provider,
		Model:     modelInfo.Model,
		Status:    statusStr,
		Endpoint:  endpoint,
		LatencyMs: latencyMs,
	})
}

func (s *Server) doPassthroughWith401Refresh(
	r *http.Request,
	client *http.Client,
	conn *model.ProviderConnection,
	buildReq func() (*http.Request, error),
) (*http.Response, error) {
	upReq, err := buildReq()
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(upReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && conn != nil && conn.Data.RefreshToken != "" {
		origBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(origBody))
		slog.Warn("Passthrough upstream returned 401, attempting token refresh",
			slog.String("provider", conn.Provider),
			slog.String("connection_id", conn.ID),
		)
		if refreshResult, refreshErr := provider.RefreshCredentials(conn.Provider, conn, s.getHTTPClient(30*time.Second)); refreshErr == nil {
			if errPersist := s.persistRefreshedCredentials(conn, refreshResult); errPersist == nil {
				if retryReq, retryBuildErr := buildReq(); retryBuildErr == nil {
					if retryResp, retryErr := client.Do(retryReq); retryErr == nil {
						return retryResp, nil
					}
				}
			} else {
				slog.Error("Failed to persist refreshed credentials on passthrough", "provider", conn.Provider, "error", errPersist)
			}
		}
	}
	return resp, nil
}

func (s *Server) newPassthroughUsageContext(r *http.Request, modelInfo model.ModelInfo, connID, endpoint string, start time.Time, status, turns int, prompt string) *usageContext {
	apiKeyID, apiKeyName := extractRequestAPIKeyIdentity(r)
	return &usageContext{
		Client:        detectClientAgent(r),
		ClientVersion: detectClientVersion(r),
		RawUA:         rawUserAgent(r),
		RequestID:     ensureRequestID(r),
		Provider:      modelInfo.Provider,
		Model:         modelInfo.Model,
		ConnectionID:  connID,
		APIKey:        extractRequestAPIKey(r),
		APIKeyID:      apiKeyID,
		APIKeyName:    apiKeyName,
		Endpoint:      endpoint,
		StartedAt:     start,
		Status:        status,
		Prompt:        prompt,
		Turns:         turns,
	}
}

func extractResponsesPrompt(body map[string]any) (string, int) {
	switch in := body["input"].(type) {
	case string:
		if strings.TrimSpace(in) != "" {
			return capPromptText(in), 1
		}
	case []any:
		turns := 0
		lastUser := ""
		for _, raw := range in {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			role, _ := item["role"].(string)
			if !strings.EqualFold(role, "user") {
				continue
			}
			turns++
			switch c := item["content"].(type) {
			case string:
				if c != "" {
					lastUser = c
				}
			case []any:
				var sb strings.Builder
				for _, partRaw := range c {
					if part, ok := partRaw.(map[string]any); ok {
						if t, ok := part["text"].(string); ok && t != "" {
							if sb.Len() > 0 {
								sb.WriteString("\n")
							}
							sb.WriteString(t)
						}
					}
				}
				if sb.Len() > 0 {
					lastUser = sb.String()
				}
			}
		}
		return capPromptText(lastUser), turns
	}
	return "", 0
}

func extractGeminiPrompt(body map[string]any) (string, int) {
	contents, ok := body["contents"].([]any)
	if !ok {
		return "", 0
	}
	turns := 0
	lastUser := ""
	for _, raw := range contents {
		c, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role, _ := c["role"].(string)
		if role != "" && !strings.EqualFold(role, "user") {
			continue
		}
		turns++
		txt := strings.TrimSpace(geminiPartsText(c["parts"]))
		if txt != "" {
			lastUser = txt
		}
	}
	return capPromptText(lastUser), turns
}

// tryNativeResponsesPassthrough proxies a /v1/responses request directly to a
// native Responses upstream (such as OpenAI Codex OAuth or Grok Build) without
// lossy translation through Chat Completions. Returns false to fall back to the
// Chat adapter for combos or non-Responses upstreams.
func (s *Server) tryNativeResponsesPassthrough(
	w http.ResponseWriter,
	r *http.Request,
	reqBody map[string]any,
	rawBody []byte,
	modelStr string,
	clientStream bool,
) bool {
	cleanModel, modelEffort := provider.ExtractModelReasoningEffort(modelStr)
	if _, isCombo := provider.ResolveCombo(cleanModel, s.DB); isCombo {
		return false
	}
	modelInfo, err := provider.ResolveModel(cleanModel, s.DB)
	if err != nil || modelInfo.Provider == "" {
		return false
	}
	providerInfo, ok := provider.GetProvider(modelInfo.Provider)
	if !ok || provider.IsQoder(modelInfo.Provider) || modelInfo.Provider == "antigravity" || !providerSupportsAPIType(providerInfo, "responses") {
		return false
	}

	keyObj := auth.APIKeyFromContext(r.Context())
	if !keyAllowsModel(keyObj, cleanModel) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": fmt.Sprintf("model '%s' is not allowed for this API key", cleanModel),
		})
		return true
	}
	if provider.IsModelDisabled(cleanModel, s.DB) || provider.IsModelDisabled(modelInfo.Provider+"/"+modelInfo.Model, s.DB) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": fmt.Sprintf("model is disabled: %s", cleanModel)})
		return true
	}

	conns, err := s.DB.ListConnectionsByProvider(modelInfo.Provider)
	if err != nil || len(conns) == 0 {
		return false
	}
	pinned := s.resolvePinnedConnection(conns, r)
	if pinned != nil {
		if base, api := effectiveProviderTarget(providerInfo, pinned); api != "responses" || base == "" {
			return false
		}
	} else if !hasNativeTargetConnection(providerInfo, conns, "responses") {
		// e.g. OpenAI API-key-only pool uses Chat Completions, so skip selecting
		// here and let the Chat adapter handle it without skewing round-robin.
		return false
	}

	sessionKey := resolveSessionKey(r, reqBody)
	conn := pinned
	if conn == nil {
		conn = s.selectAvailableConnection(conns, modelInfo.Model, nil, sessionKey)
	}
	if conn == nil {
		return false
	}
	s.tryRefreshToken(conn)
	baseURL, effectiveAPIType := effectiveProviderTarget(providerInfo, conn)
	if effectiveAPIType != "responses" || baseURL == "" {
		return false
	}

	requestID := ensureRequestID(r)
	w.Header().Set(requestIDHeader, requestID)

	if modelEffort != "" {
		reqBody["model"] = modelInfo.Model
		if _, hasReasoning := reqBody["reasoning"]; !hasReasoning && modelEffort != "none" && modelEffort != "off" {
			reqBody["reasoning"] = map[string]any{"effort": modelEffort}
		}
	}
	if keyObj != nil && keyObj.SystemPrompt != "" {
		rtk.InjectSystemPrompt(reqBody, "responses", keyObj.SystemPrompt)
		if updated, err := json.Marshal(reqBody); err == nil {
			rawBody = updated
		}
	}

	var settings *db.Settings
	if s.DB != nil {
		settings, _ = s.DB.GetSettings()
	}
	cacheEnabled := settings != nil && settings.ResponseCacheEnabled && s.Cache != nil
	var cacheKey string
	if !clientStream && cacheEnabled && !cache.ShouldBypass(r.Header) {
		eligible, key, err := cache.IsEligible("responses:"+modelInfo.Provider, rawBody, settings.ResponseCacheAll)
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
				return true
			}
		}
	}
	if cacheKey != "" {
		w.Header().Set("X-Cyrene-Cache", "MISS")
		rec := cache.NewRecorder(w)
		defer func() {
			if rec.ShouldCache() {
				ttl := time.Duration(settings.ResponseCacheTTL) * time.Second
				if ttl <= 0 {
					ttl = time.Hour
				}
				s.Cache.Set(cacheKey, rec.StatusCode(), rec.HeaderMap(), rec.BodyBytes(), ttl, rec.Header().Get("X-Cyrene-Served-Model"), rec.ExtractTokens())
			}
		}()
		w = rec
	}

	upstreamStream := clientStream || providerInfo.ForceStream
	reqBody["model"] = modelInfo.Model
	reqBody["stream"] = upstreamStream
	s.applyTokenSaver(reqBody, "responses", modelInfo.Provider, r)
	s.applyLoopGuard(reqBody, rawBody, "responses", modelInfo.Provider, modelStr, reqBody["tools"] != nil)

	endpoint := resolveRequestEndpoint(r, "/v1/responses")
	prompt, turns := extractResponsesPrompt(reqBody)

	tried := map[string]bool{conn.ID: true}
	var lastStatus int
	var lastBody []byte
	for cur := conn; cur != nil; {
		retryable, status, errBody := s.attemptResponsesPassthrough(w, r, reqBody, modelInfo, providerInfo, cur, endpoint, clientStream, upstreamStream, prompt, turns)
		if !retryable {
			return true
		}
		lastStatus, lastBody = status, errBody
		if pinned != nil {
			break
		}
		if provider.CheckFallbackError(status, string(errBody), cur.Data.BackoffLevel).IsModelSaturation {
			break
		}
		s.publishRequestEvent(r, RequestEvent{Type: "fallback", Provider: modelInfo.Provider, Model: modelInfo.Model, Status: "fallback", Endpoint: endpoint})
		cur = nil
		for {
			next := s.selectAvailableConnection(conns, modelInfo.Model, tried, sessionKey)
			if next == nil {
				break
			}
			tried[next.ID] = true
			s.tryRefreshToken(next)
			if b, e := effectiveProviderTarget(providerInfo, next); e == "responses" && b != "" {
				cur = next
				break
			}
		}
	}

	if lastStatus == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("no eligible accounts for provider: %s", modelInfo.Provider),
		})
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(lastStatus)
	w.Write(wrapErrorWithHint(lastStatus, lastBody, modelInfo.Provider))
	return true
}

func cloneMapShallowAndSanitizeResponsesBody(reqBody map[string]any, modelInfo model.ModelInfo, conn *model.ProviderConnection) ([]byte, error) {
	if modelInfo.Provider == "openai" && conn != nil && conn.AuthType == "oauth" {
		cloned := make(map[string]any, len(reqBody)+2)
		maps.Copy(cloned, reqBody)
		if tools, ok := cloned["tools"].([]any); ok {
			// Copy tools recursively only via marshal/unmarshal if stripping is needed.
			rawTools, err := json.Marshal(tools)
			if err == nil {
				var toolsCopy []any
				if json.Unmarshal(rawTools, &toolsCopy) == nil {
					for _, tool := range toolsCopy {
						if tm, ok := tool.(map[string]any); ok {
							if params, ok := tm["parameters"].(map[string]any); ok {
								translator.StripCodexUnsupportedPatterns(params)
							}
							if fn, ok := tm["function"].(map[string]any); ok {
								if params, ok := fn["parameters"].(map[string]any); ok {
									translator.StripCodexUnsupportedPatterns(params)
								}
							}
						}
					}
					cloned["tools"] = toolsCopy
				}
			}
		}
		cloned["store"] = false
		if _, ok := cloned["instructions"]; !ok {
			cloned["instructions"] = ""
		}
		return json.Marshal(cloned)
	}
	return json.Marshal(reqBody)
}

func (s *Server) attemptResponsesPassthrough(
	w http.ResponseWriter,
	r *http.Request,
	reqBody map[string]any,
	modelInfo model.ModelInfo,
	providerInfo provider.ProviderInfo,
	conn *model.ProviderConnection,
	endpoint string,
	clientStream, upstreamStream bool,
	prompt string,
	turns int,
) (retryable bool, status int, errBody []byte) {
	baseURL, _ := effectiveProviderTarget(providerInfo, conn)
	transport := provider.ResolveTransport(providerInfo, baseURL, "responses", conn)
	targetURL := provider.BuildTransportURL(transport, modelInfo.Model, upstreamStream)
	if targetURL == "" {
		targetURL = strings.TrimRight(baseURL, "/") + "/responses"
	}

	payloadBytes, err := cloneMapShallowAndSanitizeResponsesBody(reqBody, modelInfo, conn)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encode responses request"})
		return false, 0, nil
	}

	buildReq := func() (*http.Request, error) {
		upReq, reqErr := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(payloadBytes))
		if reqErr != nil {
			return nil, reqErr
		}
		for k, v := range transport.Headers {
			upReq.Header.Set(k, v)
		}
		if modelInfo.Provider == "grok-cli" {
			provider.ApplyGrokConversationHeaders(upReq, reqBody, conn)
		}
		creds := provider.ResolveCredentials(conn, modelInfo.Provider, modelInfo.Model)
		provider.ApplyAuth(upReq, transport, creds)
		upReq.Header.Set("Content-Type", "application/json")
		if upstreamStream {
			upReq.Header.Set("Accept", "text/event-stream")
		}
		return upReq, nil
	}

	start := time.Now()
	s.publishRequestEvent(r, RequestEvent{Type: "routing", Provider: modelInfo.Provider, Model: modelInfo.Model, Endpoint: endpoint, Status: "routing"})

	clientTimeout := 5 * time.Minute
	if clientStream || upstreamStream {
		clientTimeout = 0
	}
	resp, err := s.doPassthroughWith401Refresh(r, s.getHTTPClient(clientTimeout), conn, buildReq)
	if err != nil {
		if r.Context().Err() != nil {
			return false, 0, nil
		}
		provider.ApplyErrorState(conn, http.StatusBadGateway, err.Error())
		_ = s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
		return true, http.StatusBadGateway, []byte("upstream request failed: " + err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		provider.ApplyErrorState(conn, resp.StatusCode, string(body))
		_ = s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
		s.recordPassthroughError(r, modelInfo, conn.ID, endpoint, start, resp.StatusCode, body, turns, prompt)
		if provider.CheckFallbackError(resp.StatusCode, string(body), conn.Data.BackoffLevel).ShouldFallback {
			return true, resp.StatusCode, body
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(wrapErrorWithHint(resp.StatusCode, body, modelInfo.Provider))
		return false, 0, nil
	}

	provider.ResetAccountState(conn)
	_ = s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
	s.publishRequestEvent(r, RequestEvent{Type: "upstream_connected", Provider: modelInfo.Provider, Model: modelInfo.Model, Status: "upstream_connected", Endpoint: endpoint})

	uc := s.newPassthroughUsageContext(r, modelInfo, conn.ID, endpoint, start, resp.StatusCode, turns, prompt)
	isSSE := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")

	switch {
	case isSSE && clientStream:
		s.proxyNativeResponsesSSE(w, r, resp, modelInfo.Model, uc)
	case isSSE && !clientStream:
		s.aggregateNativeResponsesSSE(w, r, resp, modelInfo.Model, uc)
	case !isSSE && clientStream:
		s.streamResponsesFromJSON(w, resp, modelInfo.Model, uc)
	default:
		s.proxyNativeResponsesNonStreaming(w, resp, modelInfo.Model, uc)
	}
	return false, 0, nil
}

func extractResponsesCompletedUsage(data []byte) usage.Usage {
	var env struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(data, &env); err == nil && len(env.Response) > 0 && string(env.Response) != "null" {
		if u := usage.ExtractFromResponses(env.Response); u.Known || u.TotalTokens > 0 {
			return u
		}
	}
	return usage.ExtractFromResponses(data)
}

func extractResponsesOutputText(body []byte) string {
	var parsed struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Type    string `json:"type"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	if parsed.OutputText != "" {
		return parsed.OutputText
	}
	var sb strings.Builder
	for _, item := range parsed.Output {
		if item.Type == "message" {
			for _, part := range item.Content {
				sb.WriteString(part.Text)
			}
		}
	}
	return sb.String()
}

func (s *Server) proxyNativeResponsesSSE(w http.ResponseWriter, r *http.Request, resp *http.Response, model string, uc *usageContext) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Cyrene-Served-Model", model)
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		_, _ = io.Copy(w, resp.Body)
		s.recordUsage(uc, usage.Usage{})
		return
	}
	flusher.Flush()

	stallReader := provider.NewStallTimeoutReader(resp.Body, provider.DefaultStreamStallTimeout)
	defer stallReader.Close()
	scanner := bufio.NewScanner(stallReader)
	scanner.Buffer(make([]byte, 0, 64*1024), maxSSELineBytes)

	var totalUsage usage.Usage
	var outBuilder strings.Builder
	sawEventHeader := false
	ctx := r.Context()

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			if uc != nil {
				uc.Status = 499
				uc.Response = outBuilder.String()
			}
			s.recordUsage(uc, totalUsage)
			return
		default:
		}
		line := scanner.Bytes()
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			sawEventHeader = false
			_, _ = w.Write(line)
			_, _ = w.Write(newlineBytes)
			flusher.Flush()
			continue
		}
		if bytes.HasPrefix(trimmed, []byte("event:")) {
			sawEventHeader = true
			_, _ = w.Write(line)
			_, _ = w.Write(newlineBytes)
			flusher.Flush()
			continue
		}
		if data, isDone, okData := translator.ParseSSEDataLine(line); okData && !isDone {
			if u := extractResponsesCompletedUsage(data); u.TotalTokens > 0 || u.Known {
				totalUsage = u
			}
			var ev struct {
				Type  string `json:"type"`
				Delta string `json:"delta"`
			}
			if json.Unmarshal(data, &ev) == nil {
				if ev.Type != "" && !sawEventHeader {
					_, _ = fmt.Fprintf(w, "event: %s\n", ev.Type)
				}
				if ev.Type == "response.output_text.delta" && ev.Delta != "" && outBuilder.Len() < 16384 {
					outBuilder.WriteString(ev.Delta)
				}
			}
		}
		sawEventHeader = false
		_, _ = w.Write(line)
		_, _ = w.Write(newlineBytes)
		flusher.Flush()
	}

	if scanErr := scanner.Err(); scanErr != nil {
		if uc != nil {
			uc.Status = http.StatusBadGateway
		}
		_, _ = fmt.Fprintf(w, "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"server_error\",\"message\":%q}}\n\n", scanErr.Error())
		flusher.Flush()
	}
	if uc != nil {
		uc.Response = outBuilder.String()
	}
	s.recordUsage(uc, totalUsage)
}

func (s *Server) aggregateNativeResponsesSSE(w http.ResponseWriter, r *http.Request, resp *http.Response, model string, uc *usageContext) {
	stallReader := provider.NewStallTimeoutReader(resp.Body, provider.DefaultStreamStallTimeout)
	defer stallReader.Close()
	reader := provider.NewSSEReader(stallReader)

	var textBuf strings.Builder
	var reasoningBuf strings.Builder
	var doneItems []any
	var completedResp map[string]any
	var streamErr any
	var lastUsage usage.Usage
	ctx := r.Context()

	for {
		ev, err := reader.ReadEvent(ctx)
		if err != nil {
			if ctx.Err() != nil {
				if uc != nil {
					uc.Status = 499
					uc.Response = textBuf.String()
				}
				s.recordUsage(uc, lastUsage)
				return
			}
			break
		}
		if len(ev.Data) == 0 || bytes.Equal(bytes.TrimSpace(ev.Data), []byte("[DONE]")) {
			continue
		}
		var payload map[string]any
		if json.Unmarshal(ev.Data, &payload) != nil {
			continue
		}
		if errObj := payload["error"]; errObj != nil {
			streamErr = errObj
			break
		}
		evType, _ := payload["type"].(string)
		switch evType {
		case "error":
			streamErr = payload
		case "response.output_text.delta":
			if d, ok := payload["delta"].(string); ok {
				textBuf.WriteString(d)
			}
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta", "response.reasoning.delta":
			if d, ok := payload["delta"].(string); ok {
				reasoningBuf.WriteString(d)
			}
		case "response.output_item.done":
			if item := payload["item"]; item != nil {
				doneItems = append(doneItems, item)
			}
		case "response.completed", "response.done", "response.incomplete", "response.failed":
			if respMap, ok := payload["response"].(map[string]any); ok {
				completedResp = respMap
				if u := extractResponsesCompletedUsage(ev.Data); u.TotalTokens > 0 || u.Known {
					lastUsage = u
				}
				if evType == "response.failed" || respMap["error"] != nil {
					if respMap["error"] != nil {
						streamErr = respMap["error"]
					} else {
						streamErr = map[string]any{"message": "upstream response failed"}
					}
				}
			}
		}
		if streamErr != nil {
			break
		}
	}

	if streamErr != nil {
		if uc != nil {
			uc.Status = http.StatusBadGateway
			uc.Response = textBuf.String()
		}
		s.recordUsage(uc, lastUsage)
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": streamErr})
		return
	}

	if completedResp == nil {
		completedResp = map[string]any{
			"id":         fmt.Sprintf("resp_%d", time.Now().UnixNano()),
			"object":     "response",
			"created_at": time.Now().Unix(),
			"status":     "completed",
			"model":      model,
		}
	}
	if _, hasObj := completedResp["object"]; !hasObj {
		completedResp["object"] = "response"
	}
	if _, hasModel := completedResp["model"]; !hasModel {
		completedResp["model"] = model
	}
	if _, hasStatus := completedResp["status"]; !hasStatus {
		completedResp["status"] = "completed"
	}

	existingOut, _ := completedResp["output"].([]any)
	if len(existingOut) == 0 {
		if len(doneItems) > 0 {
			completedResp["output"] = doneItems
		} else {
			var built []any
			if reasoningBuf.Len() > 0 {
				built = append(built, map[string]any{
					"id":     fmt.Sprintf("rs_%d", time.Now().UnixNano()),
					"type":   "reasoning",
					"status": "completed",
					"summary": []any{
						map[string]any{"type": "summary_text", "text": reasoningBuf.String()},
					},
				})
			}
			if textBuf.Len() > 0 {
				built = append(built, map[string]any{
					"id":     fmt.Sprintf("msg_%d", time.Now().UnixNano()),
					"type":   "message",
					"status": "completed",
					"role":   "assistant",
					"content": []any{
						map[string]any{"type": "output_text", "text": textBuf.String()},
					},
				})
			}
			if built == nil {
				built = []any{}
			}
			completedResp["output"] = built
		}
	}
	if _, hasOutText := completedResp["output_text"]; !hasOutText && textBuf.Len() > 0 {
		completedResp["output_text"] = textBuf.String()
	}

	encoded, err := json.Marshal(completedResp)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encode aggregated response"})
		return
	}
	if u := usage.ExtractFromResponses(encoded); u.TotalTokens > 0 || u.Known {
		lastUsage = u
	}
	if uc != nil {
		uc.Response = extractResponsesOutputText(encoded)
		if uc.Response == "" {
			uc.Response = textBuf.String()
		}
	}
	s.recordUsage(uc, lastUsage)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cyrene-Served-Model", model)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}

func (s *Server) proxyNativeJSON(
	w http.ResponseWriter,
	resp *http.Response,
	model string,
	uc *usageContext,
	extractUsage func([]byte) usage.Usage,
	extractText func([]byte) string,
) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to read upstream response"})
		return
	}
	u := extractUsage(body)
	if uc != nil {
		uc.Response = extractText(body)
	}
	s.recordUsage(uc, u)

	for k, vals := range resp.Header {
		if isHopByHopHeader(k) {
			continue
		}
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cyrene-Served-Model", model)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func (s *Server) streamSingleJSONAsSSE(
	w http.ResponseWriter,
	resp *http.Response,
	model string,
	uc *usageContext,
	extractUsage func([]byte) usage.Usage,
	extractText func([]byte) string,
	formatFrame func([]byte) string,
) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to read upstream response"})
		return
	}
	u := extractUsage(body)
	if uc != nil {
		uc.Response = extractText(body)
	}
	s.recordUsage(uc, u)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Cyrene-Served-Model", model)
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, formatFrame(bytes.TrimSpace(body)))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *Server) proxyNativeResponsesNonStreaming(w http.ResponseWriter, resp *http.Response, model string, uc *usageContext) {
	s.proxyNativeJSON(w, resp, model, uc, usage.ExtractFromResponses, extractResponsesOutputText)
}

func (s *Server) streamResponsesFromJSON(w http.ResponseWriter, resp *http.Response, model string, uc *usageContext) {
	s.streamSingleJSONAsSSE(w, resp, model, uc, usage.ExtractFromResponses, extractResponsesOutputText, func(b []byte) string {
		return fmt.Sprintf("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":%s}\n\n", b)
	})
}

// tryNativeGeminiPassthrough proxies a /v1beta/models/* generateContent request
// directly to a native Gemini upstream while keeping Antigravity on its specialized path.
func (s *Server) tryNativeGeminiPassthrough(
	w http.ResponseWriter,
	r *http.Request,
	reqBody map[string]any,
	rawBody []byte,
	modelStr, endpoint string,
	clientStream bool,
) bool {
	cleanModel, modelEffort := provider.ExtractModelReasoningEffort(modelStr)
	if _, isCombo := provider.ResolveCombo(cleanModel, s.DB); isCombo {
		return false
	}
	modelInfo, err := provider.ResolveModel(cleanModel, s.DB)
	if err != nil || modelInfo.Provider == "" {
		return false
	}
	providerInfo, ok := provider.GetProvider(modelInfo.Provider)
	if !ok || provider.IsQoder(modelInfo.Provider) || modelInfo.Provider == "antigravity" || !providerSupportsAPIType(providerInfo, "gemini") {
		return false
	}

	keyObj := auth.APIKeyFromContext(r.Context())
	if !keyAllowsModel(keyObj, cleanModel) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": fmt.Sprintf("model '%s' is not allowed for this API key", cleanModel),
		})
		return true
	}
	if provider.IsModelDisabled(cleanModel, s.DB) || provider.IsModelDisabled(modelInfo.Provider+"/"+modelInfo.Model, s.DB) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": fmt.Sprintf("model is disabled: %s", cleanModel)})
		return true
	}

	conns, err := s.DB.ListConnectionsByProvider(modelInfo.Provider)
	if err != nil || len(conns) == 0 {
		return false
	}
	pinned := s.resolvePinnedConnection(conns, r)
	if pinned != nil {
		if base, api := effectiveProviderTarget(providerInfo, pinned); api != "gemini" || base == "" {
			return false
		}
	} else if !hasNativeTargetConnection(providerInfo, conns, "gemini") {
		return false
	}

	sessionKey := resolveSessionKey(r, reqBody)
	conn := pinned
	if conn == nil {
		conn = s.selectAvailableConnection(conns, modelInfo.Model, nil, sessionKey)
	}
	if conn == nil {
		return false
	}
	s.tryRefreshToken(conn)
	baseURL, effectiveAPIType := effectiveProviderTarget(providerInfo, conn)
	if effectiveAPIType != "gemini" || baseURL == "" {
		return false
	}

	requestID := ensureRequestID(r)
	w.Header().Set(requestIDHeader, requestID)

	if modelEffort != "" {
		injectGeminiEffortIfMissing(reqBody, modelEffort)
	}
	if keyObj != nil && keyObj.SystemPrompt != "" {
		rtk.InjectSystemPrompt(reqBody, "gemini", keyObj.SystemPrompt)
	}

	var settings *db.Settings
	if s.DB != nil {
		settings, _ = s.DB.GetSettings()
	}
	cacheEnabled := settings != nil && settings.ResponseCacheEnabled && s.Cache != nil
	var cacheKey string
	if !clientStream && cacheEnabled && !cache.ShouldBypass(r.Header) {
		cachePayload, _ := json.Marshal(reqBody)
		eligible, key, err := cache.IsEligible("gemini:"+modelInfo.Provider+"/"+modelInfo.Model, cachePayload, settings.ResponseCacheAll)
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
				return true
			}
		}
	}
	if cacheKey != "" {
		w.Header().Set("X-Cyrene-Cache", "MISS")
		rec := cache.NewRecorder(w)
		defer func() {
			if rec.ShouldCache() {
				ttl := time.Duration(settings.ResponseCacheTTL) * time.Second
				if ttl <= 0 {
					ttl = time.Hour
				}
				s.Cache.Set(cacheKey, rec.StatusCode(), rec.HeaderMap(), rec.BodyBytes(), ttl, rec.Header().Get("X-Cyrene-Served-Model"), rec.ExtractTokens())
			}
		}()
		w = rec
	}

	// Set "model" temporarily so tokenSaver exclusion rules can inspect it, then
	// delete it because Gemini generateContent rejects unknown top-level "model".
	reqBody["model"] = modelInfo.Model
	s.applyTokenSaver(reqBody, "gemini", modelInfo.Provider, r)
	delete(reqBody, "model")
	s.applyLoopGuard(reqBody, rawBody, "gemini", modelInfo.Provider, modelStr, reqBody["tools"] != nil)

	payloadBytes, err := json.Marshal(reqBody)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encode gemini request"})
		return true
	}

	upstreamStream := clientStream || providerInfo.ForceStream
	prompt, turns := extractGeminiPrompt(reqBody)

	tried := map[string]bool{conn.ID: true}
	var lastStatus int
	var lastBody []byte
	for cur := conn; cur != nil; {
		retryable, status, errBody := s.attemptGeminiPassthrough(w, r, payloadBytes, modelInfo, providerInfo, cur, endpoint, clientStream, upstreamStream, prompt, turns)
		if !retryable {
			return true
		}
		lastStatus, lastBody = status, errBody
		if pinned != nil {
			break
		}
		if provider.CheckFallbackError(status, string(errBody), cur.Data.BackoffLevel).IsModelSaturation {
			break
		}
		s.publishRequestEvent(r, RequestEvent{Type: "fallback", Provider: modelInfo.Provider, Model: modelInfo.Model, Status: "fallback", Endpoint: endpoint})
		cur = nil
		for {
			next := s.selectAvailableConnection(conns, modelInfo.Model, tried, sessionKey)
			if next == nil {
				break
			}
			tried[next.ID] = true
			s.tryRefreshToken(next)
			if b, e := effectiveProviderTarget(providerInfo, next); e == "gemini" && b != "" {
				cur = next
				break
			}
		}
	}

	if lastStatus == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("no eligible accounts for provider: %s", modelInfo.Provider),
		})
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(lastStatus)
	w.Write(wrapErrorWithHint(lastStatus, lastBody, modelInfo.Provider))
	return true
}

func injectGeminiEffortIfMissing(reqBody map[string]any, effort string) {
	gc, _ := reqBody["generationConfig"].(map[string]any)
	if gc == nil {
		gc = make(map[string]any)
		reqBody["generationConfig"] = gc
	}
	if _, hasTC := gc["thinkingConfig"]; hasTC {
		return
	}
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "minimal":
		gc["thinkingConfig"] = map[string]any{"thinkingBudget": 1024, "includeThoughts": true}
	case "low":
		gc["thinkingConfig"] = map[string]any{"thinkingBudget": 2048, "includeThoughts": true}
	case "medium":
		gc["thinkingConfig"] = map[string]any{"thinkingBudget": 8192, "includeThoughts": true}
	case "high":
		gc["thinkingConfig"] = map[string]any{"thinkingBudget": 16384, "includeThoughts": true}
	case "max", "xhigh":
		gc["thinkingConfig"] = map[string]any{"thinkingBudget": 32768, "includeThoughts": true}
	case "adaptive":
		gc["thinkingConfig"] = map[string]any{"thinkingBudget": -1, "includeThoughts": true}
	case "none", "off":
		gc["thinkingConfig"] = map[string]any{"thinkingBudget": 0}
	}
}

func (s *Server) attemptGeminiPassthrough(
	w http.ResponseWriter,
	r *http.Request,
	payloadBytes []byte,
	modelInfo model.ModelInfo,
	providerInfo provider.ProviderInfo,
	conn *model.ProviderConnection,
	endpoint string,
	clientStream, upstreamStream bool,
	prompt string,
	turns int,
) (retryable bool, status int, errBody []byte) {
	baseURL, _ := effectiveProviderTarget(providerInfo, conn)
	transport := provider.ResolveTransport(providerInfo, baseURL, "gemini", conn)
	targetURL := provider.BuildTransportURL(transport, modelInfo.Model, upstreamStream)
	if targetURL == "" {
		targetURL = provider.BuildGeminiURL(baseURL, modelInfo.Model, upstreamStream)
	}

	buildReq := func() (*http.Request, error) {
		upReq, reqErr := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(payloadBytes))
		if reqErr != nil {
			return nil, reqErr
		}
		for k, v := range transport.Headers {
			upReq.Header.Set(k, v)
		}
		creds := provider.ResolveCredentials(conn, modelInfo.Provider, modelInfo.Model)
		provider.ApplyAuth(upReq, transport, creds)
		upReq.Header.Set("Content-Type", "application/json")
		if upstreamStream {
			upReq.Header.Set("Accept", "text/event-stream")
		}
		return upReq, nil
	}

	start := time.Now()
	s.publishRequestEvent(r, RequestEvent{Type: "routing", Provider: modelInfo.Provider, Model: modelInfo.Model, Endpoint: endpoint, Status: "routing"})

	clientTimeout := 5 * time.Minute
	if clientStream || upstreamStream {
		clientTimeout = 0
	}
	resp, err := s.doPassthroughWith401Refresh(r, s.getHTTPClient(clientTimeout), conn, buildReq)
	if err != nil {
		if r.Context().Err() != nil {
			return false, 0, nil
		}
		provider.ApplyErrorState(conn, http.StatusBadGateway, err.Error())
		_ = s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
		return true, http.StatusBadGateway, []byte("upstream request failed: " + err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		provider.ApplyErrorState(conn, resp.StatusCode, string(body))
		_ = s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
		s.recordPassthroughError(r, modelInfo, conn.ID, endpoint, start, resp.StatusCode, body, turns, prompt)
		if provider.CheckFallbackError(resp.StatusCode, string(body), conn.Data.BackoffLevel).ShouldFallback {
			return true, resp.StatusCode, body
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(wrapErrorWithHint(resp.StatusCode, body, modelInfo.Provider))
		return false, 0, nil
	}

	provider.ResetAccountState(conn)
	_ = s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
	s.publishRequestEvent(r, RequestEvent{Type: "upstream_connected", Provider: modelInfo.Provider, Model: modelInfo.Model, Status: "upstream_connected", Endpoint: endpoint})

	uc := s.newPassthroughUsageContext(r, modelInfo, conn.ID, endpoint, start, resp.StatusCode, turns, prompt)
	isSSE := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")

	switch {
	case isSSE && clientStream:
		s.proxyNativeGeminiSSE(w, r, resp, modelInfo.Model, uc)
	case isSSE && !clientStream:
		s.aggregateNativeGeminiSSE(w, r, resp, modelInfo.Model, uc)
	case !isSSE && clientStream:
		s.streamGeminiFromJSON(w, resp, modelInfo.Model, uc)
	default:
		s.proxyNativeGeminiNonStreaming(w, resp, modelInfo.Model, uc)
	}
	return false, 0, nil
}

func extractGeminiCandidateText(body []byte) string {
	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text    string `json:"text"`
					Thought bool   `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, c := range parsed.Candidates {
		for _, p := range c.Content.Parts {
			if !p.Thought && p.Text != "" {
				sb.WriteString(p.Text)
			}
		}
	}
	return sb.String()
}

func (s *Server) proxyNativeGeminiSSE(w http.ResponseWriter, r *http.Request, resp *http.Response, model string, uc *usageContext) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Cyrene-Served-Model", model)
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		_, _ = io.Copy(w, resp.Body)
		s.recordUsage(uc, usage.Usage{})
		return
	}
	flusher.Flush()

	stallReader := provider.NewStallTimeoutReader(resp.Body, provider.DefaultStreamStallTimeout)
	defer stallReader.Close()
	scanner := bufio.NewScanner(stallReader)
	scanner.Buffer(make([]byte, 0, 64*1024), maxSSELineBytes)

	var totalUsage usage.Usage
	var outBuilder strings.Builder
	ctx := r.Context()

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			if uc != nil {
				uc.Status = 499
				uc.Response = outBuilder.String()
			}
			s.recordUsage(uc, totalUsage)
			return
		default:
		}
		line := scanner.Bytes()
		if data, isDone, okData := translator.ParseSSEDataLine(line); okData && !isDone {
			if u := usage.ExtractFromGemini(data); u.TotalTokens > 0 || u.Known {
				totalUsage = u
			}
			if txt := extractGeminiCandidateText(data); txt != "" && outBuilder.Len() < 16384 {
				outBuilder.WriteString(txt)
			}
		}
		_, _ = w.Write(line)
		_, _ = w.Write(newlineBytes)
		flusher.Flush()
	}

	if scanErr := scanner.Err(); scanErr != nil {
		if uc != nil {
			uc.Status = http.StatusBadGateway
		}
		_, _ = fmt.Fprintf(w, "data: {\"error\":{\"code\":502,\"status\":\"UNAVAILABLE\",\"message\":%q}}\n\n", scanErr.Error())
		flusher.Flush()
	}
	if uc != nil {
		uc.Response = outBuilder.String()
	}
	s.recordUsage(uc, totalUsage)
}

func (s *Server) aggregateNativeGeminiSSE(w http.ResponseWriter, r *http.Request, resp *http.Response, model string, uc *usageContext) {
	stallReader := provider.NewStallTimeoutReader(resp.Body, provider.DefaultStreamStallTimeout)
	defer stallReader.Close()
	reader := provider.NewSSEReader(stallReader)

	var allParts []any
	finishReason := ""
	modelVersion := model
	var lastUsageMeta any
	var lastUsage usage.Usage
	var textBuf strings.Builder
	ctx := r.Context()

	for {
		ev, err := reader.ReadEvent(ctx)
		if err != nil {
			if ctx.Err() != nil {
				if uc != nil {
					uc.Status = 499
					uc.Response = textBuf.String()
				}
				s.recordUsage(uc, lastUsage)
				return
			}
			break
		}
		if len(ev.Data) == 0 || bytes.Equal(bytes.TrimSpace(ev.Data), []byte("[DONE]")) {
			continue
		}
		var payload map[string]any
		if json.Unmarshal(ev.Data, &payload) != nil {
			continue
		}
		if errObj := payload["error"]; errObj != nil {
			if uc != nil {
				uc.Status = http.StatusBadGateway
				uc.Response = textBuf.String()
			}
			s.recordUsage(uc, lastUsage)
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": errObj})
			return
		}
		if mv, ok := payload["modelVersion"].(string); ok && mv != "" {
			modelVersion = mv
		}
		if um := payload["usageMetadata"]; um != nil {
			lastUsageMeta = um
		}
		if u := usage.ExtractFromGemini(ev.Data); u.TotalTokens > 0 || u.Known {
			lastUsage = u
		}
		if cands, ok := payload["candidates"].([]any); ok && len(cands) > 0 {
			if c0, ok := cands[0].(map[string]any); ok {
				if fr, ok := c0["finishReason"].(string); ok && fr != "" {
					finishReason = fr
				}
				if content, ok := c0["content"].(map[string]any); ok {
					if parts, ok := content["parts"].([]any); ok {
						for _, p := range parts {
							allParts = append(allParts, p)
							if pm, ok := p.(map[string]any); ok {
								if thought, _ := pm["thought"].(bool); !thought {
									if t, ok := pm["text"].(string); ok {
										textBuf.WriteString(t)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if allParts == nil {
		allParts = []any{}
	}
	if finishReason == "" {
		finishReason = "STOP"
	}
	out := map[string]any{
		"candidates": []any{
			map[string]any{
				"content": map[string]any{
					"role":  "model",
					"parts": allParts,
				},
				"finishReason": finishReason,
				"index":        0,
			},
		},
		"modelVersion": modelVersion,
	}
	if lastUsageMeta != nil {
		out["usageMetadata"] = lastUsageMeta
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encode gemini response"})
		return
	}
	if uc != nil {
		uc.Response = textBuf.String()
	}
	s.recordUsage(uc, lastUsage)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cyrene-Served-Model", model)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}

func (s *Server) proxyNativeGeminiNonStreaming(w http.ResponseWriter, resp *http.Response, model string, uc *usageContext) {
	s.proxyNativeJSON(w, resp, model, uc, usage.ExtractFromGemini, extractGeminiCandidateText)
}

func (s *Server) streamGeminiFromJSON(w http.ResponseWriter, resp *http.Response, model string, uc *usageContext) {
	s.streamSingleJSONAsSSE(w, resp, model, uc, usage.ExtractFromGemini, extractGeminiCandidateText, func(b []byte) string {
		return fmt.Sprintf("data: %s\n\n", b)
	})
}
