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

	if userID == "" || conn.Data.AccessToken == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "qoder credential is missing userId or accessToken; reconnect the account via OAuth",
		})
		return
	}

	creds := provider.QoderCosyCreds{
		UserID:    userID,
		AuthToken: conn.Data.AccessToken,
		Name:      conn.Name,
		Email:     conn.Email,
		MachineID: machineID,
	}

	// Build the request body map from the raw body to preserve unknown fields
	var bodyMap map[string]any
	json.Unmarshal(rawBody, &bodyMap)

	client := s.getHTTPClient(5 * time.Minute)

	encodedBody, qoderKey, err := provider.BuildQoderRequestBody(modelInfo.Model, bodyMap, creds, client)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	cosyHeaders, err := provider.BuildQoderCosyHeaders(encodedBody, provider.QoderChatURL, creds)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": fmt.Sprintf("qoder cosy signing failed: %v", err),
		})
		return
	}

	modelSource := "system"

	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", provider.QoderChatURL, bytes.NewReader(encodedBody))
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
	}

	// Stream with envelope unwrapping
	s.proxyQoderStreaming(w, r, resp, qoderKey, uc)
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
	doneEmitted := false

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

		if doneEmitted {
			continue
		}

		line := scanner.Text()
		data, done := provider.UnwrapQoderSSELine(line, "qoder/"+model)

		if done {
			if lastUsage.TotalTokens > 0 {
				s.recordUsage(uc, lastUsage)
			}
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			doneEmitted = true
			continue
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

	if !doneEmitted {
		if lastUsage.TotalTokens > 0 {
			s.recordUsage(uc, lastUsage)
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}
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
