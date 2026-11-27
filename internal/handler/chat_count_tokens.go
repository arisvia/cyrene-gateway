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
	if !keyAllowsModel(keyObj, modelStr) {
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
	if modelInfo, err := provider.ResolveModel(modelStr, s.DB); err == nil {
		if provider.IsModelDisabled(modelInfo.Provider+"/"+modelInfo.Model, s.DB) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": fmt.Sprintf("model is disabled: %s", modelStr)})
			return
		}
		if modelInfo.Provider == "anthropic" || modelInfo.Provider == "claude" {
			if conns, err := s.DB.ListConnectionsByProvider(modelInfo.Provider); err == nil && len(conns) > 0 {
				if conn := s.selectAvailableConnection(conns, modelInfo.Model, nil, ""); conn != nil {
					providerInfo, _ := provider.GetProvider(modelInfo.Provider)
					baseURL := providerInfo.BaseURL
					if conn.Data.BaseURL != "" {
						baseURL = conn.Data.BaseURL
					}
					trimmedBase := strings.TrimRight(baseURL, "/")
					var targetURL string
					switch {
					case strings.HasSuffix(trimmedBase, "/v1/messages") || strings.HasSuffix(trimmedBase, "/messages"):
						targetURL = trimmedBase + "/count_tokens"
					case strings.HasSuffix(trimmedBase, "/v1"):
						targetURL = trimmedBase + "/messages/count_tokens"
					default:
						targetURL = trimmedBase + "/v1/messages/count_tokens"
					}
					forwardBody := make(map[string]any, len(reqBody))
					for k, v := range reqBody {
						forwardBody[k] = v
					}
					forwardBody["model"] = modelInfo.Model
					forwardBytes, marshalErr := json.Marshal(forwardBody)
					if marshalErr != nil {
						forwardBytes = bodyBytes
					}
					allowPrivate := s.Config != nil && s.Config.AllowPrivateNetworks
					if _, err := provider.ValidateUpstreamURL(targetURL, allowPrivate); err == nil {
						if upReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(forwardBytes)); err == nil {
							transport := provider.ResolveTransport(providerInfo, baseURL, providerInfo.APIType, conn)
							for k, v := range transport.Headers {
								upReq.Header.Set(k, v)
							}
							provider.ApplyAuth(upReq, transport, provider.ResolveCredentials(conn, modelInfo.Provider, modelInfo.Model))
							upReq.Header.Set("Content-Type", "application/json")
							if anthVer := r.Header.Get("anthropic-version"); anthVer != "" {
								upReq.Header.Set("anthropic-version", anthVer)
							} else {
								upReq.Header.Set("anthropic-version", provider.AnthropicAPIVersion)
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
	}

	// Local multi-lingual BPE token estimation fallback
	estimatedTokens := rtk.EstimateAnthropicTokens(reqBody)

	writeJSON(w, http.StatusOK, map[string]any{
		"input_tokens": estimatedTokens,
	})
}
