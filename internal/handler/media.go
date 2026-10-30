package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/media"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
	"github.com/arisvia/cyrene-gateway/internal/translator"
)

// extractRoutingFields parses a JSON body, detects case-conflicting routing
// fields (e.g. both "model" and "Model"), and returns a normalised body with
// canonical lowercase "model"/"provider" keys plus the extracted values.
// Using map[string]json.RawMessage preserves numeric precision for unrelated
// fields (e.g. large integers that would lose precision through float64).
func extractRoutingFields(bodyBytes []byte) (cleanBody []byte, bodyMap map[string]json.RawMessage, modelName, providerName string, err error) {
	var raw map[string]json.RawMessage
	if err = json.Unmarshal(bodyBytes, &raw); err != nil {
		return nil, nil, "", "", err
	}

	// Detect case-insensitive duplicates for "model" and "provider".
	for key := range raw {
		lower := strings.ToLower(key)
		if lower != "model" && lower != "provider" {
			continue
		}
		for other := range raw {
			if other != key && strings.EqualFold(other, key) {
				return nil, nil, "", "", fmt.Errorf("ambiguous routing field: both %q and %q present", key, other)
			}
		}
	}

	// Extract values from whichever case variant exists.
	for key, val := range raw {
		if strings.ToLower(key) == "model" {
			json.Unmarshal(val, &modelName)
		}
		if strings.ToLower(key) == "provider" {
			json.Unmarshal(val, &providerName)
		}
	}

	// Rebuild body: remove non-canonical routing keys, insert lowercase.
	normalized := make(map[string]json.RawMessage, len(raw))
	for k, v := range raw {
		lower := strings.ToLower(k)
		if lower == "model" || lower == "provider" {
			continue
		}
		normalized[k] = v
	}
	if modelName != "" {
		b, _ := json.Marshal(modelName)
		normalized["model"] = b
	}
	if providerName != "" {
		b, _ := json.Marshal(providerName)
		normalized["provider"] = b
	}

	cleanBody, err = json.Marshal(normalized)
	if err != nil {
		return nil, nil, "", "", err
	}
	// The routing id drives connection lookups, SupportsKind and credential
	// branches, so a shorthand must land on the canonical id. The forwarded body
	// above keeps the client's own spelling.
	providerName = provider.ResolveProviderAlias(providerName)
	return cleanBody, normalized, modelName, providerName, nil
}

// checkMediaModelAccess is applied to both the requested and resolved model.
// An empty model is only allowed for keys without model restrictions.
func (s *Server) checkMediaModelAccess(w http.ResponseWriter, r *http.Request, modelName string) bool {
	if keyObj := auth.APIKeyFromContext(r.Context()); keyObj != nil && !keyObj.IsModelAllowed(modelName) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": fmt.Sprintf("model '%s' is not allowed for this API key", modelName),
		})
		return false
	}
	if modelName != "" && provider.IsModelDisabled(modelName, s.DB) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": fmt.Sprintf("model is disabled: %s", modelName)})
		return false
	}
	return true
}

// handleImageGeneration handles POST /v1/images/generations
func (s *Server) handleImageGeneration(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := readRequestBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	bodyBytes, bodyMap, modelName, _, err := extractRoutingFields(bodyBytes)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if modelName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing model"})
		return
	}

	if !s.checkMediaModelAccess(w, r, modelName) {
		return
	}
	modelInfo, err := provider.ResolveModel(modelName, s.DB)
	if err != nil || modelInfo.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("cannot resolve model: %s", modelName)})
		return
	}
	if !s.checkMediaModelAccess(w, r, modelInfo.Provider+"/"+modelInfo.Model) {
		return
	}

	if !media.SupportsKind(modelInfo.Provider, media.KindImage) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider '%s' does not support image generation", modelInfo.Provider)})
		return
	}

	conns, first := s.resolveMediaPool(modelInfo.Provider)
	if first == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("no active credentials for provider: %s", modelInfo.Provider)})
		return
	}

	// Replace model in body with resolved model
	bodyMap["model"], _ = json.Marshal(modelInfo.Model)
	bodyBytes, _ = json.Marshal(bodyMap)

	cfg := media.GetConfig(modelInfo.Provider, media.KindImage)
	resp, _, status, errBody := s.walkMediaAccounts(r, modelInfo.Provider, modelInfo.Model, "/v1/images/generations", conns, first,
		func(ctx context.Context, conn *model.ProviderConnection) (*http.Response, error) {
			return s.MediaClient.HandleImageGeneration(ctx, modelInfo.Provider, bodyBytes, mediaCreds(conn))
		})
	if resp == nil {
		writeMediaTerminal(w, status, errBody, modelInfo.Provider)
		return
	}
	defer resp.Body.Close()

	if modelInfo.Provider == "antigravity" {
		// Antigravity streamGenerateContent returns SSE event stream with inlineData
		s.aggregateAntigravityImageResponse(w, resp)
		return
	}
	if cfg != nil && media.NeedsImageNormalization(cfg.Format) {
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to read upstream image response"})
			return
		}
		out, err := media.NormalizeImage(cfg.Format, raw)
		if err != nil {
			slog.Error("Image response normalization failed", "provider", modelInfo.Provider, "error", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, json.RawMessage(out))
		return
	}

	s.proxyMediaResponse(w, resp)
}

