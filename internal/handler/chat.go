package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/loopguard"
	"github.com/arisvia/cyrene-gateway/internal/metrics"
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
	Provider     string
	Model        string
	ConnectionID string
	APIKey       string
	Endpoint     string
	StartedAt    time.Time // upstream attempt start (metrics duration)
	Status       int       // final HTTP status returned to the client
}

// ChatCompletionRequest represents an OpenAI-compatible chat request
type ChatCompletionRequest struct {
	Model       string          `json:"model"`
	Messages    []Message       `json:"messages"`
	Stream      bool            `json:"stream,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Tools       json.RawMessage `json:"tools,omitempty"`
	ToolChoice  json.RawMessage `json:"tool_choice,omitempty"`
}

type Message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	rawBody, err := io.ReadAll(r.Body)
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

	// Check if model is disabled
	if provider.IsModelDisabled(req.Model, s.DB) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": fmt.Sprintf("model is disabled: %s", req.Model)})
		return
	}

	// Check if model string is a combo
	if combo, ok := provider.ResolveCombo(req.Model, s.DB); ok {
		s.handleComboChat(w, r, req, rawBody, combo)
		return
	}

	// Single model path
	s.handleSingleModelChat(w, r, req, rawBody)
}

// runLoopGuard analyzes messages for loop patterns and returns a hint if detected.
func runLoopGuard(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}
	// Convert handler messages to loopguard messages
	lgMsgs := make([]loopguard.Message, 0, len(messages))
	for _, m := range messages {
		lgMsg := loopguard.Message{
			Role:    m.Role,
			Content: m.Content,
		}
		if m.ToolCalls != nil {
			var tcs []loopguard.ToolCall
			if err := json.Unmarshal(m.ToolCalls, &tcs); err == nil {
				lgMsg.ToolCalls = tcs
			}
		}
		lgMsgs = append(lgMsgs, lgMsg)
	}
	result := loopguard.DetectLoop(lgMsgs)
	if result.Detected {
		slog.Warn("Loop detected in conversation", slog.String("hint", result.Hint[:min(60, len(result.Hint))]))
		return result.Hint
	}
	return ""
}

// getHTTPClient returns an HTTP client, using proxy rotation if available.
func (s *Server) getHTTPClient(timeout time.Duration) *http.Client {
	allowPrivate := false
	if s.Config != nil && s.Config.AllowPrivateNetworks {
		allowPrivate = true
	}
	if s.Proxies != nil && s.Proxies.HasProxies() {
		client := s.Proxies.GetHTTPClient(allowPrivate)
		client.Timeout = timeout
		return client
	}
	return provider.SafeHTTPClient(timeout, allowPrivate)
}

// tryRefreshToken attempts to refresh OAuth credentials if needed.
// Uses dedup lock to prevent concurrent refreshes for the same token.
// Returns true if refresh was attempted (regardless of success).
func (s *Server) tryRefreshToken(conn *model.ProviderConnection) bool {
	// GitHub Copilot: multi-step exchange (GitHub OAuth token → Copilot API token).
	// The stored access_token is the GitHub OAuth token; we exchange it for a
	// short-lived Copilot token before each request when the cached one expires.
	if conn.Provider == "github" {
		return s.tryRefreshCopilotToken(conn)
	}

	if !provider.ShouldRefresh(conn) {
		return false
	}
	oldToken := conn.Data.AccessToken
	result, err := provider.DedupRefresh(conn.Provider, oldToken, func() (*provider.RefreshResult, error) {
		return provider.RefreshCredentials(conn.Provider, conn, nil)
	})
	if err != nil {
		slog.Warn("Token refresh failed", slog.String("provider", conn.Provider), "error", err)
		if provider.IsUnrecoverableRefreshError(err) {
			conn.Data.TestStatus = "expired"
			conn.Data.LastError = err.Error()
			s.DB.UpdateConnection(conn)
		}
		return true
	}
	provider.ApplyRefreshResult(conn, result)
	s.DB.UpdateConnection(conn)
	return true
}

// tryRefreshCopilotToken exchanges the stored GitHub OAuth token for a fresh
// Copilot API token. The GitHub token is long-lived; the Copilot token is
// short-lived (~30 min) and cached in ProviderSpecificData.copilotToken.
func (s *Server) tryRefreshCopilotToken(conn *model.ProviderConnection) bool {
	psd := conn.Data.ProviderSpecificData
	if psd == nil {
		psd = make(map[string]any)
		conn.Data.ProviderSpecificData = psd
	}

	// Check if cached copilot token is still valid.
	if expAt, ok := psd["copilotTokenExpiresAt"].(string); ok {
		if t, err := time.Parse(time.RFC3339, expAt); err == nil && time.Now().Before(t.Add(-2*time.Minute)) {
			if token, ok := psd["copilotToken"].(string); ok && token != "" {
				conn.Data.AccessToken = token
				return false
			}
		}
	}

	githubToken := conn.Data.AccessToken
	if psd["githubAccessToken"] != nil {
		if gt, ok := psd["githubAccessToken"].(string); ok && gt != "" {
			githubToken = gt
		}
	}

	result, err := provider.DedupRefresh("copilot", githubToken, func() (*provider.RefreshResult, error) {
		return provider.ExchangeCopilotToken(githubToken, nil)
	})
	if err != nil {
		slog.Warn("Copilot token exchange failed", "error", err)
		return true
	}

	// Cache the copilot token; preserve the GitHub token for future exchanges.
	psd["githubAccessToken"] = githubToken
	psd["copilotToken"] = result.AccessToken
	if result.ExpiresIn > 0 {
		psd["copilotTokenExpiresAt"] = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}
	conn.Data.AccessToken = result.AccessToken
	s.DB.UpdateConnection(conn)
	return true
}

// handleComboChat processes a combo request with fallback/round-robin strategy.
func (s *Server) handleComboChat(w http.ResponseWriter, r *http.Request, req ChatCompletionRequest, rawBody []byte, combo *model.Combo) {
	// Determine strategy from combo kind or global settings
	strategy := provider.StrategyFallback
	if combo.Kind == "round-robin" {
		strategy = provider.StrategyRoundRobin
	} else if combo.Kind == "" {
		settings, err := s.DB.GetSettings()
		if err == nil && settings.ComboStrategy == "round-robin" {
			strategy = provider.StrategyRoundRobin
		}
	}

	// Get sticky limit from settings (default 1)
	stickyLimit := 1

	// Apply rotation
	models := s.Combos.GetRotatedModels(combo.Models, combo.Name, strategy, stickyLimit)

	slog.Info("Combo request",
		slog.String("combo", combo.Name),
		slog.String("strategy", string(strategy)),
		slog.Int("models", len(models)),
	)
	start := time.Now()

	// Try each model in order with fallback
	var lastStatus int
	var lastError string

	for i, modelStr := range models {
		slog.Info("Combo trying model",
			slog.Int("attempt", i+1),
			slog.Int("total", len(models)),
			slog.String("model", modelStr),
		)

		// Resolve this model string
		modelInfo, err := provider.ResolveModel(modelStr, s.DB)
		if err != nil || modelInfo.Provider == "" {
			lastError = fmt.Sprintf("cannot resolve model: %s", modelStr)
			lastStatus = 400
			continue
		}

		// Skip disabled models within combo
		if provider.IsModelDisabled(modelStr, s.DB) {
			lastError = fmt.Sprintf("model disabled: %s", modelStr)
			lastStatus = 403
			continue
		}

		providerInfo, ok := provider.GetProvider(modelInfo.Provider)
		if !ok {
			lastError = fmt.Sprintf("unknown provider: %s", modelInfo.Provider)
			lastStatus = 400
			continue
		}

		conns, err := s.DB.ListConnectionsByProvider(modelInfo.Provider)
		if err != nil || len(conns) == 0 {
			lastError = fmt.Sprintf("no credentials for provider: %s", modelInfo.Provider)
			lastStatus = 503
			continue
		}

		conn := s.selectAvailableConnection(conns, modelInfo.Model, nil)
		if conn == nil {
			lastError = fmt.Sprintf("all accounts rate-limited for: %s", modelInfo.Provider)
			lastStatus = 503
			continue
		}

		// Phase 9: Pre-check OAuth token refresh
		s.tryRefreshToken(conn)

		baseURL, comboAPIType := providerInfo.EffectiveBaseURL(conn.AuthType, conn.Data.APIKey != "")
		if conn.Data.BaseURL != "" {
			baseURL = conn.Data.BaseURL
			comboAPIType = providerInfo.APIType
		}
		if baseURL == "" {
			lastError = fmt.Sprintf("no base URL for provider: %s", modelInfo.Provider)
			lastStatus = 503
			continue
		}

		// Qoder 需要专属 COSY 签名执行器；通用 transport 未签名会被上游 RST
		if modelInfo.Provider == "qoder" {
			slog.Info("Combo delegating to Qoder executor", slog.String("model", modelInfo.Model))
			s.handleQoderChat(w, r, req, rawBody, modelInfo, conn, providerInfo)
			return
		}

		// Build and execute upstream request — use raw body to preserve unknown fields
		var comboBody map[string]any
		json.Unmarshal(rawBody, &comboBody)
		comboBody["model"] = modelInfo.Model
		bodyBytes, err := json.Marshal(comboBody)
		if err != nil {
			lastError = "failed to marshal request"
			lastStatus = 500
			continue
		}

		comboTransport := provider.ResolveTransport(providerInfo, baseURL, comboAPIType, conn)
		targetURL := provider.BuildTransportURL(comboTransport, modelInfo.Model, req.Stream)
		upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(bodyBytes))
		if err != nil {
			lastError = "failed to create upstream request"
			lastStatus = 500
			continue
		}

		for k, v := range comboTransport.Headers {
			upstreamReq.Header.Set(k, v)
		}
		comboCreds := provider.Credentials{
			APIKey:               conn.Data.APIKey,
			AccessToken:          conn.Data.AccessToken,
			ProviderSpecificData: conn.Data.ProviderSpecificData,
		}
		provider.ApplyAuth(upstreamReq, comboTransport, comboCreds)
		if providerInfo.NoAuth && comboCreds.APIKey == "" && comboCreds.AccessToken == "" {
			upstreamReq.Header.Set("Authorization", "Bearer public")
		}
		upstreamReq.Header.Set("Content-Type", "application/json")
		if req.Stream {
			upstreamReq.Header.Set("Accept", "text/event-stream")
		}

		client := s.getHTTPClient(5 * time.Minute)
		resp, err := client.Do(upstreamReq)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) {
				slog.Info("Client canceled combo request", slog.String("model", modelStr))
				return
			}
			lastError = fmt.Sprintf("upstream request failed: %v", err)
			lastStatus = 502
			slog.Warn("Combo model failed", slog.String("model", modelStr), "error", err)
			provider.ApplyErrorState(conn, 502, err.Error())
			s.DB.UpdateConnection(conn)
			continue
		}

		// Success (2xx) - stream response to client
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			slog.Info("Combo model succeeded", slog.String("model", modelStr))
			provider.ResetAccountState(conn)
			s.DB.UpdateConnection(conn)
			uc := &usageContext{
				Provider:     modelInfo.Provider,
				Model:        modelInfo.Model,
				ConnectionID: conn.ID,
				APIKey:       extractRequestAPIKey(r),
				Endpoint:     "/v1/chat/completions",
				StartedAt:    start,
				Status:       resp.StatusCode,
			}
			s.proxyResponse(w, r, resp, req.Stream, translator.FormatOpenAI, modelInfo.Model, uc)
			resp.Body.Close()
			return
		}

		// Error response - read body for error classification
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		lastStatus = resp.StatusCode
		lastError = string(errBody)

		// Apply and persist error state for this connection
		provider.ApplyErrorState(conn, resp.StatusCode, string(errBody))
		s.DB.UpdateConnection(conn)

		// Check if should fallback
		fallbackResult := provider.CheckFallbackError(resp.StatusCode, string(errBody), conn.Data.BackoffLevel)
		if !fallbackResult.ShouldFallback {
			// Non-fallbackable error, return immediately
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.StatusCode)
			w.Write(errBody)
			return
		}

		slog.Warn("Combo model failed, trying next",
			slog.String("model", modelStr),
			slog.Int("status", resp.StatusCode),
		)
	}
	if lastStatus == 0 {
		lastStatus = 503
	}
	if lastError == "" {
		lastError = "all combo models unavailable"
	}
	writeJSON(w, lastStatus, map[string]string{"error": lastError})
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

	// Auto-provision a connection for NoAuth providers (e.g. OpenCode Free, MiMo Free)
	if len(conns) == 0 && providerInfo.NoAuth {
		conn := s.autoProvisionNoAuthConnection(providerInfo)
		if conn != nil {
			conns = []model.ProviderConnection{*conn}
		}
	}

	if len(conns) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("no active credentials for provider: %s", modelInfo.Provider),
		})
		return
	}

	// Select best available connection (priority-ordered, cooldown-aware, model-lock-aware)
	conn := s.selectAvailableConnection(conns, modelInfo.Model, nil)
	if conn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("all accounts rate-limited for provider: %s", modelInfo.Provider),
		})
		return
	}

	// Phase 9: Pre-check OAuth token refresh
	s.tryRefreshToken(conn)

	// Determine base URL (auth-mode-aware: 9router#2881)
	baseURL, effectiveAPIType := providerInfo.EffectiveBaseURL(conn.AuthType, conn.Data.APIKey != "")
	if conn.Data.BaseURL != "" {
		baseURL = conn.Data.BaseURL
		effectiveAPIType = providerInfo.APIType // user override resets to provider default
	}
	if baseURL == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("no base URL configured for provider: %s", modelInfo.Provider),
		})
		return
	}

	// Determine target format and build upstream request
	targetFormat := translator.FormatOpenAI
	switch effectiveAPIType {
	case "anthropic":
		targetFormat = translator.FormatAnthropic
	case "gemini":
		targetFormat = translator.FormatGemini
	}

	// Phase 30: resolve the provider transport (base URL, format, auth scheme,
	// hooks) once and use it for both URL building and auth injection.
	transport := provider.ResolveTransport(providerInfo, baseURL, effectiveAPIType, conn)

	// --- Qoder special path: COSY-signed custom protocol ---
	if modelInfo.Provider == "qoder" {
		s.handleQoderChat(w, r, req, rawBody, modelInfo, conn, providerInfo)
		return
	}

	var bodyBytes []byte
	var targetURL string

	if targetFormat == translator.FormatOpenAI {
		// Standard OpenAI-compatible passthrough — use raw body to preserve
		// unknown fields (service_tier, top_p, stop, etc.) per 9router c97963c.
		var bodyMap map[string]any
		json.Unmarshal(rawBody, &bodyMap)
		bodyMap["model"] = modelInfo.Model

		// Loop guard detection
		if loopHint := runLoopGuard(req.Messages); loopHint != "" {
			loopguard.InjectLoopHint(bodyMap, "openai", loopHint)
		}

		// Termination prompt injection (only when tools are present)
		if req.Tools != nil {
			loopguard.InjectTerminationPrompt(bodyMap, "openai")
		}

		// Max_tokens clamping for specific providers
		provider.ClampMaxTokens(modelInfo.Provider, modelInfo.Model, bodyMap)

		// Phase 13: Token saver (RTK + caveman + ponytail)
		s.applyTokenSaver(bodyMap, "openai")

		bodyBytes, err = json.Marshal(bodyMap)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to marshal request"})
			return
		}
		targetURL = provider.BuildTransportURL(transport, modelInfo.Model, req.Stream)
	} else {
		// Translate request to provider format — use raw body to preserve unknown fields
		var bodyMap map[string]any
		json.Unmarshal(rawBody, &bodyMap)

		// Phase 9: Loop guard + termination prompt for translated formats
		if loopHint := runLoopGuard(req.Messages); loopHint != "" {
			loopguard.InjectLoopHint(bodyMap, string(targetFormat), loopHint)
		}
		if req.Tools != nil {
			loopguard.InjectTerminationPrompt(bodyMap, string(targetFormat))
		}

		translated, err := translator.TranslateRequest(targetFormat, modelInfo.Model, bodyMap, req.Stream)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("translation failed: %v", err)})
			return
		}

		// Max_tokens clamping on translated body
		provider.ClampMaxTokens(modelInfo.Provider, modelInfo.Model, translated)

		// Phase 13: Token saver (RTK + caveman + ponytail)
		s.applyTokenSaver(translated, string(targetFormat))

		bodyBytes, err = json.Marshal(translated)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to marshal translated request"})
			return
		}

		targetURL = provider.BuildTransportURL(transport, modelInfo.Model, req.Stream)
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upstream request"})
		return
	}

	// Phase 30: apply transport headers + auth (scheme-aware injection + hooks).
	// Headers first (matches 9router: config.headers spread before applyAuth) so
	// the AnthropicVersion guard in ApplyAuth sees any pre-set anthropic-version.
	for k, v := range transport.Headers {
		upstreamReq.Header.Set(k, v)
	}
	creds := provider.Credentials{
		APIKey:               conn.Data.APIKey,
		AccessToken:          conn.Data.AccessToken,
		ProviderSpecificData: conn.Data.ProviderSpecificData,
	}
	provider.ApplyAuth(upstreamReq, transport, creds)
	if providerInfo.NoAuth && creds.APIKey == "" && creds.AccessToken == "" {
		// NoAuth providers (e.g. OpenCode Free) use a public bearer token
		upstreamReq.Header.Set("Authorization", "Bearer public")
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	if req.Stream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	}

	slog.Info("Proxying request",
		slog.String("model", modelInfo.Model),
		slog.String("provider", modelInfo.Provider),
		slog.String("format", string(targetFormat)),
		slog.String("connection", conn.ID),
		slog.Bool("stream", req.Stream),
	)

	// Phase 9: Use proxy-aware HTTP client
	start := time.Now()
	client := s.getHTTPClient(5 * time.Minute)
	resp, err := client.Do(upstreamReq)
	if err != nil {
		slog.Error("Upstream request failed", "error", err, "provider", modelInfo.Provider)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream request failed"})
		return
	}
	defer resp.Body.Close()

	// Phase 9: On-401 retry with token refresh
	if resp.StatusCode == http.StatusUnauthorized && conn.Data.RefreshToken != "" {
		io.ReadAll(resp.Body)
		resp.Body.Close()

		result, refreshErr := provider.RefreshCredentials(conn.Provider, conn, nil)
		if refreshErr == nil {
			provider.ApplyRefreshResult(conn, result)
			s.DB.UpdateConnection(conn)

			// Retry the request with new token
			retryReq, retryErr := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(bodyBytes))
			if retryErr == nil {
				for k, v := range transport.Headers {
					retryReq.Header.Set(k, v)
				}
				retryCreds := provider.Credentials{
					APIKey:               conn.Data.APIKey,
					AccessToken:          conn.Data.AccessToken,
					ProviderSpecificData: conn.Data.ProviderSpecificData,
				}
				provider.ApplyAuth(retryReq, transport, retryCreds)
				retryReq.Header.Set("Content-Type", "application/json")
				if req.Stream {
					retryReq.Header.Set("Accept", "text/event-stream")
				}

				slog.Info("Retrying after token refresh", slog.String("provider", modelInfo.Provider))
				resp, err = client.Do(retryReq)
				if err != nil {
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
		provider.ApplyErrorState(conn, resp.StatusCode, string(errBody))
		s.DB.UpdateConnection(conn)
		if s.Metrics != nil {
			s.Metrics.ObserveRequest(modelInfo.Provider, modelInfo.Model, "/v1/chat/completions", resp.StatusCode, time.Since(start).Seconds(), nil)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		// Wrap error with actionable hint (Phase 26 Error UX)
		hinted := wrapErrorWithHint(resp.StatusCode, errBody, modelInfo.Provider)
		w.Write(hinted)
		return
	}

	// Reset error state on success
	provider.ResetAccountState(conn)
	s.DB.UpdateConnection(conn)

	// Proxy the response with format translation
	uc := &usageContext{
		Provider:     modelInfo.Provider,
		Model:        modelInfo.Model,
		ConnectionID: conn.ID,
		APIKey:       extractRequestAPIKey(r),
		Endpoint:     "/v1/chat/completions",
		StartedAt:    start,
		Status:       resp.StatusCode,
	}
	s.proxyResponse(w, r, resp, req.Stream, targetFormat, modelInfo.Model, uc)
}

// proxyResponse handles streaming and non-streaming response proxying with format translation.
func (s *Server) proxyResponse(w http.ResponseWriter, r *http.Request, resp *http.Response, stream bool, format translator.Format, model string, uc *usageContext) {
	if !stream {
		s.proxyNonStreaming(w, resp, format, model, uc)
		return
	}
	s.proxyStreaming(w, r, resp, format, model, uc)
}

// proxyNonStreaming reads the full response, translates if needed, and writes it.
func (s *Server) proxyNonStreaming(w http.ResponseWriter, resp *http.Response, format translator.Format, model string, uc *usageContext) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to read upstream response"})
		return
	}

	// Extract usage before translation
	var u usage.Usage
	switch format {
	case translator.FormatAnthropic:
		u = usage.ExtractFromClaude(body)
	case translator.FormatGemini:
		u = usage.ExtractFromGemini(body)
	default:
		u = usage.ExtractFromOpenAI(body)
	}

	if u.TotalTokens > 0 {
		slog.Info("Usage extracted",
			slog.String("model", model),
			slog.Int("prompt_tokens", u.PromptTokens),
			slog.Int("completion_tokens", u.CompletionTokens),
		)
		s.recordUsage(uc, u)
	}

	// Translate response to OpenAI format if needed
	if format != translator.FormatOpenAI {
		translated, err := translator.TranslateResponse(format, body, model)
		if err != nil {
			// Fallback to raw response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.StatusCode)
			w.Write(body)
			return
		}
		body = translated
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

// proxyStreaming handles SSE streaming with disconnect awareness and [DONE] handling.
func (s *Server) proxyStreaming(w http.ResponseWriter, r *http.Request, resp *http.Response, format translator.Format, model string, uc *usageContext) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		// Fallback: read all and write
		io.Copy(w, resp.Body)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	reader := provider.NewSSEReader(resp.Body)
	var lastUsage usage.Usage

	for {
		select {
		case <-ctx.Done():
			slog.Info("Client disconnected during stream", slog.String("model", model))
			if lastUsage.TotalTokens > 0 {
				s.recordUsage(uc, lastUsage)
			}
			return
		default:
		}

		event, err := reader.ReadEvent(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				// Normal stream completion without explicit [DONE]
				if lastUsage.TotalTokens > 0 {
					s.recordUsage(uc, lastUsage)
				}
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}
			if errors.Is(err, context.Canceled) {
				return
			}
			// Mid-stream read error (network dropped or corrupted event)
			slog.Warn("Upstream SSE read error", slog.String("model", model), slog.String("error", err.Error()))
			// Do NOT synthesize clean [DONE] on real upstream error
			return
		}

		if len(event.Data) == 0 {
			continue
		}

		dataStr := strings.TrimSpace(string(event.Data))
		if dataStr == "[DONE]" {
			if lastUsage.TotalTokens > 0 {
				s.recordUsage(uc, lastUsage)
			}
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		}

		// Extract usage from SSE chunk
		if u := usage.ExtractFromSSELine(event.Data); u.TotalTokens > 0 {
			lastUsage = u
		}

		// Translate SSE chunks if needed
		if format != translator.FormatOpenAI {
			translated, isDone, err := translator.TranslateSSEChunk(format, event.Data, model)
			if err != nil || translated == nil {
				continue
			}
			if isDone {
				if lastUsage.TotalTokens > 0 {
					s.recordUsage(uc, lastUsage)
				}
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", translated)
		} else {
			// OpenAI format passthrough
			fmt.Fprintf(w, "data: %s\n\n", dataStr)
		}
		flusher.Flush()
	}
}

// handleMessages implements the Anthropic-compatible /v1/messages passthrough endpoint.
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	// Read the raw body for passthrough
	bodyBytes, err := io.ReadAll(r.Body)
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

	stream, _ := reqBody["stream"].(bool)

	// Resolve model
	modelInfo, err := provider.ResolveModel(modelStr, s.DB)
	if err != nil || modelInfo.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("cannot resolve model: %s", modelStr)})
		return
	}

	providerInfo, ok := provider.GetProvider(modelInfo.Provider)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider: %s", modelInfo.Provider)})
		return
	}

	conns, err := s.DB.ListConnectionsByProvider(modelInfo.Provider)
	if err != nil || len(conns) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("no active credentials for provider: %s", modelInfo.Provider),
		})
		return
	}

	conn := s.selectAvailableConnection(conns, modelInfo.Model, nil)
	if conn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("all accounts rate-limited for provider: %s", modelInfo.Provider),
		})
		return
	}

	baseURL, effectiveAPIType := providerInfo.EffectiveBaseURL(conn.AuthType, conn.Data.APIKey != "")
	if conn.Data.BaseURL != "" {
		baseURL = conn.Data.BaseURL
		effectiveAPIType = providerInfo.APIType
	}

	// Set the resolved model name
	reqBody["model"] = modelInfo.Model
	translatedBody, _ := json.Marshal(reqBody)

	// Phase 30: resolve transport for URL building + auth injection.
	transport := provider.ResolveTransport(providerInfo, baseURL, effectiveAPIType, conn)
	targetURL := provider.BuildTransportURL(transport, modelInfo.Model, stream)
	if targetURL == "" {
		// Fallback for providers without a usable base URL.
		switch effectiveAPIType {
		case "anthropic":
			targetURL = strings.TrimRight(baseURL, "/") + "/v1/messages"
		default:
			targetURL = strings.TrimRight(baseURL, "/") + "/chat/completions"
		}
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(translatedBody))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upstream request"})
		return
	}

	for k, v := range transport.Headers {
		upstreamReq.Header.Set(k, v)
	}
	creds := provider.Credentials{
		APIKey:               conn.Data.APIKey,
		AccessToken:          conn.Data.AccessToken,
		ProviderSpecificData: conn.Data.ProviderSpecificData,
	}
	provider.ApplyAuth(upstreamReq, transport, creds)
	upstreamReq.Header.Set("Content-Type", "application/json")

	slog.Info("Messages passthrough",
		slog.String("model", modelInfo.Model),
		slog.String("provider", modelInfo.Provider),
		slog.Bool("stream", stream),
	)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream request failed"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		provider.ApplyErrorState(conn, resp.StatusCode, string(errBody))
		s.DB.UpdateConnection(conn)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(errBody)
		return
	}

	provider.ResetAccountState(conn)
	s.DB.UpdateConnection(conn)

	uc := &usageContext{
		Provider:     modelInfo.Provider,
		Model:        modelInfo.Model,
		ConnectionID: conn.ID,
		APIKey:       extractRequestAPIKey(r),
		Endpoint:     "/v1/messages",
	}

	// For Anthropic passthrough, stream directly without translation
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			io.Copy(w, resp.Body)
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var totalUsage usage.Usage
		ctx := r.Context()

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			line := scanner.Bytes()
			if after, ok0 := bytes.CutPrefix(line, []byte("data: ")); ok0 {
				data := after
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
			fmt.Fprintf(w, "%s\n", line)
			flusher.Flush()
		}
		if totalUsage.TotalTokens > 0 {
			s.recordUsage(uc, totalUsage)
		}
	} else {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to read upstream response"})
			return
		}
		u := usage.ExtractFromClaude(respBody)
		if u.TotalTokens > 0 {
			s.recordUsage(uc, u)
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
func (s *Server) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string          `json:"model"`
		Input json.RawMessage `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	modelInfo, err := provider.ResolveModel(req.Model, s.DB)
	if err != nil || modelInfo.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("cannot resolve model: %s", req.Model)})
		return
	}

	providerInfo, ok := provider.GetProvider(modelInfo.Provider)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider: %s", modelInfo.Provider)})
		return
	}

	conns, err := s.DB.ListConnectionsByProvider(modelInfo.Provider)
	if err != nil || len(conns) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("no active credentials for provider: %s", modelInfo.Provider),
		})
		return
	}

	conn := s.selectAvailableConnection(conns, modelInfo.Model, nil)
	if conn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "all accounts rate-limited"})
		return
	}

	baseURL := providerInfo.BaseURL
	if conn.Data.BaseURL != "" {
		baseURL = conn.Data.BaseURL
	}

	// Forward with resolved model name
	req.Model = modelInfo.Model
	bodyBytes, _ := json.Marshal(req)

	targetURL := strings.TrimRight(baseURL, "/") + "/embeddings"
	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upstream request"})
		return
	}

	if conn.Data.APIKey != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+conn.Data.APIKey)
	}
	upstreamReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream request failed"})
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// selectAvailableConnection picks the best connection using priority, cooldown, model locks, and quota.
func (s *Server) selectAvailableConnection(conns []model.ProviderConnection, modelName string, excludeIDs map[string]bool) *model.ProviderConnection {
	return provider.SelectCredentialWithQuota(conns, modelName, excludeIDs, s.quotaChecker())
}

