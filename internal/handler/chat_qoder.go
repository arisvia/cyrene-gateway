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
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
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
	resolved, err := provider.ResolveQoderCredentialFor(conn.Provider, rawToken, userID, client)
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
		ProviderID:   conn.Provider,
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
	s.applyLoopGuard(bodyMap, rawBody, "openai", req.Tools != nil)
	s.applyTokenSaver(bodyMap, "openai", modelInfo.Provider)

	start := time.Now()
	encodedBody, qoderKey, modelSource, err := provider.BuildQoderRequestBody(modelInfo.Model, bodyMap, creds, client)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// jt- tokens are rejected by api3 ("Login expired" 403) — route them to
	// api2.qoder.sh like the official qodercli.
	chatURL := provider.QoderChatURLForToken(conn.Provider, resolved.AccessToken)
	cosyHeaders, err := provider.BuildQoderCosyHeaders(encodedBody, chatURL, creds)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": fmt.Sprintf("qoder cosy signing failed: %v", err),
		})
		return
	}
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
	endpoint := resolveRequestEndpoint(r, "/v1/chat/completions")
	s.publishRequestEvent(r, RequestEvent{Type: "routing", Provider: modelInfo.Provider, Model: modelInfo.Model, Endpoint: endpoint, Status: "routing"})

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
		if s.DB != nil {
			s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
		}
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
		if s.DB != nil {
			_ = s.DB.SaveRequestDetail(&db.RequestDetail{
				ID:           rdID,
				Timestamp:    rdData["timestamp"].(string),
				Provider:     modelInfo.Provider,
				Model:        modelInfo.Model,
				ConnectionID: conn.ID,
				Status:       statusStr,
				Data:         string(rdBytes),
			})
		}
		if s.Metrics != nil {
			s.Metrics.ObserveRequest(modelInfo.Provider, modelInfo.Model, endpoint, resp.StatusCode, time.Since(start).Seconds(), nil)
		}
		s.publishRequestEvent(r, RequestEvent{Type: "request", Provider: modelInfo.Provider, Model: modelInfo.Model, Status: statusStr, Endpoint: endpoint, LatencyMs: int64(latencyMs)})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(errBody)
		return
	}

	provider.ResetAccountState(conn)
	if s.DB != nil {
		s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
	}

	uc := &usageContext{
		Provider:     modelInfo.Provider,
		Model:        modelInfo.Model,
		ConnectionID: conn.ID,
		APIKey:       extractRequestAPIKey(r),
		Endpoint:     endpoint,
		StartedAt:    start,
		Status:       resp.StatusCode,
		Prompt:       extractPromptSummary(req.Messages),
	}
	// 非流式请求（OpenAI 契约 stream 默认 false）：上游只有 SSE，
	// 在网关侧聚合 chunks 后拼成标准 chat.completion JSON 返回。
	if !req.Stream {
		s.proxyQoderNonStreaming(w, r, resp, qoderKey, uc, conn)
		return
	}

	// Stream with envelope unwrapping
	s.proxyQoderStreaming(w, r, resp, qoderKey, uc, conn)
}

