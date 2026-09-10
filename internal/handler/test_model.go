package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// handleTestModel tests whether a specific model is reachable through the gateway.
// POST /api/models/test
// Body: { "model": "provider/model-id" }
func (s *Server) handleTestModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model        string `json:"model"`
		ConnectionID string `json:"connectionId,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model is required"})
		return
	}

	modelInfo, err := provider.ResolveModel(req.Model, s.DB)
	if err != nil || modelInfo.Provider == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": fmt.Sprintf("cannot resolve model: %s", req.Model)})
		return
	}

	providerInfo, ok := provider.GetProvider(modelInfo.Provider)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": fmt.Sprintf("unknown provider: %s", modelInfo.Provider)})
		return
	}

	conns, err := s.DB.ListConnectionsByProvider(modelInfo.Provider)
	if err != nil || len(conns) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "no connections for provider"})
		return
	}

	var conn *model.ProviderConnection
	if req.ConnectionID != "" {
		c, err := s.DB.GetConnection(req.ConnectionID)
		if err == nil && c != nil && c.IsActive {
			conn = c
		}
	}
	if conn == nil {
		conn = s.selectAvailableConnection(conns, modelInfo.Model, nil)
	}
	if conn == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "all connections rate-limited"})
		return
	}

	s.tryRefreshToken(conn)

	if conn.Provider == "antigravity" {
		start := time.Now()
		client := s.getHTTPClient(15 * time.Second)
		projID := ""
		if conn.Data.ProviderSpecificData != nil {
			if p, ok := conn.Data.ProviderSpecificData["projectId"].(string); ok && p != "" {
				projID = p
			}
		}
		if projID == "" {
			var err error
			projID, err = DiscoverAntigravityProject(r.Context(), client, conn.Data.AccessToken)
			if err != nil {
				writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "latency": time.Since(start).String()})
				return
			}
			if conn.Data.ProviderSpecificData == nil {
				conn.Data.ProviderSpecificData = make(map[string]any)
			}
			conn.Data.ProviderSpecificData["projectId"] = projID
			s.DB.UpdateConnection(conn)
		}

		availableModels := s.getAntigravityAvailableModels()
		targetModel, _, _, isImage := resolveAntigravityModel(modelInfo.Model, "", availableModels)
		reqType := "agent"
		upstreamAction := "streamGenerateContent?alt=sse"
		if isImage {
			reqType = "image_gen"
			upstreamAction = "generateContent"
		}
		innerReq := map[string]any{
			"contents": []map[string]any{
				{
					"role":  "user",
					"parts": []map[string]any{{"text": "Hi"}},
				},
			},
			"generationConfig": map[string]any{
				"maxOutputTokens": 5,
			},
		}
		envelope := map[string]any{
			"project":     projID,
			"model":       targetModel,
			"request":     innerReq,
			"requestType": reqType,
			"userAgent":   "antigravity",
			"requestId":   fmt.Sprintf("test-%d", time.Now().UnixMilli()),
		}
		envelopeBytes, _ := json.Marshal(envelope)
		upstreamURL := fmt.Sprintf("%s/v1internal:%s", provider.AntigravityBaseURL, upstreamAction)
		upReq, err := http.NewRequestWithContext(r.Context(), "POST", upstreamURL, bytes.NewReader(envelopeBytes))
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "latency": time.Since(start).String()})
			return
		}
		upReq.Header.Set("Authorization", "Bearer "+conn.Data.AccessToken)
		upReq.Header.Set("Content-Type", "application/json")
		upReq.Header.Set("Accept", "text/event-stream")
		upReq.Header.Set("User-Agent", provider.AntigravityUserAgent)
		upReq.Header.Set("X-Goog-Api-Client", provider.AntigravityXGoogClient)
		upReq.Header.Set("Client-Metadata", provider.AntigravityMetadata)

		resp, err := client.Do(upReq)
		latency := time.Since(start)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "latency": latency.String()})
			return
		}
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "latency": latency.String(), "code": resp.StatusCode})
		} else {
			errMsg := parseUpstreamErrorMessage(resp.StatusCode, errBody)
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": errMsg, "latency": latency.String(), "code": resp.StatusCode})
		}
		return
	}

	baseURL, effectiveAPIType := providerInfo.EffectiveBaseURL(conn.AuthType, conn.Data.APIKey != "")
	if conn.Data.BaseURL != "" {
		baseURL = conn.Data.BaseURL
		effectiveAPIType = providerInfo.APIType
	}
	if baseURL == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "no base URL configured"})
		return
	}

	// Build a minimal chat completion request with the specific model
	var targetURL string
	var testBody []byte

	switch effectiveAPIType {
	case "anthropic":
		targetURL = provider.BuildChatURL(baseURL, "anthropic")
		testBody, _ = json.Marshal(map[string]any{
			"model":      modelInfo.Model,
			"max_tokens": 5,
			"messages":   []any{map[string]any{"role": "user", "content": "Hi"}},
		})
	case "gemini":
		targetURL = provider.BuildGeminiURL(baseURL, modelInfo.Model, false)
		testBody, _ = json.Marshal(map[string]any{
			"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "Hi"}}}},
		})
	default:
		targetURL = provider.BuildChatURL(baseURL, effectiveAPIType)
		testBody, _ = json.Marshal(map[string]any{
			"model":      modelInfo.Model,
			"max_tokens": 5,
			"messages":   []any{map[string]any{"role": "user", "content": "Hi"}},
		})
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(testBody))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "failed to create request"})
		return
	}

	if conn.Data.APIKey != "" {
		if effectiveAPIType == "anthropic" {
			upstreamReq.Header.Set("x-api-key", conn.Data.APIKey)
			upstreamReq.Header.Set("anthropic-version", "2023-06-01")
		} else if effectiveAPIType == "gemini" {
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
	for k, v := range providerInfo.Headers {
		upstreamReq.Header.Set(k, v)
	}

	client := s.getHTTPClient(30 * time.Second)
	start := time.Now()
	resp, err := client.Do(upstreamReq)
	latency := time.Since(start)

	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "latency": latency.String()})
		return
	}
	defer resp.Body.Close()
	errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "latency": latency.String(), "code": resp.StatusCode})
	} else {
		errMsg := parseUpstreamErrorMessage(resp.StatusCode, errBody)
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": errMsg, "latency": latency.String(), "code": resp.StatusCode})
	}
}

func parseUpstreamErrorMessage(statusCode int, body []byte) string {
	defaultMsg := fmt.Sprintf("HTTP %d", statusCode)
	if len(body) == 0 {
		return defaultMsg
	}
	var errObj struct {
		Error   any    `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &errObj); err == nil {
		if msg, ok := errObj.Error.(string); ok && msg != "" {
			return fmt.Sprintf("HTTP %d: %s", statusCode, msg)
		} else if errMap, ok := errObj.Error.(map[string]any); ok {
			if m, ok := errMap["message"].(string); ok && m != "" {
				return fmt.Sprintf("HTTP %d: %s", statusCode, m)
			}
		} else if errObj.Message != "" {
			return fmt.Sprintf("HTTP %d: %s", statusCode, errObj.Message)
		}
	}
	return defaultMsg
}
