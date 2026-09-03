package handler

// Qoder chat handler: COSY-signed requests to api3.qoder.sh with SSE
// envelope unwrapping. Ported from 9router executors/qoder.js.

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

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
	"github.com/arisvia/cyrene-gateway/internal/usage"
)

// handleQoderChat executes a chat request against Qoder's COSY-signed endpoint.
func (s *Server) handleQoderChat(w http.ResponseWriter, r *http.Request, req ChatCompletionRequest, rawBody []byte, modelInfo model.ModelInfo, conn *model.ProviderConnection, providerInfo provider.ProviderInfo) {
	psd := conn.Data.ProviderSpecificData
	userID := ""
	machineID := ""
	if psd != nil {
		userID, _ = psd["userId"].(string)
		machineID, _ = psd["machineId"].(string)
	}

	rawToken := conn.Data.AccessToken
	if rawToken == "" {
		rawToken = conn.Data.APIKey
	}
	if rawToken == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "qoder credential is missing; connect the account via OAuth or add a Personal Access Token (pt-...)",
		})
		return
	}

	client := s.getHTTPClient(5 * time.Minute)

	// PAT (pt-...) → exchange for short-lived job token + resolve userId so
	// COSY signing works (9router@9c9dd7b1, @d433c0b2). Device/job tokens
	// pass through unchanged.
	resolved, err := provider.ResolveQoderCredential(rawToken, userID, client)
	if err != nil {
		slog.Warn("Qoder PAT exchange failed", slog.String("connection", conn.ID), "error", err)
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": fmt.Sprintf("qoder PAT exchange failed: %v", err),
		})
		return
	}
	if resolved.UserID == "" {
		resolved.UserID = userID
	}
	if resolved.UserID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "qoder credential is missing userId; reconnect the account via OAuth",
		})
		return
	}

	ideVersion, _ := conn.Data.ProviderSpecificData["ideVersion"].(string)
	if ideVersion == "" {
		ideVersion, _ = conn.Data.ProviderSpecificData["qoderIDEVersion"].(string)
	}
	publicKeyPEM, _ := conn.Data.ProviderSpecificData["rsaPublicKeyPEM"].(string)
	if publicKeyPEM == "" {
		publicKeyPEM, _ = conn.Data.ProviderSpecificData["publicKeyPEM"].(string)
	}

	creds := provider.QoderCosyCreds{
		UserID:       resolved.UserID,
		AuthToken:    resolved.AccessToken,
		Name:         conn.Name,
		Email:        conn.Email,
		MachineID:    machineID,
		IDEVersion:   ideVersion,
		PublicKeyPEM: publicKeyPEM,
	}

	// Build the request body map from the raw body to preserve unknown fields
	var bodyMap map[string]any
	json.Unmarshal(rawBody, &bodyMap)

	start := time.Now()
	encodedBody, qoderKey, err := provider.BuildQoderRequestBody(modelInfo.Model, bodyMap, creds, client)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// jt- tokens are rejected by api3 ("Login expired" 403) — route them to
	// api2.qoder.sh like the official qodercli.
	chatURL := provider.QoderChatURLForToken(resolved.AccessToken)

	cosyHeaders, err := provider.BuildQoderCosyHeaders(encodedBody, chatURL, creds)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": fmt.Sprintf("qoder cosy signing failed: %v", err),
		})
		return
	}

	modelSource := "system"

	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", chatURL, bytes.NewReader(encodedBody))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upstream request"})
		return
	}

	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", "text/event-stream")
	upstreamReq.Header.Set("Cache-Control", "no-cache")
	upstreamReq.Header.Set("X-Model-Key", qoderKey)
	upstreamReq.Header.Set("X-Model-Source", modelSource)
	// gzip triggers signature validation on Qoder's CDN; force identity
	upstreamReq.Header.Set("Accept-Encoding", "identity")
	for k, v := range cosyHeaders {
		upstreamReq.Header.Set(k, v)
	}

	slog.Info("Proxying Qoder request",
		slog.String("model", qoderKey),
		slog.String("connection", conn.ID),
	)

	resp, err := client.Do(upstreamReq)
	if err != nil {
		slog.Error("Qoder upstream request failed", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "qoder upstream request failed"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		provider.ApplyErrorState(conn, resp.StatusCode, string(errBody))
		s.DB.UpdateConnection(conn)
		if s.Metrics != nil {
			s.Metrics.ObserveRequest(modelInfo.Provider, modelInfo.Model, "/v1/chat/completions", resp.StatusCode, time.Since(start).Seconds(), nil)
		}
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
		Endpoint:     "/v1/chat/completions",
		StartedAt:    start,
		Status:       resp.StatusCode,
	}
	// 非流式请求（OpenAI 契约 stream 默认 false）：上游只有 SSE，
	// 在网关侧聚合 chunks 后拼成标准 chat.completion JSON 返回。
	if !req.Stream {
		s.proxyQoderNonStreaming(w, r, resp, qoderKey, uc)
		return
	}

	// Stream with envelope unwrapping
	s.proxyQoderStreaming(w, r, resp, qoderKey, uc)
}

