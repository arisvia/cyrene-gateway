package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/provider"
	"github.com/arisvia/cyrene-gateway/internal/rtk"
)

// handleCountTokens implements the Anthropic-compatible POST /v1/messages/count_tokens endpoint.
// It provides fast local token estimation across all providers and models, with optional
// upstream forwarding when connected to official Anthropic credentials.
func (s *Server) handleCountTokens(w http.ResponseWriter, r *http.Request) {
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

	keyObj := auth.APIKeyFromContext(r.Context())
	if keyObj != nil && !keyObj.IsModelAllowed(modelStr) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": fmt.Sprintf("model '%s' is not allowed for this API key", modelStr),
		})
		return
	}

	if provider.IsModelDisabled(modelStr, s.DB) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": fmt.Sprintf("model is disabled: %s", modelStr)})
		return
	}

	// If the model belongs to an active official Anthropic connection, attempt upstream counting
	if modelInfo, err := provider.ResolveModel(modelStr, s.DB); err == nil && (modelInfo.Provider == "anthropic" || modelInfo.Provider == "claude") {
		if conns, err := s.DB.ListConnectionsByProvider(modelInfo.Provider); err == nil && len(conns) > 0 {
			if conn := s.selectAvailableConnection(conns, modelInfo.Model, nil); conn != nil {
				providerInfo, _ := provider.GetProvider(modelInfo.Provider)
				baseURL := providerInfo.BaseURL
				if conn.Data.BaseURL != "" {
					baseURL = conn.Data.BaseURL
				}
				targetURL := strings.TrimRight(baseURL, "/") + "/v1/messages/count_tokens"
				allowPrivate := s.Config != nil && s.Config.AllowPrivateNetworks
				if _, err := provider.ValidateUpstreamURL(targetURL, allowPrivate); err == nil {
					if upReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(bodyBytes)); err == nil {
						transport := provider.ResolveTransport(providerInfo, baseURL, providerInfo.APIType, conn)
						provider.ApplyAuth(upReq, transport, provider.ResolveCredentials(conn, modelInfo.Provider, modelInfo.Model))
						upReq.Header.Set("Content-Type", "application/json")
						if anthVer := r.Header.Get("anthropic-version"); anthVer != "" {
							upReq.Header.Set("anthropic-version", anthVer)
						} else {
							upReq.Header.Set("anthropic-version", "2023-06-01")
						}

						client := s.getHTTPClient(10 * time.Second)
						if resp, err := client.Do(upReq); err == nil {
							defer resp.Body.Close()
							if resp.StatusCode == http.StatusOK {
								w.Header().Set("Content-Type", "application/json")
								w.WriteHeader(http.StatusOK)
								io.Copy(w, resp.Body)
								return
							}
						}
					}
				}
			}
		}
	}

	// Local multi-lingual BPE token estimation fallback
	estimatedTokens := rtk.EstimateAnthropicTokens(reqBody)

	writeJSON(w, http.StatusOK, map[string]any{
		"input_tokens": estimatedTokens,
	})
}
