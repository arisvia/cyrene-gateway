package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// handleComboChat processes a combo request with fallback/round-robin strategy.
func (s *Server) handleComboChat(w http.ResponseWriter, r *http.Request, req ChatCompletionRequest, rawBody []byte, combo *model.Combo) {
	// One conversation stays on one account per provider across combo steps and
	// re-dispatches, keeping upstream prompt KV caches warm.
	sessionKey := requestSessionKey(r, rawBody)

	// Determine strategy from combo kind or global settings
	strategy := provider.StrategyFallback
	switch combo.Kind {
	case "round-robin":
		strategy = provider.StrategyRoundRobin
	case "cost-first":
		strategy = provider.StrategyCostFirst
	case "latency-first":
		strategy = provider.StrategyLatencyFirst
	case "":
		if settings, err := s.DB.GetSettings(); err == nil && settings != nil {
			switch settings.ComboStrategy {
			case "round-robin":
				strategy = provider.StrategyRoundRobin
			case "cost-first":
				strategy = provider.StrategyCostFirst
			case "latency-first":
				strategy = provider.StrategyLatencyFirst
			}
		}
	}

	// Get sticky limit from settings (default 1)
	stickyLimit := 1

	// Cost resolver for cost-first routing
	costFn := func(m string) float64 {
		return s.resolveModelCostFactor(m)
	}

	// Apply rotation / sorting
	models := s.Combos.GetRotatedModels(combo.Models, combo.Name, strategy, stickyLimit, costFn)
	slog.Info("Combo request",
		slog.String("combo", combo.Name),
		slog.String("strategy", string(strategy)),
		slog.Int("models", len(models)),
	)
	start := time.Now()
	loopGuard := s.loopGuardEnabled()

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

		// Walk the account pool for this model: an account-scoped failure
		// (quota, throttle, transport) retries on the next account before the
		// combo advances to the next model — parity with the chat pipeline.
		tried := make(map[string]bool)
	accountLoop:
		for {
			conn := s.selectAvailableConnection(conns, modelInfo.Model, tried, sessionKey)
			if conn == nil {
				if len(tried) == 0 {
					lastError = fmt.Sprintf("all accounts rate-limited for: %s", modelInfo.Provider)
					lastStatus = 503
				}
				break
			}
			tried[conn.ID] = true

			// Phase 9: Pre-check OAuth token refresh
			s.tryRefreshToken(conn)

			// Qoder and Antigravity use specialized protocols and executors. They
			// report retryable when the account (not the request) is at fault, so
			// the next account gets a chance before the combo moves on.
			if modelInfo.Provider == "qoder" || modelInfo.Provider == "qoder-cn" {
				slog.Info("Combo delegating to Qoder executor", slog.String("model", modelInfo.Model))
				if retryable := s.handleQoderChat(w, r, req, rawBody, modelInfo, conn, providerInfo); !retryable {
					return
				}
				lastError = fmt.Sprintf("qoder account unavailable: %s", conn.ID)
				lastStatus = 503
				continue
			}
			if modelInfo.Provider == "antigravity" {
				slog.Info("Combo delegating to Antigravity executor", slog.String("model", modelInfo.Model))
				if retryable := s.handleAntigravityChat(w, r, req, rawBody, modelInfo, conn, providerInfo); !retryable {
					return
				}
				lastError = fmt.Sprintf("antigravity account unavailable: %s", conn.ID)
				lastStatus = 503
				continue
			}

			upstreamStream := req.Stream || providerInfo.ForceStream
			upstreamReq, targetFormat, err := provider.PrepareUpstreamRequest(r.Context(),
				provider.ExecutionRequest{RawBody: rawBody, Stream: req.Stream, HasTools: req.Tools != nil, LoopGuardEnabled: loopGuard, AutoPromptCache: s.autoPromptCacheEnabled()},
				provider.ExecutionCandidate{Connection: conn, ModelInfo: modelInfo, ProviderInfo: providerInfo},
				func(body map[string]any, format string) { s.applyTokenSaver(body, format, modelInfo.Provider, r) },
			)
			if err != nil {
				// Request-build failures repeat on every account of this model.
				lastError = err.Error()
				lastStatus = http.StatusBadRequest
				break accountLoop
			}

			endpoint := resolveRequestEndpoint(r, "/v1/chat/completions")
			s.publishRequestEvent(r, RequestEvent{Type: "routing", Provider: modelInfo.Provider, Model: modelInfo.Model, Endpoint: endpoint, Status: "routing"})

			clientTimeout := 5 * time.Minute
			if upstreamStream {
				clientTimeout = 0
			}
			client := s.getHTTPClient(clientTimeout)
			resp, err := client.Do(upstreamReq)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) {
					slog.Info("Client canceled combo request", slog.String("model", modelStr))
					return
				}
				lastError = fmt.Sprintf("upstream request failed: %v", err)
				lastStatus = 502
				slog.Warn("Combo account transport failed, trying next account", slog.String("model", modelStr), "error", err)
				provider.ApplyErrorState(conn, 502, err.Error())
				s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
				continue
			}
			// Success (2xx) - check for zero usable content on non-final models (9router #4750 parity)
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				isLastModel := (i == len(models)-1)
				if !isLastModel {
					respBytes, err := io.ReadAll(resp.Body)
					resp.Body.Close()
					if err == nil {
						if isZeroUsableContent(respBytes, resp.Header.Get("Content-Type")) {
							slog.Warn("Combo model returned HTTP 200 with zero usable content, falling back to next model",
								slog.String("model", modelStr),
								slog.Int("attempt", i+1),
							)
							lastStatus = resp.StatusCode
							lastError = "upstream returned HTTP 200 with zero usable content"
							s.publishRequestEvent(r, RequestEvent{Type: "fallback", Provider: modelInfo.Provider, Model: modelInfo.Model, Status: "fallback", Endpoint: resolveRequestEndpoint(r, "/v1/chat/completions")})
							break accountLoop
						}
						// Reconstruct response body with read bytes
						resp.Body = io.NopCloser(bytes.NewReader(respBytes))
					}
				}

				slog.Info("Combo model succeeded", slog.String("model", modelStr), slog.String("connection", conn.ID))
				s.Combos.RecordModelLatency(modelStr, time.Since(start).Milliseconds())
				provider.ResetAccountState(conn)
				s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
				s.publishRequestEvent(r, RequestEvent{Type: "upstream_connected", Provider: modelInfo.Provider, Model: modelInfo.Model, Status: "upstream_connected", Endpoint: resolveRequestEndpoint(r, "/v1/chat/completions")})
				uc := &usageContext{
					RequestID:    ensureRequestID(r),
					Provider:     modelInfo.Provider,
					Model:        modelInfo.Model,
					ConnectionID: conn.ID,
					APIKey:       extractRequestAPIKey(r),
					Endpoint:     resolveRequestEndpoint(r, "/v1/chat/completions"),
					StartedAt:    start,
					Status:       resp.StatusCode,
					Prompt:       extractPromptSummary(req.Messages),
				}
				s.proxyResponse(w, r, resp, req.Stream, upstreamStream, targetFormat, modelInfo.Model, uc)
				resp.Body.Close()
				return
			}
			// Error response - read body for error classification
			errBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			lastStatus = resp.StatusCode
			lastError = string(errBody)

			provider.ApplyErrorState(conn, resp.StatusCode, string(errBody))
			s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
			// Account-scoped failures (quota, throttle, 5xx) get the next account
			// on the SAME model; request-scoped 4xx repeat everywhere, so advance
			// to the next model (9router#5057 parity) — or surface them on the wire
			// when this already was the final model.
			if !provider.CheckFallbackError(resp.StatusCode, string(errBody), conn.Data.BackoffLevel).ShouldFallback {
				if i == len(models)-1 {
					s.Combos.RecordModelFailure(modelStr)
					s.recordComboFailure(r, combo.Name, resp.StatusCode, string(errBody), start, extractPromptSummary(req.Messages))
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(resp.StatusCode)
					w.Write(errBody)
					return
				}
				slog.Warn("Combo model returned request-scoped error, advancing to next model",
					slog.String("model", modelStr),
					slog.Int("status", resp.StatusCode),
				)
				break accountLoop
			}
			s.publishRequestEvent(r, RequestEvent{Type: "fallback", Provider: modelInfo.Provider, Model: modelInfo.Model, Status: "fallback", Endpoint: resolveRequestEndpoint(r, "/v1/chat/completions")})
			slog.Warn("Combo account failed, trying next account for the same model",
				slog.String("model", modelStr),
				slog.String("connection", conn.ID),
				slog.Int("status", resp.StatusCode),
			)
		}
		if len(tried) > 0 {
			// Every account of this model failed; mark the model itself failed
			// so rotation advances for subsequent requests.
			s.Combos.RecordModelFailure(modelStr)
		}
	}
	if lastStatus == 0 {
		lastStatus = 503
	}
	if lastError == "" {
		lastError = "all combo models unavailable"
	}
	s.recordComboFailure(r, combo.Name, lastStatus, lastError, start, extractPromptSummary(req.Messages))
	writeJSON(w, lastStatus, map[string]string{"error": lastError})
}