// applyTokenSaver applies RTK compression, caveman, and ponytail based on settings.
func (s *Server) applyTokenSaver(bodyMap map[string]any, format string) {
	settings, err := s.DB.GetSettings()
	if err != nil {
		return
	}

	// Check per-provider exclusion (9router#2767)
	providerID, _ := bodyMap["model"].(string)
	if providerID != "" {
		if idx := strings.Index(providerID, "/"); idx > 0 {
			providerID = providerID[:idx]
		}
	}
	if slices.Contains(settings.TokenSaverExclude, providerID) {
		return
	}

	// RTK compression of tool results
	if settings.RTKEnabled {
		if saved := rtk.CompressMessages(bodyMap, true); saved > 0 {
			slog.Debug("RTK compressed tool results", slog.Int("bytes_saved", saved))
		}
	}

	// Caveman: inject terse-style system prompt
	if settings.CavemanEnabled && settings.CavemanLevel != "" {
		rtk.InjectCaveman(bodyMap, format, settings.CavemanLevel)
	}

	// Ponytail: inject lazy-senior-dev system prompt
	if settings.PonytailEnabled && settings.PonytailLevel != "" {
		rtk.InjectPonytail(bodyMap, format, settings.PonytailLevel)
	}
}

// recordUsage persists a usage entry to the database and publishes a real-time event.
func (s *Server) recordUsage(uc *usageContext, u usage.Usage) {
	if uc == nil || u.TotalTokens == 0 {
		return
	}
	if s.Metrics != nil {
		dur := time.Since(uc.StartedAt).Seconds()
		if dur < 0 {
			dur = 0
		}
		s.Metrics.ObserveRequest(uc.Provider, uc.Model, uc.Endpoint, uc.Status, dur, &metrics.Usage{
			PromptTokens:     u.PromptTokens,
			CompletionTokens: u.CompletionTokens,
			CachedTokens:     u.CachedTokens,
			ReasoningTokens:  u.ReasoningTokens,
		})
	}
	entry := &db.UsageEntry{
		Provider:         uc.Provider,
		Model:            uc.Model,
		ConnectionID:     uc.ConnectionID,
		APIKey:           uc.APIKey,
		Endpoint:         uc.Endpoint,
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		Status:           "ok",
	}
	// Store cache/reasoning metadata (9router#2873)
	if u.CachedTokens > 0 || u.ReasoningTokens > 0 {
		meta := map[string]int{}
		if u.CachedTokens > 0 {
			meta["cached_tokens"] = u.CachedTokens
		}
		if u.ReasoningTokens > 0 {
			meta["reasoning_tokens"] = u.ReasoningTokens
		}
		b, _ := json.Marshal(meta)
		entry.Meta = string(b)
	}
	if err := s.DB.SaveUsageEntry(entry); err != nil {
		slog.Warn("Failed to record usage", "error", err, "model", uc.Model)
	}

	// Publish real-time event for SSE stream
	if s.Events != nil {
		s.Events.Publish(RequestEvent{
			Type:      "request",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Provider:  uc.Provider,
			Model:     uc.Model,
			Status:    "ok",
			Prompt:    u.PromptTokens,
			Compl:     u.CompletionTokens,
			Endpoint:  uc.Endpoint,
		})
	}
}

// extractRequestAPIKey extracts the API key from the request Authorization header.
func extractRequestAPIKey(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return after
	}
	if key := r.Header.Get("x-api-key"); key != "" {
		return key
	}
	return ""
}
