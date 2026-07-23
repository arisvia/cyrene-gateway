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

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/loopguard"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
	"github.com/arisvia/cyrene-gateway/internal/translator"
	"github.com/arisvia/cyrene-gateway/internal/usage"
)

// usageContext carries metadata for recording usage after a proxied response.
type usageContext struct {
	Provider     string
	Model        string
	ConnectionID string
	APIKey       string
	Endpoint     string
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
	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
		s.handleComboChat(w, r, req, combo)
		return
	}

	// Single model path
	s.handleSingleModelChat(w, r, req)
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
	if s.Proxies != nil && s.Proxies.HasProxies() {
		client := s.Proxies.GetHTTPClient()
		client.Timeout = timeout
		return client
	}
	return &http.Client{Timeout: timeout}
}

// tryRefreshToken attempts to refresh OAuth credentials if needed.
// Returns true if refresh was attempted (regardless of success).
func (s *Server) tryRefreshToken(conn *model.ProviderConnection) bool {
	if !provider.ShouldRefresh(conn) {
		return false
	}
	result, err := provider.RefreshCredentials(conn.Provider, conn, nil)
	if err != nil {
		slog.Warn("Token refresh failed", slog.String("provider", conn.Provider), "error", err)
		return true
	}
	provider.ApplyRefreshResult(conn, result)
	s.DB.UpdateConnection(conn)
	return true
}

// handleComboChat processes a combo request with fallback/round-robin strategy.
func (s *Server) handleComboChat(w http.ResponseWriter, r *http.Request, req ChatCompletionRequest, combo *model.Combo) {
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

		conn := selectAvailableConnection(conns, modelInfo.Model, nil)
		if conn == nil {
			lastError = fmt.Sprintf("all accounts rate-limited for: %s", modelInfo.Provider)
			lastStatus = 503
			continue
		}

		// Phase 9: Pre-check OAuth token refresh
		s.tryRefreshToken(conn)

		baseURL := providerInfo.BaseURL
		if conn.Data.BaseURL != "" {
			baseURL = conn.Data.BaseURL
		}
		if baseURL == "" {
			lastError = fmt.Sprintf("no base URL for provider: %s", modelInfo.Provider)
			lastStatus = 503
			continue
		}

		// Build and execute upstream request
		reqCopy := req
		reqCopy.Model = modelInfo.Model
		bodyBytes, err := json.Marshal(reqCopy)
		if err != nil {
			lastError = "failed to marshal request"
			lastStatus = 500
			continue
		}

		targetURL := strings.TrimRight(baseURL, "/") + "/chat/completions"
		upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(bodyBytes))
		if err != nil {
			lastError = "failed to create upstream request"
			lastStatus = 500
			continue
		}

		if conn.Data.APIKey != "" {
			upstreamReq.Header.Set("Authorization", "Bearer "+conn.Data.APIKey)
		} else if conn.Data.AccessToken != "" {
			upstreamReq.Header.Set("Authorization", "Bearer "+conn.Data.AccessToken)
		}
		upstreamReq.Header.Set("Content-Type", "application/json")

		client := s.getHTTPClient(5 * time.Minute)
		resp, err := client.Do(upstreamReq)
		if err != nil {
			lastError = fmt.Sprintf("upstream request failed: %v", err)
			lastStatus = 502
			slog.Warn("Combo model failed", slog.String("model", modelStr), "error", err)
			continue
		}

		// Success (2xx) - stream response to client
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			slog.Info("Combo model succeeded", slog.String("model", modelStr))
			uc := &usageContext{
				Provider:     modelInfo.Provider,
				Model:        modelInfo.Model,
				ConnectionID: conn.ID,
				APIKey:       extractRequestAPIKey(r),
				Endpoint:     "/v1/chat/completions",
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

		// Check if should fallback
		fallbackResult := provider.CheckFallbackError(resp.StatusCode, string(errBody), 0)
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

	// All models failed
	if lastStatus == 0 {
		lastStatus = 503
	}
	if lastError == "" {
		lastError = "all combo models unavailable"
	}
	writeJSON(w, lastStatus, map[string]string{"error": lastError})
}