// handleAudioSpeech handles POST /v1/audio/speech (TTS)
func (s *Server) handleAudioSpeech(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := readRequestBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	bodyBytes, bodyMap, modelName, _, err := extractRoutingFields(bodyBytes)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if modelName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing model"})
		return
	}

	if !s.checkMediaModelAccess(w, r, modelName) {
		return
	}
	modelInfo, err := provider.ResolveModel(modelName, s.DB)
	if err != nil || modelInfo.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("cannot resolve model: %s", modelName)})
		return
	}
	if !s.checkMediaModelAccess(w, r, modelInfo.Provider+"/"+modelInfo.Model) {
		return
	}

	if !media.SupportsKind(modelInfo.Provider, media.KindTTS) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider '%s' does not support TTS", modelInfo.Provider)})
		return
	}

	conns, first := s.resolveMediaPool(modelInfo.Provider)
	if first == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("no active credentials for provider: %s", modelInfo.Provider)})
		return
	}

	bodyMap["model"], _ = json.Marshal(modelInfo.Model)
	bodyBytes, _ = json.Marshal(bodyMap)

	cfg := media.GetConfig(modelInfo.Provider, media.KindTTS)
	resp, _, status, errBody := s.walkMediaAccounts(r, modelInfo.Provider, modelInfo.Model, "/v1/audio/speech", conns, first,
		func(ctx context.Context, conn *model.ProviderConnection) (*http.Response, error) {
			return s.MediaClient.HandleTTS(ctx, modelInfo.Provider, bodyBytes, mediaCreds(conn))
		})
	if resp == nil {
		writeMediaTerminal(w, status, errBody, modelInfo.Provider)
		return
	}
	defer resp.Body.Close()

	if cfg != nil && media.NeedsAudioNormalization(cfg.Format) {
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to read upstream audio response"})
			return
		}
		audioBytes, mime, err := media.NormalizeAudio(cfg.Format, raw)
		if err != nil {
			slog.Error("TTS response normalization failed", "provider", modelInfo.Provider, "error", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		if mime != "" {
			w.Header().Set("Content-Type", mime)
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		w.Write(audioBytes)
		return
	}

	s.proxyMediaResponse(w, resp)
}

// handleAudioTranscriptions handles POST /v1/audio/transcriptions (STT)
func (s *Server) handleAudioTranscriptions(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse multipart form"})
		return
	}

	modelStr := r.FormValue("model")
	if modelStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing model"})
		return
	}

	if !s.checkMediaModelAccess(w, r, modelStr) {
		return
	}
	modelInfo, err := provider.ResolveModel(modelStr, s.DB)
	if err != nil || modelInfo.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("cannot resolve model: %s", modelStr)})
		return
	}
	if !s.checkMediaModelAccess(w, r, modelInfo.Provider+"/"+modelInfo.Model) {
		return
	}

	if !media.SupportsKind(modelInfo.Provider, media.KindSTT) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider '%s' does not support STT", modelInfo.Provider)})
		return
	}

	conns, first := s.resolveMediaPool(modelInfo.Provider)
	if first == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("no active credentials for provider: %s", modelInfo.Provider)})
		return
	}

	cfg := media.GetConfig(modelInfo.Provider, media.KindSTT)
	resp, _, status, errBody := s.walkMediaAccounts(r, modelInfo.Provider, modelInfo.Model, "/v1/audio/transcriptions", conns, first,
		func(ctx context.Context, conn *model.ProviderConnection) (*http.Response, error) {
			return s.MediaClient.HandleSTT(ctx, modelInfo.Provider, modelInfo.Model, r, mediaCreds(conn))
		})
	if resp == nil {
		writeMediaTerminal(w, status, errBody, modelInfo.Provider)
		return
	}
	defer resp.Body.Close()

	if cfg != nil && media.NeedsSTTNormalization(cfg.Format) {
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to read upstream transcript response"})
			return
		}
		out, err := media.NormalizeSTT(cfg.Format, raw)
		if err != nil {
			slog.Error("STT response normalization failed", "provider", modelInfo.Provider, "error", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, json.RawMessage(out))
		return
	}

	s.proxyMediaResponse(w, resp)
}