// proxyQoderNonStreaming aggregates the upstream SSE envelope into a single
// OpenAI chat.completion JSON object for stream=false clients.
func (s *Server) proxyQoderNonStreaming(w http.ResponseWriter, r *http.Request, resp *http.Response, model string, uc *usageContext) {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lastUsage usage.Usage
	var content strings.Builder
	var reasoning strings.Builder
	var toolCalls []map[string]any
	finishReason := ""
	id, created := "", int64(0)
	role := "assistant"

	for scanner.Scan() {
		select {
		case <-r.Context().Done():
			slog.Info("Client disconnected during Qoder non-stream aggregation", slog.String("model", model))
			return
		default:
		}

		data, done := provider.UnwrapQoderSSELine(scanner.Text(), "qoder/"+model)
		if done {
			break
		}
		if data == "" {
			continue
		}
		if u := usage.ExtractFromSSELine([]byte(data)); u.TotalTokens > 0 {
			lastUsage = u
		}

		var chunk struct {
			ID      string `json:"id"`
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Delta        struct {
					Role             string          `json:"role"`
					Content          string          `json:"content"`
					ReasoningContent string          `json:"reasoning_content"`
					ToolCalls        json.RawMessage `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Created int64 `json:"created"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.ID != "" {
			id = chunk.ID
		}
		if chunk.Created > 0 {
			created = chunk.Created
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		c := chunk.Choices[0]
		if c.Delta.Role != "" {
			role = c.Delta.Role
		}
		content.WriteString(c.Delta.Content)
		reasoning.WriteString(c.Delta.ReasoningContent)
		if c.FinishReason != "" {
			finishReason = c.FinishReason
		}
		if len(c.Delta.ToolCalls) > 0 {
			var tcs []map[string]any
			if err := json.Unmarshal(c.Delta.ToolCalls, &tcs); err == nil {
				toolCalls = append(toolCalls, tcs...)
			}
		}
	}

	if lastUsage.TotalTokens > 0 {
		s.recordUsage(uc, lastUsage)
	}

	if id == "" {
		id = "chatcmpl-qoder-" + generateID()[:12]
	}
	if created == 0 {
		created = time.Now().Unix()
	}

	msg := map[string]any{"role": role, "content": content.String()}
	if reasoning.Len() > 0 {
		msg["reasoning_content"] = reasoning.String()
	}
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}
	respBody := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": created,
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       msg,
			"finish_reason": finishReason,
		}},
		"usage": map[string]any{
			"prompt_tokens":     lastUsage.PromptTokens,
			"completion_tokens": lastUsage.CompletionTokens,
			"total_tokens":      lastUsage.TotalTokens,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(respBody)
}

// proxyQoderStreaming unwraps Qoder's {statusCodeValue, body} SSE envelope
// into plain OpenAI SSE chunks.
func (s *Server) proxyQoderStreaming(w http.ResponseWriter, r *http.Request, resp *http.Response, model string, uc *usageContext) {
	flusher, ok := w.(http.Flusher)
	if !ok {
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
		select {
		case <-ctx.Done():
			slog.Info("Client disconnected during Qoder stream", slog.String("model", model))
			if lastUsage.TotalTokens > 0 {
				s.recordUsage(uc, lastUsage)
			}
			return
		default:
		}

		line := scanner.Text()
		data, done := provider.UnwrapQoderSSELine(line, "qoder/"+model)

		if done {
			if lastUsage.TotalTokens > 0 {
				s.recordUsage(uc, lastUsage)
			}
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			// Terminal frame received — Qoder keeps the socket open after
			// [DONE] (agent keepalive); stop reading so the stream closes
			// immediately (9router@9c9dd7b1).
			return
		}

		if data == "" {
			continue
		}

		if u := usage.ExtractFromSSELine([]byte(data)); u.TotalTokens > 0 {
			lastUsage = u
		}

		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// Stream ended without a terminal frame — emit [DONE] ourselves.
	if lastUsage.TotalTokens > 0 {
		s.recordUsage(uc, lastUsage)
	}
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// autoProvisionNoAuthConnection creates a persistent connection for NoAuth
// providers so they work out of the box (like 9router's free providers).
func (s *Server) autoProvisionNoAuthConnection(providerInfo provider.ProviderInfo) *model.ProviderConnection {
	conn := &model.ProviderConnection{
		ID:       generateID(),
		Provider: providerInfo.ID,
		AuthType: "none",
		Name:     providerInfo.Name + " (auto)",
		Priority: providerInfo.Priority,
		IsActive: true,
	}
	if err := s.DB.CreateConnection(conn); err != nil {
		slog.Warn("Failed to auto-provision NoAuth connection",
			slog.String("provider", providerInfo.ID), "error", err)
		return nil
	}
	slog.Info("Auto-provisioned NoAuth connection", slog.String("provider", providerInfo.ID))
	return conn
}