// resolveModelCostFactor resolves the relative cost factor for a model.
// Free models or -free suffix evaluate to 0.0; otherwise looks up cached price_factor.
func (s *Server) resolveModelCostFactor(modelStr string) float64 {
	lower := strings.ToLower(modelStr)
	if strings.HasSuffix(lower, "-free") || lower == "big-pickle" {
		return 0.0
	}
	modelInfo, err := provider.ResolveModel(modelStr, s.DB)
	if err != nil || modelInfo.Provider == "" {
		return 1.0
	}
	if s.DB != nil {
		if raw, err := s.DB.KVGet("providerModelCache", modelInfo.Provider); err == nil && raw != "" {
			var cache struct {
				Models []struct {
					ID          string  `json:"id"`
					PriceFactor float64 `json:"price_factor"`
					IsFree      bool    `json:"is_free"`
				} `json:"models"`
			}
			if err := json.Unmarshal([]byte(raw), &cache); err == nil {
				for _, m := range cache.Models {
					if m.ID == modelInfo.Model || m.ID == modelStr {
						if m.IsFree {
							return 0.0
						}
						if m.PriceFactor > 0 {
							return m.PriceFactor
						}
					}
				}
			}
		}
	}
	return 1.0
}

// isZeroUsableContent checks whether a 2xx upstream response contains zero usable completion/tool/reasoning content
// (e.g. empty assistant message with in_progress or incomplete finish_reason).
func isZeroUsableContent(bodyBytes []byte, contentType string) bool {
	if len(bodyBytes) == 0 {
		return true
	}
	// Only inspect JSON responses; SSE or binary formats stream progressively
	if strings.Contains(contentType, "text/event-stream") {
		return false
	}

	var raw map[string]any
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return false
	}

	// 1. OpenAI chat format: choices array
	if choices, ok := raw["choices"].([]any); ok {
		if len(choices) == 0 {
			return true
		}
		first, ok := choices[0].(map[string]any)
		if !ok || first == nil {
			return true
		}
		msg, _ := first["message"].(map[string]any)
		if msg == nil {
			msg, _ = first["delta"].(map[string]any)
		}

		hasContent := false
		if c, ok := msg["content"].(string); ok && strings.TrimSpace(c) != "" {
			hasContent = true
		} else if arr, ok := msg["content"].([]any); ok && len(arr) > 0 {
			hasContent = true
		}

		hasToolCalls := false
		if tcs, ok := msg["tool_calls"].([]any); ok && len(tcs) > 0 {
			hasToolCalls = true
		}
		if fc, ok := msg["function_call"].(map[string]any); ok && len(fc) > 0 {
			hasToolCalls = true
		}

		hasReasoning := false
		if rc, ok := msg["reasoning_content"].(string); ok && strings.TrimSpace(rc) != "" {
			hasReasoning = true
		}

		finish, _ := first["finish_reason"].(string)
		isNormalFinish := (finish == "stop" || finish == "length" || finish == "tool_calls" || finish == "function_call")

		if !hasContent && !hasToolCalls && !hasReasoning && (!isNormalFinish || finish == "in_progress" || finish == "incomplete") {
			return true
		}
	}

	// 2. Anthropic messages format: content array
	if content, ok := raw["content"].([]any); ok {
		hasContent := false
		for _, c := range content {
			if cm, ok := c.(map[string]any); ok {
				t, _ := cm["type"].(string)
				if t == "text" {
					if txt, ok := cm["text"].(string); ok && strings.TrimSpace(txt) != "" {
						hasContent = true
						break
					}
				} else if t == "tool_use" {
					hasContent = true
					break
				}
			}
		}
		stopReason, _ := raw["stop_reason"].(string)
		if !hasContent && (stopReason == "" || stopReason == "in_progress" || stopReason == "incomplete") {
			return true
		}
	}

	return false
}

