package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
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

// handleEnableFreeProviders creates a NoAuth connection for every registry
// provider that requires no authentication and does not already have one.
// This powers the panel's one-click "start with free providers" onboarding.
// POST /api/providers/enable-free
// Body (optional): { "providers": ["id1", "id2"] } — empty = all NoAuth providers
func (s *Server) handleEnableFreeProviders(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Providers []string `json:"providers"`
	}
	// Body is optional; ignore decode errors on an empty body.
	_ = json.NewDecoder(r.Body).Decode(&req)

	want := make(map[string]bool, len(req.Providers))
	for _, id := range req.Providers {
		want[id] = true
	}

	existing, err := s.DB.ListConnections()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list connections"})
		return
	}
	already := make(map[string]bool, len(existing))
	for _, c := range existing {
		already[c.Provider] = true
	}

	enabled := make([]string, 0)
	skipped := make([]string, 0)
	for id, info := range provider.Registry {
		if !info.NoAuth {
			continue
		}
		if len(want) > 0 && !want[id] {
			continue
		}
		if already[id] {
			skipped = append(skipped, id)
			continue
		}
		pc := &model.ProviderConnection{
			ID:       generateID(),
			Provider: id,
			AuthType: "none",
			Name:     info.Name,
			Priority: info.Priority,
			IsActive: true,
		}
		if err := s.DB.CreateConnection(pc); err != nil {
			slog.Error("failed to enable free provider", "provider", id, "error", err)
			continue
		}
		enabled = append(enabled, id)
	}
	sort.Strings(enabled)
	sort.Strings(skipped)

	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": enabled,
		"skipped": skipped,
		"count":   len(enabled),
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

	baseURL, effectiveAPIType := providerInfo.EffectiveBaseURL(conn.AuthType, conn.Data.APIKey != "")
	if conn.Data.BaseURL != "" {
		baseURL = conn.Data.BaseURL
		effectiveAPIType = providerInfo.APIType
	}
	if baseURL == "" {
		return testResult{Error: "no base URL configured"}
	}

	s.tryRefreshToken(conn)

	// Phase 30: resolve the provider transport and use it for both URL building
	// and auth injection so connection tests exercise the real upstream path.
	transport := provider.ResolveTransport(providerInfo, baseURL, effectiveAPIType, conn)

	var targetURL string
	var testBody []byte

	switch effectiveAPIType {
	case "anthropic":
		targetURL = provider.BuildTransportURL(transport, "claude-3-haiku-20240307", false)
		testBody, _ = json.Marshal(map[string]any{
			"model":      "claude-3-haiku-20240307",
			"max_tokens": 5,
			"messages":   []any{map[string]any{"role": "user", "content": "Hi"}},
		})
	case "gemini":
		targetURL = provider.BuildTransportURL(transport, "gemini-2.0-flash", false)
		testBody, _ = json.Marshal(map[string]any{
			"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "Hi"}}}},
		})
	default:
		// For OpenAI-compatible providers, test via the models endpoint.
		// Full endpoint URLs (e.g. .../v1/chat/completions) are stripped first.
		targetURL = provider.BuildModelsURL(baseURL)
		if transport.URLSuffix != "" {
			targetURL = strings.TrimRight(baseURL, "/") + transport.URLSuffix
		}
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

	for k, v := range transport.Headers {
		req.Header.Set(k, v)
	}
	creds := provider.Credentials{
		APIKey:               conn.Data.APIKey,
		AccessToken:          conn.Data.AccessToken,
		ProviderSpecificData: conn.Data.ProviderSpecificData,
	}
	provider.ApplyAuth(req, transport, creds)
	if providerInfo.NoAuth && creds.APIKey == "" && creds.AccessToken == "" {
		req.Header.Set("Authorization", "Bearer public")
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