// handleVideoGenerations handles POST /v1/videos/generations
func (s *Server) handleVideoGenerations(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := readRequestBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	bodyBytes, bodyMap, modelName, providerID, err := extractRoutingFields(bodyBytes)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if !s.checkMediaModelAccess(w, r, modelName) {
		return
	}
	resolvedModel := modelName
	if modelName != "" {
		modelInfo, err := provider.ResolveModel(modelName, s.DB)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("cannot resolve model: %s", modelName)})
			return
		}
		resolvedModel = modelInfo.Model
		if resolvedModel == "" && !s.checkMediaModelAccess(w, r, "") {
			return
		}
		if providerID == "" {
			providerID = modelInfo.Provider
		} else if modelInfo.Provider != "" && modelInfo.Provider != providerID && !s.checkMediaModelAccess(w, r, modelInfo.Provider+"/"+resolvedModel) {
			return
		}
		if !s.checkMediaModelAccess(w, r, providerID+"/"+resolvedModel) {
			return
		}
	}
	if providerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing provider or model"})
		return
	}

	if !media.SupportsKind(providerID, media.KindVideo) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider '%s' does not support video generation", providerID)})
		return
	}

	conns, first := s.resolveMediaPool(providerID)
	if first == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("no active credentials for provider: %s", providerID)})
		return
	}

	if resolvedModel != modelName {
		bodyMap["model"], _ = json.Marshal(resolvedModel)
		bodyBytes, _ = json.Marshal(bodyMap)
	}

	resp, connID, status, errBody := s.walkMediaAccounts(r, providerID, modelName, "/v1/videos/generations", conns, first,
		func(ctx context.Context, conn *model.ProviderConnection) (*http.Response, error) {
			return s.MediaClient.HandleVideo(ctx, providerID, "generations", bodyBytes, mediaCreds(conn))
		})
	if resp == nil {
		writeMediaTerminal(w, status, errBody, providerID)
		return
	}
	defer resp.Body.Close()

	// Bind the created job to the account that produced it so status polling
	// reaches the same credentials without a provider hint.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if raw, err := io.ReadAll(resp.Body); err == nil {
			var job struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(raw, &job) == nil && job.ID != "" {
				s.registerVideoJob(job.ID, providerID, connID)
			}
			resp.Body = io.NopCloser(bytes.NewReader(raw))
		}
	}

	s.proxyMediaResponse(w, resp)
}

// handleVideoStatus handles GET /v1/videos/{id}
func (s *Server) handleVideoStatus(w http.ResponseWriter, r *http.Request) {
	requestID := r.PathValue("id")
	providerID := provider.ResolveProviderAlias(r.URL.Query().Get("provider"))
	if providerID == "" {
		// The job registry makes the provider hint optional: a job ID created
		// through this gateway resolves to its owning account.
		if ref, ok := s.lookupVideoJob(requestID); ok {
			providerID = ref.Provider
		}
	}
	if providerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing provider query param"})
		return
	}
	// A job ID alone cannot prove which model was used.
	if !s.checkMediaModelAccess(w, r, "") {
		return
	}

	conns, first := s.resolveMediaPool(providerID)
	if first == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("no active credentials for provider: %s", providerID)})
		return
	}
	// Prefer the exact account that created the job when it is still active.
	if ref, ok := s.lookupVideoJob(requestID); ok && ref.Connection != "" {
		for i := range conns {
			if conns[i].ID == ref.Connection && conns[i].IsActive {
				first = &conns[i]
				break
			}
		}
	}

	// A job ID is account-bound: one attempt, no pool walk (a 404 on another
	// account is expected, not a failover signal).
	resp, _, status, errBody := s.attemptMediaRequest(r, first, providerID, "", "/v1/videos/"+requestID,
		func(ctx context.Context, conn *model.ProviderConnection) (*http.Response, error) {
			return s.MediaClient.HandleVideoStatus(ctx, providerID, requestID, mediaCreds(conn))
		})
	if resp == nil {
		writeMediaTerminal(w, status, errBody, providerID)
		return
	}
	defer resp.Body.Close()

	s.proxyMediaResponse(w, resp)
}

