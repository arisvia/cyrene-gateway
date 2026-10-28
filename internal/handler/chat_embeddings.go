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

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/cache"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

func (s *Server) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	rawBody, err := readRequestBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	var req struct {
		Provider       string          `json:"provider,omitempty"`
		Model          string          `json:"model"`
		Input          json.RawMessage `json:"input"`
		Dimensions     int             `json:"dimensions,omitempty"`
		EncodingFormat string          `json:"encoding_format,omitempty"`
	}
	if err := json.Unmarshal(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.Model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing model"})
		return
	}

	keyObj := auth.APIKeyFromContext(r.Context())
	if keyObj != nil && !keyObj.IsModelAllowed(req.Model) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": fmt.Sprintf("model '%s' is not allowed for this API key", req.Model),
		})
		return
	}

	modelInfo, err := provider.ResolveModel(req.Model, s.DB)
	if err != nil || modelInfo.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("cannot resolve model: %s", req.Model)})
		return
	}

	if req.Provider != "" {
		// The explicit override has to land on the same canonical id that
		// connections, credentials and usage rows are keyed by. The model may
		// carry either spelling as its prefix, so trim whichever one matches.
		canonical := provider.ResolveProviderAlias(req.Provider)
		modelInfo.Provider = canonical
		modelInfo.Model = req.Model
		for _, prefix := range [2]string{req.Provider + "/", canonical + "/"} {
			if trimmed, ok := strings.CutPrefix(modelInfo.Model, prefix); ok {
				modelInfo.Model = trimmed
				break
			}
		}
	}
	if provider.IsModelDisabled(req.Model, s.DB) || provider.IsModelDisabled(modelInfo.Provider+"/"+modelInfo.Model, s.DB) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": fmt.Sprintf("model is disabled: %s", req.Model)})
		return
	}

	// Phase: Response Cache check
	var settings *db.Settings
	if s.DB != nil {
		settings, _ = s.DB.GetSettings()
	}
	cacheEnabled := settings != nil && settings.ResponseCacheEnabled && s.Cache != nil
	var cacheKey string
	if cacheEnabled && !cache.ShouldBypass(r.Header) {
		eligible, key, err := cache.IsEligible("embeddings", rawBody, true)
		if err == nil && eligible {
			cacheKey = key
			if entry, hit := s.Cache.Get(key); hit {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Cyrene-Cache", "HIT")
				if entry.ServedModel != "" {
					w.Header().Set("X-Cyrene-Served-Model", entry.ServedModel)
				}
				w.WriteHeader(entry.StatusCode)
				w.Write(entry.Body)
				return
			}
		}
	}

	if cacheKey != "" {
		w.Header().Set("X-Cyrene-Cache", "MISS")
		rec := cache.NewRecorder(w)
		defer func() {
			if r := recover(); r != nil {
				panic(r)
			}
			if rec.ShouldCache() {
				ttl := time.Duration(settings.ResponseCacheTTL) * time.Second
				if ttl <= 0 {
					ttl = time.Hour
				}
				tokens := rec.ExtractTokens()
				s.Cache.Set(cacheKey, rec.StatusCode(), rec.HeaderMap(), rec.BodyBytes(), ttl, req.Model, tokens)
			}
		}()
		w = rec
	}
	providerInfo, ok := provider.GetProvider(modelInfo.Provider)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider: %s", modelInfo.Provider)})
		return
	}

	conns, err := s.DB.ListConnectionsByProvider(modelInfo.Provider)
	if err != nil || len(conns) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("no active credentials for provider: %s", modelInfo.Provider),
		})
		return
	}

	conn := s.selectAvailableConnection(conns, modelInfo.Model, nil, "")
	if conn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "all accounts rate-limited"})
		return
	}
	s.tryRefreshToken(conn)

	baseURL, effectiveAPIType := providerInfo.EffectiveBaseURL(conn.AuthType, conn.Data.APIKey != "")
	if conn.Data.BaseURL != "" {
		baseURL = conn.Data.BaseURL
		if effectiveAPIType == "" {
			effectiveAPIType = providerInfo.APIType
		}
	}

	// Forward with resolved model name while preserving all client parameters (dimensions, encoding_format, etc.)
	var bodyBytes []byte
	var bodyMap map[string]any
	if err := json.Unmarshal(rawBody, &bodyMap); err == nil {
		bodyMap["model"] = modelInfo.Model
		delete(bodyMap, "provider")
		delete(bodyMap, "prompt")
		delete(bodyMap, "query")
		bodyBytes, _ = json.Marshal(bodyMap)
	} else {
		req.Model = modelInfo.Model
		bodyBytes, _ = json.Marshal(req)
	}
	baseURL = strings.TrimSuffix(baseURL, "/chat/completions")
	baseURL = strings.TrimSuffix(baseURL, "/completions")
	targetURL := strings.TrimRight(baseURL, "/") + "/embeddings"
	if baseURL == "" {
		// The OpenAI-style /embeddings proxy needs an absolute base; providers
		// that only speak through dedicated handlers (or per-connection
		// overrides) cannot serve it. Fail clean instead of dialing "/embeddings".
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider %s has no base URL for embeddings; set a connection base URL override", modelInfo.Provider)})
		return
	}
	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upstream request"})
		return
	}

	// Unified credential path (AGENTS §7): transport headers + ApplyAuth resolve
	// API-key, OAuth bearer, and provider-specific auth per effective format —
	// the old manual `Bearer conn.Data.APIKey` silently sent OAuth-only
	// connections (and openai dual-auth) to upstream unauthenticated.
	transport := provider.ResolveTransport(providerInfo, baseURL, effectiveAPIType, conn)
	for k, v := range transport.Headers {
		upstreamReq.Header.Set(k, v)
	}
	creds := provider.ResolveCredentials(conn, modelInfo.Provider, modelInfo.Model)
	provider.ApplyAuth(upstreamReq, transport, creds)
	upstreamReq.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	client := s.getHTTPClient(2 * time.Minute)
	resp, err := client.Do(upstreamReq)
	if err != nil {
		slog.Error("embeddings upstream request failed",
			slog.String("target", targetURL),
			slog.String("connection", conn.ID),
			slog.String("error", err.Error()),
		)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("upstream request failed: %v", err)})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body = io.NopCloser(bytes.NewReader(errBody))
		provider.ApplyErrorState(conn, resp.StatusCode, string(errBody))
		s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
	}

	latencyMs := int(time.Since(startTime).Milliseconds())
	rdID := fmt.Sprintf("%d-%s", time.Now().UnixNano(), modelInfo.Model)
	rdData := map[string]any{
		"id":           rdID,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"model":        modelInfo.Model,
		"provider":     modelInfo.Provider,
		"connectionId": conn.ID,
		"status":       fmt.Sprintf("%d", resp.StatusCode),
		"latencyMs":    latencyMs,
		"endpoint":     "/v1/embeddings",
	}
	rdBytes, _ := json.Marshal(rdData)
	_ = s.DB.SaveRequestDetail(&db.RequestDetail{
		ID:           rdID,
		Timestamp:    rdData["timestamp"].(string),
		Provider:     modelInfo.Provider,
		Model:        modelInfo.Model,
		ConnectionID: conn.ID,
		Status:       rdData["status"].(string),
		Data:         string(rdBytes),
	})

	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