// proxyQoderNonStreaming aggregates the upstream SSE envelope into a single
// OpenAI chat.completion JSON object for stream=false clients.
func (s *Server) proxyQoderNonStreaming(w http.ResponseWriter, r *http.Request, resp *http.Response, model string, uc *usageContext, conn *model.ProviderConnection) {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lastUsage usage.Usage
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
	id, created := "", int64(0)
	complete := false
	defer func() {
		if uc != nil {
			if choice := choices[0]; choice != nil {
				uc.Response = choice.content.String()
				if uc.Response == "" {
					uc.Response = choice.reasoning.String()
				}
			}
		}
		s.recordUsage(uc, lastUsage)
	}()

	for scanner.Scan() {
		if r.Context().Err() != nil {
			if uc != nil {
				uc.Status = 499
			}
			return
		}
		data, done := provider.UnwrapQoderSSELine(scanner.Text(), "qoder/"+model)
		if done {
			if data != "" {
				s.writeQoderFailure(w, conn, uc, data, false)
				return
			}
			complete = true
			break
		}
		if data == "" {
			continue
		}
		if u := usage.ExtractFromSSELine([]byte(data)); u.TotalTokens > 0 {
			lastUsage = u
		}
		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			s.writeQoderFailure(w, conn, uc, "invalid qoder SSE chunk", false)
			return
		}
		if chunk.ID != "" {
			id = chunk.ID
		}
		if chunk.Created > 0 {
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
	if r.Context().Err() != nil {
		if uc != nil {
			uc.Status = 499
		}
		return
	}
	if !complete {
		message := "qoder stream ended before completion"
		if err := scanner.Err(); err != nil {
			message = "failed to read qoder stream: " + err.Error()
		}
		s.writeQoderFailure(w, conn, uc, message, false)
		return
	}
	if id == "" {
		id = "chatcmpl-qoder-" + generateID()[:12]
	}
	if created == 0 {
		created = time.Now().Unix()
	}
	var output []map[string]any
	for _, index := range slices.Sorted(maps.Keys(choices)) {
		choice := choices[index]
		msg := map[string]any{"role": "assistant", "content": choice.content.String()}
		if choice.reasoning.Len() > 0 {
			msg["reasoning_content"] = choice.reasoning.String()
		}
		if len(choice.tools) > 0 {
			var calls []map[string]any
			for _, toolIndex := range slices.Sorted(maps.Keys(choice.tools)) {
				tool := choice.tools[toolIndex]
				calls = append(calls, map[string]any{
					"id": tool.id, "type": tool.kind,
					"function": map[string]any{"name": tool.name.String(), "arguments": tool.arguments.String()},
				})
			}
			msg["tool_calls"] = calls
		}
		output = append(output, map[string]any{"index": index, "message": msg, "finish_reason": choice.finishReason})
	}
	respBody := map[string]any{
		"id": id, "object": "chat.completion", "created": created,
		"model": model, "choices": output,
		"usage": lastUsage.OpenAI(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(respBody)
}

// proxyQoderStreaming unwraps Qoder's {statusCodeValue, body} SSE envelope
// into plain OpenAI SSE chunks.
func (s *Server) proxyQoderStreaming(w http.ResponseWriter, r *http.Request, resp *http.Response, model string, uc *usageContext, conn *model.ProviderConnection) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		if uc != nil {
			uc.Status = http.StatusInternalServerError
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		s.recordUsage(uc, usage.Usage{})
		return
	}

	ctx := r.Context()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		lastUsage   usage.Usage
		respBuilder strings.Builder
	)
	defer func() {
		if uc != nil {
			uc.Response = respBuilder.String()
		}
		s.recordUsage(uc, lastUsage)
	}()

	// Peek the first data line before committing HTTP 200 headers
	var (
		firstData string
		firstDone bool
		hasFirst  bool
	)
	for scanner.Scan() {
		if ctx.Err() != nil {
			if uc != nil {
				uc.Status = 499
			}
			return
		}
		text := scanner.Text()
		data, done := provider.UnwrapQoderSSELine(text, "qoder/"+model)
		if !done && data == "" {
			continue
		}
		firstData = data
		firstDone = done
		hasFirst = true
		break
	}

	if !hasFirst {
		if scanErr := scanner.Err(); scanErr != nil {
			s.writeQoderFailure(w, conn, uc, scanErr.Error(), true)
		} else {
			s.writeQoderFailure(w, conn, uc, "upstream closed without response", true)
		}
		return
	}

	// First-frame upstream error: return error response
	if firstDone && firstData != "" {
		s.writeQoderFailure(w, conn, uc, firstData, true)
		return
	}

	// Stream is healthy: commit HTTP 200 headers now
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	if firstDone {
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}

	if firstData != "" {
		if u := usage.ExtractFromSSELine([]byte(firstData)); u.TotalTokens > 0 {
			lastUsage = u
		}
		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(firstData), &chunk); err != nil {
			s.writeQoderFailure(w, conn, uc, "invalid qoder SSE chunk", true)
			return
		}
		for _, choice := range chunk.Choices {
			if respBuilder.Len() < 16384 {
				if choice.Delta.Content != "" {
					respBuilder.WriteString(choice.Delta.Content)
				} else if choice.Delta.ReasoningContent != "" {
					respBuilder.WriteString(choice.Delta.ReasoningContent)
				}
			}
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", firstData); err != nil {
			if uc != nil {
				uc.Status = 499
			}
			return
		}
		flusher.Flush()
	}

	for scanner.Scan() {
		if ctx.Err() != nil {
			if uc != nil {
				uc.Status = 499
			}
			return
		}

		data, done := provider.UnwrapQoderSSELine(scanner.Text(), "qoder/"+model)
		if done {
			if data != "" {
				s.writeQoderFailure(w, conn, uc, data, true)
				return
			}
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			// Qoder keeps the socket open after [DONE]; stop reading immediately.
			return
		}
		if data == "" {
			continue
		}
		if u := usage.ExtractFromSSELine([]byte(data)); u.TotalTokens > 0 {
			lastUsage = u
		}
		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			s.writeQoderFailure(w, conn, uc, "invalid qoder SSE chunk", true)
			return
		}
		for _, choice := range chunk.Choices {
			if respBuilder.Len() < 16384 {
				if choice.Delta.Content != "" {
					respBuilder.WriteString(choice.Delta.Content)
				} else if choice.Delta.ReasoningContent != "" {
					respBuilder.WriteString(choice.Delta.ReasoningContent)
				}
			}
		}

		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			if uc != nil {
				uc.Status = 499
			}
			return
		}
		flusher.Flush()
	}
	if ctx.Err() != nil {
		if uc != nil {
			uc.Status = 499
		}
		return
	}
	message := "qoder stream ended before completion"
	if err := scanner.Err(); err != nil {
		message = "failed to read qoder stream: " + err.Error()
	}
	s.writeQoderFailure(w, conn, uc, message, true)
}

// writeQoderFailure preserves upstream protocol errors, never completion text.
// Streaming headers may already be committed, so report failure as an SSE event.
//
// Qoder wraps auth/quota failures inside an HTTP 200 SSE envelope
// (e.g. 403 {"code":"112","pricingUrl":...}), so the transport-level status
// branch in handleQoderChat never sees them. Applying the same error-state
// classification here keeps cooldown/backoff and the panel's connection health
// in sync with what the upstream actually reported.
func (s *Server) writeQoderFailure(w http.ResponseWriter, conn *model.ProviderConnection, uc *usageContext, data string, stream bool) {
	status := http.StatusBadGateway
	var payload struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil || len(payload.Error) == 0 {
		encoded, _ := json.Marshal(map[string]any{"error": map[string]any{
			"message": data, "type": "upstream_error", "code": status,
		}})
		data = string(encoded)
	} else {
		var upstream struct {
			Code int `json:"code"`
		}
		if json.Unmarshal(payload.Error, &upstream) == nil && upstream.Code >= 400 && upstream.Code <= 599 {
			status = upstream.Code
		}
	}
	if conn != nil {
		provider.ApplyErrorState(conn, status, data)
		if s.DB != nil {
			_ = s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
		}
	}
	if uc != nil {
		uc.Status = status
	}
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(status)
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprintln(w, data)
	}
}