// handleWebFetch handles POST /v1/web/fetch
func (s *Server) handleWebFetch(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := readRequestBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	var req struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.Provider == "" && req.Model != "" {
		if idx := strings.Index(req.Model, "/"); idx != -1 {
			req.Provider = req.Model[:idx]
		} else if media.SupportsKind(req.Model, media.KindWebFetch) {
			req.Provider = req.Model
		}
	}
	if req.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing provider"})
		return
	}
	req.Provider = provider.ResolveProviderAlias(req.Provider)

	if !media.SupportsKind(req.Provider, media.KindWebFetch) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider '%s' does not support web fetch", req.Provider)})
		return
	}

	conns, first := s.resolveMediaPool(req.Provider)
	if first == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("no active credentials for provider: %s", req.Provider)})
		return
	}

	resp, _, status, errBody := s.walkMediaAccounts(r, req.Provider, "", "/v1/web/fetch", conns, first,
		func(ctx context.Context, conn *model.ProviderConnection) (*http.Response, error) {
			return s.MediaClient.HandleWebFetch(ctx, req.Provider, bodyBytes, mediaCreds(conn))
		})
	if resp == nil {
		writeMediaTerminal(w, status, errBody, req.Provider)
		return
	}
	defer resp.Body.Close()

	s.proxyMediaResponse(w, resp)
}

// handleWebSearch handles POST /v1/search
func (s *Server) handleWebSearch(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := readRequestBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	bodyBytes, bodyMap, modelName, providerID, err := extractRoutingFields(bodyBytes)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if providerID == "" && modelName != "" {
		if idx := strings.Index(modelName, "/"); idx != -1 {
			providerID = modelName[:idx]
		} else if media.SupportsKind(modelName, media.KindWebSearch) {
			providerID = modelName
		}
		providerID = provider.ResolveProviderAlias(providerID)
	}
	if providerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing provider"})
		return
	}

	if !media.SupportsKind(providerID, media.KindWebSearch) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider '%s' does not support web search", providerID)})
		return
	}
	// Authorize the same default model that the Antigravity client would use,
	// skipping engines the operator disabled for chat cost control.
	if modelName == "" && providerID == "antigravity" {
		modelName = s.defaultSearchModel(providerID)
		if modelName == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "all web search engines are disabled"})
			return
		}
	}
	if modelName != "" {
		if !s.checkMediaModelAccess(w, r, modelName) {
			return
		}
		modelInfo, err := provider.ResolveModel(modelName, s.DB)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("cannot resolve model: %s", modelName)})
			return
		}
		if modelInfo.Model == "" && !s.checkMediaModelAccess(w, r, "") {
			return
		}
		if modelInfo.Provider != "" && modelInfo.Provider != providerID && !s.checkMediaModelAccess(w, r, modelInfo.Provider+"/"+modelInfo.Model) {
			return
		}
		if !s.checkMediaModelAccess(w, r, providerID+"/"+modelInfo.Model) {
			return
		}
		bodyMap["model"], _ = json.Marshal(modelInfo.Model)
		bodyBytes, _ = json.Marshal(bodyMap)
	}

	conns, first := s.resolveMediaPool(providerID)
	if first == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("no active credentials for provider: %s", providerID)})
		return
	}

	var queryStr string
	if q, ok := bodyMap["query"]; ok {
		json.Unmarshal(q, &queryStr)
	}

	resp, _, status, errBody := s.walkMediaAccounts(r, providerID, modelName, "/v1/search", conns, first,
		func(ctx context.Context, conn *model.ProviderConnection) (*http.Response, error) {
			return s.MediaClient.HandleWebSearch(ctx, providerID, bodyBytes, mediaCreds(conn))
		})
	if resp == nil {
		writeMediaTerminal(w, status, errBody, providerID)
		return
	}
	defer resp.Body.Close()

	if providerID == "antigravity" {
		s.aggregateAntigravitySearchResponse(w, resp, queryStr)
		return
	}

	s.proxyMediaResponse(w, resp)
}