// handleSingleModelChat processes a single model request (non-combo).
func (s *Server) handleSingleModelChat(w http.ResponseWriter, r *http.Request, req ChatCompletionRequest) {
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
	if err != nil || len(conns) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("no active credentials for provider: %s", modelInfo.Provider),
		})
		return
	}

	// Select best available connection (priority-ordered, cooldown-aware, model-lock-aware)
	conn := selectAvailableConnection(conns, modelInfo.Model, nil)
	if conn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("all accounts rate-limited for provider: %s", modelInfo.Provider),
		})
		return
	}

	// Phase 9: Pre-check OAuth token refresh
	s.tryRefreshToken(conn)

	// Determine base URL
	baseURL := providerInfo.BaseURL
	if conn.Data.BaseURL != "" {
		baseURL = conn.Data.BaseURL
	}
	if baseURL == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("no base URL configured for provider: %s", modelInfo.Provider),
		})
		return
	}

	// Determine target format and build upstream request
	targetFormat := translator.FormatOpenAI
	switch providerInfo.APIType {
	case "anthropic":
		targetFormat = translator.FormatAnthropic
	case "gemini":
		targetFormat = translator.FormatGemini
	}

	var bodyBytes []byte
	var targetURL string

	if targetFormat == translator.FormatOpenAI {
		// Standard OpenAI-compatible passthrough
		req.Model = modelInfo.Model

		// Phase 9: Loop guard + termination prompt + max_tokens clamping
		var bodyMap map[string]any
		rawBody, _ := json.Marshal(req)
		json.Unmarshal(rawBody, &bodyMap)

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

		bodyBytes, err = json.Marshal(bodyMap)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to marshal request"})
			return
		}
		targetURL = strings.TrimRight(baseURL, "/") + "/chat/completions"
	} else {
		// Translate request to provider format
		var bodyMap map[string]any
		rawBody, _ := json.Marshal(req)
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

		bodyBytes, err = json.Marshal(translated)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to marshal translated request"})
			return
		}

		switch targetFormat {
		case translator.FormatAnthropic:
			targetURL = strings.TrimRight(baseURL, "/") + "/v1/messages"
		case translator.FormatGemini:
			if req.Stream {
				targetURL = strings.TrimRight(baseURL, "/") + "/v1beta/models/" + modelInfo.Model + ":streamGenerateContent?alt=sse"
			} else {
				targetURL = strings.TrimRight(baseURL, "/") + "/v1beta/models/" + modelInfo.Model + ":generateContent"
			}
		}
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upstream request"})
		return
	}

	// Set auth headers
	if conn.Data.APIKey != "" {
		if targetFormat == translator.FormatAnthropic {
			upstreamReq.Header.Set("x-api-key", conn.Data.APIKey)
			upstreamReq.Header.Set("anthropic-version", "2023-06-01")
		} else if targetFormat == translator.FormatGemini {
			// Gemini uses query param for API key
			q := upstreamReq.URL.Query()
			q.Set("key", conn.Data.APIKey)
			upstreamReq.URL.RawQuery = q.Encode()
		} else {
			upstreamReq.Header.Set("Authorization", "Bearer "+conn.Data.APIKey)
		}
	} else if conn.Data.AccessToken != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+conn.Data.AccessToken)
	}
	upstreamReq.Header.Set("Content-Type", "application/json")

	slog.Info("Proxying request",
		slog.String("model", modelInfo.Model),
		slog.String("provider", modelInfo.Provider),
		slog.String("format", string(targetFormat)),
		slog.String("connection", conn.ID),
		slog.Bool("stream", req.Stream),
	)

	// Phase 9: Use proxy-aware HTTP client
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
				if conn.Data.AccessToken != "" {
					retryReq.Header.Set("Authorization", "Bearer "+conn.Data.AccessToken)
				}
				retryReq.Header.Set("Content-Type", "application/json")
				if targetFormat == translator.FormatAnthropic {
					retryReq.Header.Set("x-api-key", conn.Data.AccessToken)
					retryReq.Header.Set("anthropic-version", "2023-06-01")
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(errBody)
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
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lastUsage usage.Usage

	for scanner.Scan() {
		// Check for client disconnect
		select {
		case <-ctx.Done():
			slog.Info("Client disconnected during stream", slog.String("model", model))
			if lastUsage.TotalTokens > 0 {
				s.recordUsage(uc, lastUsage)
			}
			return
		default:
		}

		line := scanner.Text()

		// Skip empty lines (SSE separators)
		if line == "" {
			continue
		}

		// Handle SSE data lines
		if !strings.HasPrefix(line, "data: ") && !strings.HasPrefix(line, "data:") {
			// Non-data SSE lines (event:, id:, etc.) - pass through for OpenAI format
			if format == translator.FormatOpenAI {
				fmt.Fprintf(w, "%s\n", line)
				flusher.Flush()
			}
			continue
		}

		// Extract data payload
		data := strings.TrimPrefix(line, "data: ")
		data = strings.TrimPrefix(data, "data:")
		data = strings.TrimSpace(data)

		// Handle [DONE] marker
		if data == "[DONE]" {
			if lastUsage.TotalTokens > 0 {
				s.recordUsage(uc, lastUsage)
			}
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		}

		// Extract usage from SSE chunk (final chunk often has usage)
		if u := usage.ExtractFromSSELine([]byte(data)); u.TotalTokens > 0 {
			lastUsage = u
		}

		// Translate SSE chunks if needed
		if format != translator.FormatOpenAI {
			translated, isDone, err := translator.TranslateSSEChunk(format, []byte(data), model)
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
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		flusher.Flush()
	}

	// Ensure [DONE] is sent if stream ends without it
	if lastUsage.TotalTokens > 0 {
		s.recordUsage(uc, lastUsage)
	}
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
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

	conn := selectAvailableConnection(conns, modelInfo.Model, nil)
	if conn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("all accounts rate-limited for provider: %s", modelInfo.Provider),
		})
		return
	}

	baseURL := providerInfo.BaseURL
	if conn.Data.BaseURL != "" {
		baseURL = conn.Data.BaseURL
	}

	// Set the resolved model name
	reqBody["model"] = modelInfo.Model
	translatedBody, _ := json.Marshal(reqBody)

	// Determine target URL based on provider API type
	var targetURL string
	switch providerInfo.APIType {
	case "anthropic":
		targetURL = strings.TrimRight(baseURL, "/") + "/v1/messages"
	default:
		// For OpenAI-compatible providers, translate Claude format to OpenAI
		targetURL = strings.TrimRight(baseURL, "/") + "/chat/completions"
		// TODO: translate Claude request to OpenAI format for non-Anthropic providers
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(translatedBody))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upstream request"})
		return
	}

	// Set auth headers
	if conn.Data.APIKey != "" {
		if providerInfo.APIType == "anthropic" {
			upstreamReq.Header.Set("x-api-key", conn.Data.APIKey)
			upstreamReq.Header.Set("anthropic-version", "2023-06-01")
		} else {
			upstreamReq.Header.Set("Authorization", "Bearer "+conn.Data.APIKey)
		}
	} else if conn.Data.AccessToken != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+conn.Data.AccessToken)
	}
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

		ctx := r.Context()
		buf := make([]byte, 4096)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				flusher.Flush()
			}
			if readErr != nil {
				break
			}
		}
	} else {
		// Copy headers and body
		for key, values := range resp.Header {
			for _, v := range values {
				w.Header().Add(key, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
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

	conn := selectAvailableConnection(conns, modelInfo.Model, nil)
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

// selectAvailableConnection picks the best connection using priority, cooldown, and model locks.
func selectAvailableConnection(conns []model.ProviderConnection, modelName string, excludeIDs map[string]bool) *model.ProviderConnection {
	return provider.SelectCredential(conns, modelName, excludeIDs)
}

// recordUsage persists a usage entry to the database asynchronously-safe (called inline).
func (s *Server) recordUsage(uc *usageContext, u usage.Usage) {
	if uc == nil || u.TotalTokens == 0 {
		return
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
	if err := s.DB.SaveUsageEntry(entry); err != nil {
		slog.Warn("Failed to record usage", "error", err, "model", uc.Model)
	}
}

// extractRequestAPIKey extracts the API key from the request Authorization header.
func extractRequestAPIKey(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if key := r.Header.Get("x-api-key"); key != "" {
		return key
	}
	return ""
}
