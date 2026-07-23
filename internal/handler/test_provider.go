package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// handleTestProvider tests a provider connection by sending a minimal request.
func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conn, err := s.DB.GetConnection(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	providerInfo, ok := provider.GetProvider(conn.Provider)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider: %s", conn.Provider)})
		return
	}

	baseURL := providerInfo.BaseURL
	if conn.Data.BaseURL != "" {
		baseURL = conn.Data.BaseURL
	}
	if baseURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no base URL configured"})
		return
	}

	// Phase 9: Pre-check token refresh before testing
	s.tryRefreshToken(conn)

	// Build a minimal test request (list models or simple completion)
	var targetURL string
	var testBody []byte

	switch providerInfo.APIType {
	case "anthropic":
		targetURL = strings.TrimRight(baseURL, "/") + "/v1/messages"
		testBody, _ = json.Marshal(map[string]any{
			"model":      "claude-3-haiku-20240307",
			"max_tokens": 5,
			"messages":   []any{map[string]any{"role": "user", "content": "Hi"}},
		})
	case "gemini":
		targetURL = strings.TrimRight(baseURL, "/") + "/v1beta/models/gemini-2.0-flash:generateContent"
		testBody, _ = json.Marshal(map[string]any{
			"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "Hi"}}}},
		})
	default:
		// OpenAI-compatible: use /models endpoint (lightweight)
		targetURL = strings.TrimRight(baseURL, "/") + "/models"
		testBody = nil
	}

	var req *http.Request
	if testBody != nil {
		req, err = http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(testBody))
	} else {
		req, err = http.NewRequestWithContext(r.Context(), "GET", targetURL, nil)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create test request"})
		return
	}

	// Set auth headers
	if conn.Data.APIKey != "" {
		if providerInfo.APIType == "anthropic" {
			req.Header.Set("x-api-key", conn.Data.APIKey)
			req.Header.Set("anthropic-version", "2023-06-01")
		} else if providerInfo.APIType == "gemini" {
			q := req.URL.Query()
			q.Set("key", conn.Data.APIKey)
			req.URL.RawQuery = q.Encode()
		} else {
			req.Header.Set("Authorization", "Bearer "+conn.Data.APIKey)
		}
	} else if conn.Data.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+conn.Data.AccessToken)
	}
	req.Header.Set("Content-Type", "application/json")

	client := s.getHTTPClient(30 * time.Second)
	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)

	if err != nil {
		conn.Data.TestStatus = "error"
		conn.Data.LastError = err.Error()
		s.DB.UpdateConnection(conn)

		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"status":  "error",
			"error":   err.Error(),
			"latency": latency.String(),
		})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		provider.ResetAccountState(conn)
		s.DB.UpdateConnection(conn)

		slog.Info("Provider test passed",
			slog.String("provider", conn.Provider),
			slog.String("connection", conn.ID),
			slog.Int("status", resp.StatusCode),
		)

		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"status":  "active",
			"code":    resp.StatusCode,
			"latency": latency.String(),
		})
	} else {
		conn.Data.TestStatus = "error"
		conn.Data.LastError = string(body)
		s.DB.UpdateConnection(conn)

		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"status":  "error",
			"code":    resp.StatusCode,
			"error":   string(body),
			"latency": latency.String(),
		})
	}
}
