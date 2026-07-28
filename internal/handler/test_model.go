package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// handleTestModel tests whether a specific model is reachable through the gateway.
// POST /api/models/test
// Body: { "model": "provider/model-id" }
func (s *Server) handleTestModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
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

	conn := s.selectAvailableConnection(conns, modelInfo.Model, nil)
	if conn == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "all connections rate-limited"})
		return
	}

	s.tryRefreshToken(conn)

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
	} else if providerInfo.NoAuth {
		upstreamReq.Header.Set("Authorization", "Bearer public")
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
	io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "latency": latency.String(), "code": resp.StatusCode})
	} else {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": fmt.Sprintf("HTTP %d", resp.StatusCode), "latency": latency.String(), "code": resp.StatusCode})
	}
}