// defaultSearchModel picks the first enabled web-search engine for a
// provider: static registry entries first, then live Gemini-family chat
// engines from the provider model cache.
func (s *Server) defaultSearchModel(providerID string) string {
	cfg := media.GetConfig(providerID, media.KindWebSearch)
	if cfg == nil {
		return ""
	}
	disabledMap, _ := s.DB.KVList(model.KVScopeDisabledModels)
	isDisabled := func(fullID string) bool { return disabledMap != nil && disabledMap[fullID] != "" }
	for _, p := range media.GetProvidersByKind(media.KindWebSearch) {
		if p.Provider != providerID {
			continue
		}
		for _, m := range p.Models {
			if m.Kind == media.KindWebSearch && !isDisabled(providerID+"/"+m.ID) {
				return providerID + "/" + m.ID
			}
		}
	}
	if raw, err := s.DB.KVGet(model.KVScopeProviderModelCache, providerID); err == nil && raw != "" {
		var cached model.CachedModels
		if json.Unmarshal([]byte(raw), &cached) == nil {
			for _, cm := range cached.Models {
				lowerID := strings.ToLower(cm.ID)
				if providerID == "antigravity" && strings.HasPrefix(lowerID, "gemini-") &&
					!strings.Contains(lowerID, "-image") &&
					slices.Contains(cm.Capabilities, "chat") &&
					!isDisabled(providerID+"/"+cm.ID) {
					return providerID + "/" + cm.ID
				}
			}
		}
	}
	return ""
}

// handleMediaProviders handles GET /api/media-providers
func (s *Server) handleMediaProviders(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	connectedOnly := r.URL.Query().Get("connectedOnly") == "true" || r.URL.Query().Get("connected") == "true"

	// Pre-fetch active connection counts, primary connection IDs, and authTypes per provider
	activeConns, _ := s.DB.ListConnections()
	connCountByProvider := make(map[string]int)
	primaryConnByProvider := make(map[string]string)
	authTypeByProvider := make(map[string]string)
	for _, c := range activeConns {
		if c.IsActive {
			connCountByProvider[c.Provider]++
			if primaryConnByProvider[c.Provider] == "" {
				primaryConnByProvider[c.Provider] = c.ID
			}
			if authTypeByProvider[c.Provider] == "" && c.AuthType != "" {
				authTypeByProvider[c.Provider] = c.AuthType
			}
		}
	}
	type EnrichedProvider struct {
		Provider            string                              `json:"provider"`
		Name                string                              `json:"name"`
		Kinds               []media.Kind                        `json:"kinds"`
		Models              []media.ModelEntry                  `json:"models,omitempty"`
		Configs             map[media.Kind]media.ProviderConfig `json:"configs,omitempty"`
		ActiveConnections   int                                 `json:"activeConnections"`
		HasConnection       bool                                `json:"hasConnection"`
		PrimaryConnectionID string                              `json:"primaryConnectionId,omitempty"`
		AuthType            string                              `json:"authType,omitempty"`
	}

	enrichList := func(entries []*media.MediaProviderInfo, filterKind media.Kind) []EnrichedProvider {
		var out []EnrichedProvider
		disabledMap, _ := s.DB.KVList(model.KVScopeDisabledModels)
		isModelDisabled := func(fullID, bareID string) bool {
			if disabledMap == nil {
				return false
			}
			return disabledMap[fullID] != "" || disabledMap[bareID] != ""
		}
		for _, e := range entries {
			count := connCountByProvider[e.Provider]
			has := count > 0
			if connectedOnly && !has {
				continue
			}
			// 按当前 Tab 的能力过滤 models，避免多能力提供商把 image/tts/stt 等所有 model 混在一起返回
			seenModel := make(map[string]bool)
			var matchedModels []media.ModelEntry
			for _, m := range e.Models {
				fullID := e.Provider + "/" + m.ID
				if isModelDisabled(fullID, m.ID) {
					continue
				}
				if filterKind == "" || m.Kind == filterKind {
					seenModel[m.ID] = true
					matchedModels = append(matchedModels, m)
				}
			}

			// 动态提取：从 providerModelCache 融合上游实时同步出的多媒体能力模型
			if raw, err := s.DB.KVGet(model.KVScopeProviderModelCache, e.Provider); err == nil && raw != "" {
				var cached model.CachedModels
				if err := json.Unmarshal([]byte(raw), &cached); err == nil && len(cached.Models) > 0 {
					for _, cm := range cached.Models {
						fullID := e.Provider + "/" + cm.ID
						if isModelDisabled(fullID, cm.ID) {
							continue
						}
						lowerID := strings.ToLower(cm.ID)
						match := false
						switch filterKind {
						case media.KindImage:
							match = slices.Contains(cm.Capabilities, "image-generation") ||
								strings.Contains(lowerID, "-image") ||
								strings.Contains(lowerID, "dall-e") ||
								strings.Contains(lowerID, "flux")
						case media.KindEmbedding:
							match = slices.Contains(cm.Capabilities, "embeddings") ||
								strings.Contains(lowerID, "embed") ||
								strings.Contains(lowerID, "bge") ||
								strings.Contains(lowerID, "gte")
						case media.KindWebSearch:
							// Web search is a tool capability of Gemini-family chat
							// engines: any active, non-disabled chat model can drive
							// it, so match the live catalog instead of a hardcoded
							// engine list (which goes stale as upstream retires tiers).
							match = e.Provider == "antigravity" &&
								strings.HasPrefix(lowerID, "gemini-") &&
								!strings.Contains(lowerID, "-image") &&
								slices.Contains(cm.Capabilities, "chat")
						}
						if match && !seenModel[cm.ID] {
							seenModel[cm.ID] = true
							displayName := cm.DisplayName
							if displayName == "" {
								displayName = cm.ID
							}
							matchedModels = append(matchedModels, media.ModelEntry{
								ID:   cm.ID,
								Name: displayName,
								Kind: filterKind,
							})
						}
					}
				}
			}
			if connectedOnly && len(matchedModels) == 0 {
				continue
			}
			out = append(out, EnrichedProvider{
				Provider:            e.Provider,
				Name:                e.Name,
				Kinds:               e.Kinds,
				Models:              matchedModels,
				Configs:             e.Configs,
				ActiveConnections:   count,
				HasConnection:       has,
				PrimaryConnectionID: primaryConnByProvider[e.Provider],
				AuthType:            authTypeByProvider[e.Provider],
			})
		}
		// 排序：已配置连接的优先置顶，其次按 Provider ID 稳定排序
		sort.Slice(out, func(i, j int) bool {
			if out[i].HasConnection != out[j].HasConnection {
				return out[i].HasConnection
			}
			return out[i].Provider < out[j].Provider
		})
		return out
	}
	if kind != "" {
		targetKind := media.Kind(kind)
		providers := enrichList(media.GetProvidersByKind(targetKind), targetKind)
		writeJSON(w, http.StatusOK, map[string]any{"providers": providers, "kind": kind, "count": len(providers)})
		return
	}

	// Return all grouped by kind
	kinds := []media.Kind{media.KindEmbedding, media.KindImage, media.KindTTS, media.KindSTT, media.KindVideo, media.KindWebFetch, media.KindWebSearch}
	grouped := make(map[string]any)
	for _, k := range kinds {
		grouped[string(k)] = enrichList(media.GetProvidersByKind(k), k)
	}
	writeJSON(w, http.StatusOK, map[string]any{"kinds": grouped})
}

