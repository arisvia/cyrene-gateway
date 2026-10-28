package handler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/cache"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/provider"
	"github.com/arisvia/cyrene-gateway/internal/rtk"
	"github.com/arisvia/cyrene-gateway/internal/translator"
	"github.com/arisvia/cyrene-gateway/internal/usage"
)

// Literal keys used to gate JSON decoding on the SSE passthrough path. Most
// streamed lines carry neither, so a byte scan avoids two full unmarshals per
// line without changing which lines get parsed.
var (
	usageKey     = []byte(`"usage"`)
	deltaKey     = []byte(`"delta"`)
	newlineBytes = []byte("\n")
)

// handleMessages implements the Anthropic-compatible /v1/messages passthrough endpoint.
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	// Read the raw body for passthrough
	bodyBytes, err := readRequestBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	var reqBody map[string]any
	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	modelStr, _ := reqBody["model"].(string)
	if modelStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing model"})
		return
	}

	keyObj := auth.APIKeyFromContext(r.Context())
	if keyObj != nil && !keyObj.IsModelAllowed(modelStr) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": fmt.Sprintf("model '%s' is not allowed for this API key", modelStr),
		})
		return
	}

	// Inject per-key system prompt context pre-cache
	if keyObj != nil && keyObj.SystemPrompt != "" {
		rtk.InjectSystemPrompt(reqBody, "anthropic", keyObj.SystemPrompt)
		if updated, err := json.Marshal(reqBody); err == nil {
			bodyBytes = updated
		}
	}
	stream, _ := reqBody["stream"].(bool)
	// If model is a combo, route via Chat Completions adapter
	if _, isCombo := provider.ResolveCombo(modelStr, s.DB); isCombo {
		s.handleMessagesViaChat(w, r, reqBody, modelStr, stream)
		return
	}

	// Resolve model
	modelInfo, err := provider.ResolveModel(modelStr, s.DB)
	if err != nil || modelInfo.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("cannot resolve model: %s", modelStr)})
		return
	}
	if provider.IsModelDisabled(modelStr, s.DB) || provider.IsModelDisabled(modelInfo.Provider+"/"+modelInfo.Model, s.DB) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": fmt.Sprintf("model is disabled: %s", modelStr)})
		return
	}
	// Phase: Response Cache check
	var settings *db.Settings
	if s.DB != nil {
		settings, _ = s.DB.GetSettings()
	}
	cacheEnabled := settings != nil && settings.ResponseCacheEnabled && s.Cache != nil
	var cacheKey string
	if cacheEnabled && !cache.ShouldBypass(r.Header) {
		eligible, key, err := cache.IsEligible("messages", bodyBytes, settings.ResponseCacheAll)
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

	providerInfo, ok := provider.GetProvider(modelInfo.Provider)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider: %s", modelInfo.Provider)})
		return
	}
	// If the provider does not support native Anthropic Messages API (e.g. Qoder, Antigravity, OpenAI, DeepSeek),
	// delegate directly to handleMessagesViaChat BEFORE selecting a connection.
	// Calling selectAvailableConnection here and then again in handleSingleModelChat advances
	// the round-robin counter twice per request, collapsing equal-priority load balancing.
	//
	// Resolve the affinity key from the Anthropic body and propagate it via the
	// request header: the via-chat delegation translates the body and drops
	// metadata.user_id, so the header is the only carrier for both branches.
	sessionKey := r.Header.Get("X-Cyrene-Session-ID")
	if sessionKey == "" {
		sessionKey = provider.SessionKeyFromBodyMap(reqBody)
		if sessionKey != "" {
			r.Header.Set("X-Cyrene-Session-ID", sessionKey)
		}
	}
	if providerInfo.APIType != "anthropic" || modelInfo.Provider == "qoder" || modelInfo.Provider == "qoder-cn" || modelInfo.Provider == "antigravity" {
		s.handleMessagesViaChat(w, r, reqBody, modelStr, stream)
		return
	}

	conns, err := s.DB.ListConnectionsByProvider(modelInfo.Provider)
	if err != nil || len(conns) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("no active credentials for provider: %s", modelInfo.Provider),
		})
		return
	}

	conn := s.selectAvailableConnection(conns, modelInfo.Model, nil, sessionKey)
	if conn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("all accounts rate-limited for provider: %s", modelInfo.Provider),
		})
		return
	}

	// Ensure fresh credentials for OAuth / Copilot connections
	s.tryRefreshToken(conn)

	baseURL, effectiveAPIType := providerInfo.EffectiveBaseURL(conn.AuthType, conn.Data.APIKey != "")
	if conn.Data.BaseURL != "" {
		baseURL = conn.Data.BaseURL
		if effectiveAPIType == "" {
			effectiveAPIType = providerInfo.APIType
		}
	}
	if effectiveAPIType != "anthropic" || baseURL == "" {
		s.handleMessagesViaChat(w, r, reqBody, modelStr, stream)
		return
	}
	// Set the resolved model name
	reqBody["model"] = modelInfo.Model

	// Strip output_config.format for non-official Anthropic endpoints to prevent 400 Bad Request
	// (Claude Code sends output_config.format json_schema for session titles which third-party gateways reject)
	if modelInfo.Provider != "anthropic" && modelInfo.Provider != "claude" && modelInfo.Provider != "cc" {
		if oc, ok := reqBody["output_config"].(map[string]any); ok {
			delete(oc, "format")
			if len(oc) == 0 {
				delete(reqBody, "output_config")
			}
		}
	}

	s.applyTokenSaver(reqBody, "anthropic", modelInfo.Provider, r)
	s.applyLoopGuard(reqBody, bodyBytes, "anthropic", reqBody["tools"] != nil)
	if s.autoPromptCacheEnabled() {
		translator.AutoInjectClaudeCacheControl(reqBody, true)
	}
	translatedBody, _ := json.Marshal(reqBody)

	// Phase 30: resolve transport for URL building + auth injection.
	transport := provider.ResolveTransport(providerInfo, baseURL, effectiveAPIType, conn)
	targetURL := provider.BuildTransportURL(transport, modelInfo.Model, stream)
	if targetURL == "" {
		targetURL = strings.TrimRight(baseURL, "/") + "/v1/messages"
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(translatedBody))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upstream request"})
		return
	}

	for k, v := range transport.Headers {
		upstreamReq.Header.Set(k, v)
	}
	// Forward Anthropic-specific client headers if present
	if v := r.Header.Get("anthropic-version"); v != "" && upstreamReq.Header.Get("anthropic-version") == "" {
		upstreamReq.Header.Set("anthropic-version", v)
	}
	if v := r.Header.Get("anthropic-beta"); v != "" && upstreamReq.Header.Get("anthropic-beta") == "" {
		upstreamReq.Header.Set("anthropic-beta", v)
	}
	if stream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	}
	creds := provider.ResolveCredentials(conn, modelInfo.Provider, modelInfo.Model)
	provider.ApplyAuth(upstreamReq, transport, creds)
	upstreamReq.Header.Set("Content-Type", "application/json")
	slog.Info("Messages passthrough",
		slog.String("model", modelInfo.Model),
		slog.String("provider", modelInfo.Provider),
		slog.Bool("stream", stream),
	)
	start := time.Now()
	endpoint := resolveRequestEndpoint(r, "/v1/messages")
	s.publishRequestEvent(r, RequestEvent{Type: "routing", Provider: modelInfo.Provider, Model: modelInfo.Model, Endpoint: endpoint, Status: "routing"})

	clientTimeout := 5 * time.Minute
	if stream {
		clientTimeout = 0
	}
	client := s.getHTTPClient(clientTimeout)
	resp, err := client.Do(upstreamReq)
	if err != nil {
		slog.Error("Messages upstream request failed", "error", err, "provider", modelInfo.Provider, "url", targetURL)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("upstream request failed: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		slog.Warn("Messages upstream error response", "status", resp.StatusCode, "provider", modelInfo.Provider, "body", string(errBody))
		provider.ApplyErrorState(conn, resp.StatusCode, string(errBody))
		s.DB.UpdateConnectionHealth(conn.ID, conn.Data)

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
			"input":        extractAnthropicPrompt(reqBody),
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
		w.Write(wrapErrorWithHint(resp.StatusCode, errBody, modelInfo.Provider))
		return
	}

	provider.ResetAccountState(conn)
	s.DB.UpdateConnectionHealth(conn.ID, conn.Data)

	prompt := extractAnthropicPrompt(reqBody)
	uc := &usageContext{
		RequestID:    ensureRequestID(r),
		Provider:     modelInfo.Provider,
		Model:        modelInfo.Model,
		ConnectionID: conn.ID,
		APIKey:       extractRequestAPIKey(r),
		Endpoint:     endpoint,
		StartedAt:    start,
		Status:       resp.StatusCode,
		Prompt:       prompt,
	}
	// For Anthropic passthrough, stream directly without translation
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			io.Copy(w, resp.Body)
			return
		}
		flusher.Flush()

		stallReader := provider.NewStallTimeoutReader(resp.Body, provider.DefaultStreamStallTimeout)
		defer stallReader.Close()
		scanner := bufio.NewScanner(stallReader)
		scanner.Buffer(make([]byte, 0, 64*1024), maxSSELineBytes)
		var totalUsage usage.Usage
		var outBuilder strings.Builder
		var totalStreamChars int
		ctx := r.Context()

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				if uc != nil {
					uc.Status = 499
					uc.Response = outBuilder.String()
				}
				if totalUsage.TotalTokens > 0 {
					s.recordUsage(uc, totalUsage)
				} else {
					estimatedPrompt := len(translatedBody) / 4
					if estimatedPrompt < 1 {
						estimatedPrompt = 1
					}
					complTokens := totalStreamChars / 4
					if complTokens < 1 && outBuilder.Len() > 0 {
						complTokens = outBuilder.Len() / 4
					}
					s.recordUsage(uc, usage.Usage{
						PromptTokens:     estimatedPrompt,
						CompletionTokens: complTokens,
						TotalTokens:      estimatedPrompt + complTokens,
					})
				}
				return
			default:
			}
			line := scanner.Bytes()
			if data, _, ok0 := translator.ParseSSEDataLine(line); ok0 {
				// Most lines are content_block_delta and carry no usage at all.
				// Parsing every line twice was the dominant cost of this loop on
				// long streams. Both keys are literal in the payload, so a byte
				// scan gates the full unmarshal without changing behaviour.
				if bytes.Contains(data, usageKey) {
					if u := usage.ExtractFromClaudeSSE(data); u.TotalTokens > 0 {
						if u.PromptTokens > 0 {
							totalUsage.PromptTokens = u.PromptTokens
						}
						if u.CompletionTokens > 0 {
							totalUsage.CompletionTokens = u.CompletionTokens
						}
						if u.CachedTokens > 0 {
							totalUsage.CachedTokens = u.CachedTokens
						}
						totalUsage.TotalTokens = totalUsage.PromptTokens + totalUsage.CompletionTokens
					}
				}
				if bytes.Contains(data, deltaKey) {
					var ev struct {
						Type  string `json:"type"`
						Delta struct {
							Type string `json:"type"`
							Text string `json:"text"`
						} `json:"delta"`
					}
					if err := json.Unmarshal(data, &ev); err == nil && ev.Delta.Text != "" {
						totalStreamChars += len(ev.Delta.Text)
						if outBuilder.Len() < 16384 {
							outBuilder.WriteString(ev.Delta.Text)
						}
					}
				}
			}
			w.Write(line)
			w.Write(newlineBytes)
			flusher.Flush()
		}
		if scanErr := scanner.Err(); scanErr != nil {
			slog.Warn("Messages passthrough SSE scan error", "error", scanErr, "model", modelInfo.Model)
			// Without a terminal frame the client only sees the stream stop and
			// cannot distinguish truncation from a clean end.
			if uc != nil {
				uc.Status = http.StatusBadGateway
			}
			fmt.Fprintf(w, "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"api_error\",\"message\":%q}}\n\n", scanErr.Error())
			flusher.Flush()
		}
		uc.Response = outBuilder.String()
		if totalUsage.TotalTokens > 0 {
			s.recordUsage(uc, totalUsage)
		} else if outBuilder.Len() > 0 || totalStreamChars > 0 {
			estimatedPrompt := len(translatedBody) / 4
			if estimatedPrompt < 1 {
				estimatedPrompt = 1
			}
			complTokens := totalStreamChars / 4
			if complTokens < 1 {
				complTokens = len(outBuilder.String()) / 4
			}
			if complTokens < 1 {
				complTokens = 1
			}
			fallbackUsage := usage.Usage{
				PromptTokens:     estimatedPrompt,
				CompletionTokens: complTokens,
				TotalTokens:      estimatedPrompt + complTokens,
			}
			s.recordUsage(uc, fallbackUsage)
		}
	} else {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to read upstream response"})
			return
		}
		var anthropicResp struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(respBody, &anthropicResp); err == nil {
			var sb strings.Builder
			for _, c := range anthropicResp.Content {
				if c.Type == "text" || c.Text != "" {
					if sb.Len() > 0 {
						sb.WriteString("\n")
					}
					sb.WriteString(c.Text)
				}
			}
			uc.Response = sb.String()
		}
		u := usage.ExtractFromClaude(respBody)
		if u.TotalTokens > 0 {
			s.recordUsage(uc, u)
		} else if uc.Response != "" {
			estimatedPrompt := len(translatedBody) / 4
			if estimatedPrompt < 1 {
				estimatedPrompt = 1
			}
			fallbackUsage := usage.Usage{
				PromptTokens:     estimatedPrompt,
				CompletionTokens: len(uc.Response) / 4,
				TotalTokens:      estimatedPrompt + len(uc.Response)/4,
			}
			s.recordUsage(uc, fallbackUsage)
		}
		// Copy headers and body (strip hop-by-hop)
		for key, values := range resp.Header {
			if isHopByHopHeader(key) {
				continue
			}
			for _, v := range values {
				w.Header().Add(key, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(respBody)
	}
}
