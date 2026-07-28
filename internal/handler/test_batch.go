package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// handleTestBatch tests multiple provider connections in parallel.
// POST /api/providers/test-batch
// Body: { "ids": ["id1", "id2", ...] }  (empty = test all active)
func (s *Server) handleTestBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	conns, err := s.DB.ListConnections()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list connections"})
		return
	}

	// Filter to requested IDs or all active
	idSet := make(map[string]bool, len(req.IDs))
	for _, id := range req.IDs {
		idSet[id] = true
	}

	type result struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		OK      bool   `json:"ok"`
		Latency string `json:"latency,omitempty"`
		Code    int    `json:"code,omitempty"`
		Error   string `json:"error,omitempty"`
	}

	var targets []string
	for _, c := range conns {
		if len(idSet) > 0 {
			if idSet[c.ID] {
				targets = append(targets, c.ID)
			}
		} else if c.IsActive {
			targets = append(targets, c.ID)
		}
	}

	if len(targets) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"results": []result{}, "total": 0})
		return
	}

	// Parallel test with concurrency limit
	const maxConcurrent = 10
	sem := make(chan struct{}, maxConcurrent)
	results := make([]result, len(targets))
	var wg sync.WaitGroup

	for i, id := range targets {
		wg.Add(1)
		go func(idx int, connID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			conn, err := s.DB.GetConnection(connID)
			if err != nil {
				results[idx] = result{ID: connID, OK: false, Error: "not found"}
				return
			}

			name := conn.Name
			if name == "" {
				name = conn.Provider
			}

			res := s.testConnection(r, conn)
			results[idx] = result{
				ID:      connID,
				Name:    name,
				OK:      res.OK,
				Latency: res.Latency,
				Code:    res.Code,
				Error:   res.Error,
			}
		}(i, id)
	}
	wg.Wait()

	passed := 0
	for _, res := range results {
		if res.OK {
			passed++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
		"total":   len(results),
		"passed":  passed,
		"failed":  len(results) - passed,
	})
}

// testResult is the internal result of a single connection test.
type testResult struct {
	OK      bool
	Latency string
	Code    int
	Error   string
}

// testConnection performs a single provider connection test (extracted from handleTestProvider).
func (s *Server) testConnection(r *http.Request, conn *model.ProviderConnection) testResult {
	providerInfo, ok := provider.GetProvider(conn.Provider)
	if !ok {
		return testResult{Error: "unknown provider: " + conn.Provider}
	}

	baseURL := providerInfo.BaseURL
	if conn.Data.BaseURL != "" {
		baseURL = conn.Data.BaseURL
	}
	if baseURL == "" {
		return testResult{Error: "no base URL configured"}
	}

	s.tryRefreshToken(conn)

	var targetURL string
	var testBody []byte

	switch providerInfo.APIType {
	case "anthropic":
		targetURL = provider.BuildChatURL(baseURL, "anthropic")
		testBody, _ = json.Marshal(map[string]any{
			"model":      "claude-3-haiku-20240307",
			"max_tokens": 5,
			"messages":   []any{map[string]any{"role": "user", "content": "Hi"}},
		})
	case "gemini":
		targetURL = provider.BuildGeminiURL(baseURL, "gemini-2.0-flash", false)
		testBody, _ = json.Marshal(map[string]any{
			"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "Hi"}}}},
		})
	default:
		// For OpenAI-compatible providers, test via the models endpoint.
		// Full endpoint URLs (e.g. .../v1/chat/completions) are stripped first.
		targetURL = provider.BuildModelsURL(baseURL)
		testBody = nil
	}

	var req *http.Request
	var err error
	if testBody != nil {
		req, err = http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(testBody))
	} else {
		req, err = http.NewRequestWithContext(r.Context(), "GET", targetURL, nil)
	}
	if err != nil {
		return testResult{Error: "failed to create test request"}
	}

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
	} else if providerInfo.NoAuth {
		req.Header.Set("Authorization", "Bearer public")
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range providerInfo.Headers {
		req.Header.Set(k, v)
	}

	client := s.getHTTPClient(30 * time.Second)
	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)

	if err != nil {
		conn.Data.TestStatus = "error"
		conn.Data.LastError = err.Error()
		s.DB.UpdateConnection(conn)
		return testResult{OK: false, Latency: latency.String(), Error: err.Error()}
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		provider.ResetAccountState(conn)
		s.DB.UpdateConnection(conn)
		return testResult{OK: true, Latency: latency.String(), Code: resp.StatusCode}
	}

	conn.Data.TestStatus = "error"
	conn.Data.LastError = "HTTP " + fmt.Sprintf("%d", resp.StatusCode)
	s.DB.UpdateConnection(conn)
	return testResult{OK: false, Latency: latency.String(), Code: resp.StatusCode, Error: "HTTP " + fmt.Sprintf("%d", resp.StatusCode)}
}