// handleMediaVoices handles GET /api/media-providers/tts/voices
func (s *Server) handleMediaVoices(w http.ResponseWriter, r *http.Request) {
	providerID := r.URL.Query().Get("provider")
	if providerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing provider query param"})
		return
	}

	// Return static voice lists for known providers
	voices := getStaticVoices(providerID)
	writeJSON(w, http.StatusOK, map[string]any{"provider": providerID, "voices": voices})
}

// recordMediaUsage persists a media call into usageHistory (charts/cost) and
// requestDetails (per-request audit), replacing the old detail-only writer so
// media traffic is visible in the usage dashboard like chat traffic.
func (s *Server) recordMediaUsage(r *http.Request, providerID, modelName, connID, endpoint string, status, latencyMs int) {
	if s.DB == nil {
		return
	}
	if modelName == "" {
		modelName = providerID
	}
	statusStr := "ok"
	if status == 499 {
		statusStr = "canceled"
	} else if status >= 400 {
		statusStr = fmt.Sprintf("%d", status)
	}
	entry := &db.UsageEntry{
		Provider:     providerID,
		Model:        modelName,
		ConnectionID: connID,
		APIKey:       extractRequestAPIKey(r),
		Endpoint:     endpoint,
		Status:       statusStr,
		Meta:         `{"usage_missing":true,"media":true}`,
	}
	if err := s.DB.SaveUsageEntry(entry); err != nil {
		slog.Warn("Failed to record media usage", "error", err, "model", modelName)
	}
	rdID := fmt.Sprintf("%d-%s", time.Now().UnixNano(), modelName)
	rdData := map[string]any{
		"id":           rdID,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"model":        modelName,
		"provider":     providerID,
		"connectionId": connID,
		"status":       statusStr,
		"latencyMs":    latencyMs,
		"endpoint":     endpoint,
	}
	rdBytes, _ := json.Marshal(rdData)
	_ = s.DB.SaveRequestDetail(&db.RequestDetail{
		ID:           rdID,
		Timestamp:    rdData["timestamp"].(string),
		Provider:     providerID,
		Model:        modelName,
		ConnectionID: connID,
		Status:       statusStr,
		Data:         string(rdBytes),
	})
}