func (s *Server) recordComboFailure(r *http.Request, comboName string, lastStatus int, lastError string, start time.Time, prompt string) {
	latencyMs := int(time.Since(start).Milliseconds())
	if latencyMs < 0 {
		latencyMs = 0
	}
	endpoint := resolveRequestEndpoint(r, "/v1/chat/completions")
	statusStr := fmt.Sprintf("%d", lastStatus)
	rdID := fmt.Sprintf("%d-%s", time.Now().UnixNano(), comboName)
	rdData := map[string]any{
		"id":        rdID,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"provider":  "combo",
		"model":     comboName,
		"status":    statusStr,
		"latencyMs": latencyMs,
		"endpoint":  endpoint,
		"error":     lastError,
	}
	if prompt != "" {
		rdData["input"] = prompt
	}
	rdBytes, _ := json.Marshal(rdData)
	_ = s.DB.SaveRequestDetail(&db.RequestDetail{
		ID:        rdID,
		Timestamp: rdData["timestamp"].(string),
		Provider:  "combo",
		Model:     comboName,
		Status:    statusStr,
		Data:      string(rdBytes),
	})
	if s.Metrics != nil {
		s.Metrics.ObserveRequest("combo", comboName, endpoint, lastStatus, time.Since(start).Seconds(), nil)
	}
	s.publishRequestEvent(r, RequestEvent{Type: "request", Provider: "combo", Model: comboName, Status: statusStr, Endpoint: endpoint, LatencyMs: int64(latencyMs)})
}