// --- Video job registry ---
// A video job ID is bound to the account that created it: status polling must
// reach the same account (and its refreshed token). The registry maps job IDs
// to provider+connection so /v1/videos/{id} needs no provider hint.

const kvScopeVideoJobs = "videoJobs"

type videoJobRef struct {
	Provider   string `json:"provider"`
	Connection string `json:"connectionId"`
	CreatedAt  int64  `json:"createdAt"`
}

func (s *Server) registerVideoJob(jobID, providerID, connID string) {
	if s.DB == nil || jobID == "" {
		return
	}
	b, _ := json.Marshal(videoJobRef{Provider: providerID, Connection: connID, CreatedAt: time.Now().Unix()})
	_ = s.DB.KVSet(kvScopeVideoJobs, jobID, string(b))
}

// lookupVideoJob resolves a job ID to its owning provider+connection.
func (s *Server) lookupVideoJob(jobID string) (videoJobRef, bool) {
	var ref videoJobRef
	if s.DB == nil {
		return ref, false
	}
	raw, err := s.DB.KVGet(kvScopeVideoJobs, jobID)
	if err != nil || json.Unmarshal([]byte(raw), &ref) != nil {
		return ref, false
	}
	return ref, true
}

// proxyMediaResponse copies an upstream response to the client.
func (s *Server) proxyMediaResponse(w http.ResponseWriter, resp *http.Response) {
	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func getStaticVoices(providerID string) []map[string]string {
	switch providerID {
	case "openai":
		return []map[string]string{
			{"id": "alloy", "name": "Alloy"},
			{"id": "echo", "name": "Echo"},
			{"id": "fable", "name": "Fable"},
			{"id": "onyx", "name": "Onyx"},
			{"id": "nova", "name": "Nova"},
			{"id": "shimmer", "name": "Shimmer"},
			{"id": "coral", "name": "Coral"},
			{"id": "verse", "name": "Verse"},
			{"id": "ballad", "name": "Ballad"},
			{"id": "ash", "name": "Ash"},
			{"id": "sage", "name": "Sage"},
			{"id": "amuch", "name": "Amuch"},
			{"id": "dan", "name": "Dan"},
		}
	case "elevenlabs":
		return []map[string]string{
			{"id": "21m00Tcm4TlvDq8ikWAM", "name": "Rachel"},
			{"id": "AZnzlk1XvdvUeBnXmlld", "name": "Domi"},
			{"id": "EXAVITQu4vr4xnSDxMaL", "name": "Bella"},
			{"id": "ErXwobaYiN019PkySvjV", "name": "Antoni"},
			{"id": "MF3mGyEYCl7XYWbV9V6O", "name": "Elli"},
			{"id": "TxGEqnHWrfWFTfGW9XjX", "name": "Josh"},
			{"id": "VR6AewLTigWG4xSOukaG", "name": "Arnold"},
			{"id": "pNInz6obpgDQGcFmaJgB", "name": "Adam"},
			{"id": "yoZ06aMxZJJ28mfd3POQ", "name": "Sam"},
		}
	default:
		return nil
	}
}

// registerMediaRoutes adds media API routes to the server.
func (s *Server) registerMediaRoutes() {
	// OpenAI-compatible media endpoints
	s.Router.HandleFunc("POST /v1/images/generations", s.handleImageGeneration)
	s.Router.HandleFunc("POST /v1/audio/speech", s.handleAudioSpeech)
	s.Router.HandleFunc("POST /v1/audio/transcriptions", s.handleAudioTranscriptions)
	s.Router.HandleFunc("POST /v1/videos/generations", s.handleVideoGenerations)
	s.Router.HandleFunc("GET /v1/videos/{id}", s.handleVideoStatus)
	s.Router.HandleFunc("POST /v1/web/fetch", s.handleWebFetch)
	s.Router.HandleFunc("POST /v1/search", s.handleWebSearch)

	// Dashboard media management API
	s.Router.HandleFunc("GET /api/media-providers", s.handleMediaProviders)
	s.Router.HandleFunc("GET /api/media-providers/tts/voices", s.handleMediaVoices)
}

// mediaProviderID extracts provider from model string (e.g. "openai/dall-e-3" -> "openai")
func mediaProviderID(modelStr string) string {
	if idx := strings.Index(modelStr, "/"); idx > 0 {
		return modelStr[:idx]
	}
	return ""
}

// aggregateAntigravityImageResponse parses Google streamGenerateContent SSE stream and packages inlineData into OpenAI Image Response
func (s *Server) aggregateAntigravityImageResponse(w http.ResponseWriter, resp *http.Response) {
	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(resp.Body)
		var errData struct {
			Error struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(respBytes, &errData) == nil && errData.Error.Message != "" {
			writeJSON(w, resp.StatusCode, map[string]any{"error": map[string]any{"message": errData.Error.Message, "type": "upstream_error", "code": resp.StatusCode}})
			return
		}
		writeJSON(w, resp.StatusCode, map[string]any{"error": map[string]any{"message": string(respBytes), "type": "upstream_error", "code": resp.StatusCode}})
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 20*1024*1024)
	var b64Images []string
	for scanner.Scan() {
		data, isDone, ok := translator.ParseSSEDataLineString(scanner.Text())
		if !ok || isDone {
			continue
		}

		type imgCandidate struct {
			Content struct {
				Parts []struct {
					InlineData *struct {
						MimeType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		}
		var payload struct {
			Response struct {
				Candidates []imgCandidate `json:"candidates"`
			} `json:"response"`
			Candidates []imgCandidate `json:"candidates"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err == nil {
			candidates := payload.Response.Candidates
			if len(candidates) == 0 {
				candidates = payload.Candidates
			}
			for _, c := range candidates {
				for _, p := range c.Content.Parts {
					if p.InlineData != nil && p.InlineData.Data != "" {
						b64Images = append(b64Images, p.InlineData.Data)
					}
				}
			}
		}
	}

	if len(b64Images) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"created": time.Now().Unix(),
			"data":    []any{},
		})
		return
	}

	var dataItems []map[string]any
	for _, b64 := range b64Images {
		dataItems = append(dataItems, map[string]any{
			"b64_json": b64,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"created": time.Now().Unix(),
		"data":    dataItems,
	})
}

// aggregateAntigravitySearchResponse unpacks Google GroundingMetadata into unified search results
func (s *Server) aggregateAntigravitySearchResponse(w http.ResponseWriter, resp *http.Response, query string) {
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to read antigravity search response"})
		return
	}

	if resp.StatusCode >= 400 {
		var errData struct {
			Error struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(respBytes, &errData) == nil && errData.Error.Message != "" {
			writeJSON(w, resp.StatusCode, map[string]string{"error": errData.Error.Message})
			return
		}
		writeJSON(w, resp.StatusCode, map[string]string{"error": string(respBytes)})
		return
	}

	type antigravityCandidate struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		GroundingMetadata struct {
			WebSearchQueries []string `json:"webSearchQueries"`
			GroundingChunks  []struct {
				Web struct {
					URI   string `json:"uri"`
					Title string `json:"title"`
				} `json:"web"`
			} `json:"groundingChunks"`
		} `json:"groundingMetadata"`
	}

	var envelope struct {
		Response struct {
			Candidates []antigravityCandidate `json:"candidates"`
		} `json:"response"`
		Candidates []antigravityCandidate `json:"candidates"`
	}

	results := []map[string]any{}
	summary := ""
	if err := json.Unmarshal(respBytes, &envelope); err == nil {
		candidates := envelope.Response.Candidates
		if len(candidates) == 0 {
			candidates = envelope.Candidates
		}
		if len(candidates) > 0 {
			cand := candidates[0]
			for _, p := range cand.Content.Parts {
				summary += p.Text
			}
			if query == "" && len(cand.GroundingMetadata.WebSearchQueries) > 0 {
				query = cand.GroundingMetadata.WebSearchQueries[0]
			}
			for i, chunk := range cand.GroundingMetadata.GroundingChunks {
				if chunk.Web.URI != "" {
					results = append(results, map[string]any{
						"position": i + 1,
						"title":    chunk.Web.Title,
						"url":      chunk.Web.URI,
						"snippet":  summary,
					})
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"provider": "antigravity",
		"query":    query,
		"count":    len(results),
		"summary":  summary,
		"results":  results,
	})
}
