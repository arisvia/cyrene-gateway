package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// clientModelEntry is one published model on the client-facing /v1 surface.
// Naming follows the two ecosystems this gateway serves at once: `name` is the
// spelling OpenAI-compatible discoverers read, `display_name` is Anthropic's own
// field. Both carry the same value so neither family falls back to the raw
// "provider/model" id as its label.
type clientModelEntry struct {
	ID            string   `json:"id"`
	Object        string   `json:"object"`
	OwnedBy       string   `json:"owned_by"`
	Name          string   `json:"name,omitempty"`
	DisplayName   string   `json:"display_name,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	Modalities    []string `json:"modalities,omitempty"`
	ContextLength int      `json:"context_length,omitempty"`
	MaxOutput     int      `json:"max_output_tokens,omitempty"`
	// Created is the model's public release stamp in Unix seconds, taken from the
	// models.dev catalog. Omitted when the catalog has no date for that model.
	Created int64 `json:"created,omitempty"`
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	models, err := s.buildClientModels()
	if err != nil {
		slog.Error("Failed to list connections", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Filter query parameter: ?type=image or ?kind=image
	targetKind := strings.ToLower(r.URL.Query().Get("type"))
	if targetKind == "" {
		targetKind = strings.ToLower(r.URL.Query().Get("kind"))
	}
	models = filterModelsByKind(models, targetKind)

	allowed := make([]clientModelEntry, 0, len(models))
	for _, m := range models {
		if modelAllowedForRequest(r, m.ID) {
			allowed = append(allowed, m)
		}
	}
	models = allowed

	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   models,
	})
}

// handleModelRetrieve serves GET /v1/models/{id} — the retrieve call both the
// OpenAI and Anthropic SDKs issue after listing. It reuses the very same builder
// as the list endpoint, so retrieving a model the client just saw returns
// byte-identical fields instead of a second, drifting definition.
func (s *Server) handleModelRetrieve(w http.ResponseWriter, r *http.Request) {
	wanted := r.PathValue("id")
	models, err := s.buildClientModels()
	if err != nil {
		slog.Error("Failed to list connections for model retrieve", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	entry, found := lookupClientModel(models, wanted, func(m clientModelEntry) bool {
		return modelAllowedForRequest(r, m.ID)
	})
	// A key scoped to specific models must not learn about the ones it cannot
	// call, so a scoped-out model reports the same 404 as an unknown one.
	if found && !modelAllowedForRequest(r, entry.ID) {
		found = false
	}
	if !found {
		writeModelNotFound(w, wanted)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

// lookupClientModel resolves a retrieve id against the published list: the exact
// "provider/model" id first, then a bare model id. Bare names take the first
// match in list order, which is registry-priority order — the same order
// provider.RegistryOrder gives the router, so retrieving a bare name describes the
// connection that will actually answer the next completion.
//
// `allowed` (nil for everything) only filters the bare pass. A qualified id is
// answered verbatim because its owner is explicit: skipping to another provider's
// same-named model would answer a question nobody asked. In the bare pass,
// however, stopping at the first candidate made a scoped key 404 on a model its
// own list showed — the deny must move on to the next sibling, not end the walk.
func lookupClientModel(models []clientModelEntry, wanted string, allowed func(clientModelEntry) bool) (clientModelEntry, bool) {
	if wanted == "" {
		return clientModelEntry{}, false
	}
	// Match the way the router matches: the reasoning-effort suffix and the
	// context-window marker never name a different model, and ids compare
	// case-insensitively. Without this, models.retrieve("GPT-4O") or
	// "openai/gpt-4o(high)" 404s on an entry that /v1/models advertises and that a
	// completion would serve from the very same connection.
	wanted = stripModelSpelling(wanted)
	for _, m := range models {
		if strings.EqualFold(m.ID, wanted) {
			return m, true
		}
	}
	for _, m := range models {
		_, bare, ok := strings.Cut(m.ID, "/")
		if !ok || !strings.EqualFold(bare, wanted) {
			continue
		}
		if allowed != nil && !allowed(m) {
			continue
		}
		return m, true
	}
	return clientModelEntry{}, false
}

// stripModelSpelling removes the decorations a client may hang on a model string
// that do not identify a different model. It is the same reduction the routing
// path applies (provider.ExtractModelReasoningEffort), so listing, retrieval and
// dispatch agree on which model a string names.
func stripModelSpelling(modelStr string) string {
	clean, _ := provider.ExtractModelReasoningEffort(modelStr)
	return clean
}

func writeModelNotFound(w http.ResponseWriter, id string) {
	writeJSON(w, http.StatusNotFound, map[string]any{
		"error": map[string]string{
			"message": fmt.Sprintf("The model '%s' does not exist", id),
			"type":    "invalid_request_error",
			"code":    "model_not_found",
		},
	})
}

// geminiModel is one entry of Google's model listing schema. It is a different
// wire shape from clientModelEntry — resource names, camelCase limits and a
// method list — so it is derived from the same published entry instead of the
// /v1 handlers growing a second set of conditionals.
type geminiModel struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName,omitempty"`
	InputTokenLimit            int      `json:"inputTokenLimit,omitempty"`
	OutputTokenLimit           int      `json:"outputTokenLimit,omitempty"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

// geminiMethodsFor maps our capability tags onto the operations Google
// advertises. Image generators have no token counting in the Gemini API, and
// embedding models answer only the embed RPCs.
func geminiMethodsFor(capabilities []string) []string {
	switch {
	case slices.Contains(capabilities, "embeddings"):
		return []string{"embedContent", "batchEmbedContents"}
	case slices.Contains(capabilities, "image-generation"):
		return []string{"generateContent", "streamGenerateContent"}
	default:
		return []string{"generateContent", "streamGenerateContent", "countTokens"}
	}
}

func toGeminiModel(m clientModelEntry) geminiModel {
	// The resource name carries the qualified id, because that is what our
	// generation routes accept: handleGeminiGenerateContent strips the leading
	// "models/" and then resolves "provider/model". A client that lists here and
	// generates from the returned name therefore reaches the same connection.
	return geminiModel{
		Name:                       "models/" + m.ID,
		DisplayName:                m.DisplayName,
		InputTokenLimit:            m.ContextLength,
		OutputTokenLimit:           m.MaxOutput,
		SupportedGenerationMethods: geminiMethodsFor(m.Capabilities),
	}
}

func (s *Server) handleGeminiModels(w http.ResponseWriter, r *http.Request) {
	models, err := s.buildClientModels()
	if err != nil {
		slog.Error("Failed to list connections for gemini models", "error", err)
		writeGeminiError(w, http.StatusInternalServerError, "Internal error.", "INTERNAL")
		return
	}
	// Google's own listing includes image models, so no kind filtering here.
	out := make([]geminiModel, 0, len(models))
	for _, m := range models {
		if !modelAllowedForRequest(r, m.ID) {
			continue
		}
		out = append(out, toGeminiModel(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": out})
}

// handleGeminiModelRetrieve serves GET /v1beta/models/{model}. Clients pass back
// the resource name they listed ("models/<provider>/<id>"), which is also valid
// as a bare or qualified id.
func (s *Server) handleGeminiModelRetrieve(w http.ResponseWriter, r *http.Request) {
	wanted := strings.TrimPrefix(r.PathValue("model"), "models/")
	models, err := s.buildClientModels()
	if err != nil {
		slog.Error("Failed to list connections for gemini model retrieve", "error", err)
		writeGeminiError(w, http.StatusInternalServerError, "Internal error.", "INTERNAL")
		return
	}
	entry, found := lookupClientModel(models, wanted, func(m clientModelEntry) bool {
		return modelAllowedForRequest(r, m.ID)
	})
	if !found || !modelAllowedForRequest(r, entry.ID) {
		writeGeminiModelNotFound(w, wanted)
		return
	}
	writeJSON(w, http.StatusOK, toGeminiModel(entry))
}

// writeGeminiError emits Google's own error envelope. The Gemini SDKs read
// error.code / error.message / error.status; handing them OpenAI's
// {"error": "internal error"} string means a failure arrives unreadable — and on
// this surface it is reachable (buildClientModels fails when the connection
// query fails), so it is not a theoretical shape problem.
func writeGeminiError(w http.ResponseWriter, httpStatus int, message, status string) {
	writeJSON(w, httpStatus, map[string]any{
		"error": map[string]any{
			"code":    httpStatus,
			"message": message,
			"status":  status,
		},
	})
}

func writeGeminiModelNotFound(w http.ResponseWriter, name string) {
	writeGeminiError(w, http.StatusNotFound, fmt.Sprintf("Model %q not found.", name), "NOT_FOUND")
}

// keyAllowsModel is the one allowlist gate for every surface that takes a client's
// model string: chat, messages, count-tokens, embeddings, media, list and retrieve.
// It tests the spelling the client sent *and* the canonical spelling the router
// dispatches on, because the reasoning-effort suffix and context marker do not pick
// a different model — a key scoped to "openai/gpt-4o" must not be refused
// "openai/gpt-4o(high)" that it can see in /v1/models and successfully call.
func keyAllowsModel(key *model.APIKey, modelStr string) bool {
	if key == nil || len(key.AllowedModels) == 0 {
		return true
	}
	if key.IsModelAllowed(modelStr) {
		return true
	}
	if clean := stripModelSpelling(modelStr); clean != modelStr {
		return key.IsModelAllowed(clean)
	}
	return false
}

// modelAllowedForRequest reports whether the API key serving this request may see
// the model. Keys without an allowlist see everything.
func modelAllowedForRequest(r *http.Request, id string) bool {
	return keyAllowsModel(auth.APIKeyFromContext(r.Context()), id)
}

// buildClientModels assembles every model the gateway publishes to clients:
// combo names, then per-provider catalog + custom models in registry priority
// order, with metadata overrides applied and disabled models removed. Kind and
// API-key scoping filters are the callers' business — the retrieve endpoint must
// see image models too. A nil slice is never returned, so the list endpoint
// encodes "data": [] rather than null.
func (s *Server) buildClientModels() ([]clientModelEntry, error) {
	conns, err := s.DB.ListConnections()
	if err != nil {
		return nil, err
	}

	var models []clientModelEntry

	// Load cached model metadata for all providers
	cacheIndex := s.loadModelCacheIndex()
	overrides, _ := s.DB.KVList(model.KVScopeModelMetaOverrides)
	isModelDisabled := s.loadDisabledModels()

	// Add combo names as models (filtering out disabled ones)
	combos, _ := s.DB.ListCombos()
	for _, c := range combos {
		if isModelDisabled.has(c.Name, c.Name) {
			continue
		}
		displayName := c.Name
		if len(c.Models) > 0 {
			displayName = fmt.Sprintf("%s (组合: %s)", c.Name, strings.Join(c.Models, " → "))
		}
		models = append(models, clientModelEntry{
			ID:          c.Name,
			Object:      "model",
			OwnedBy:     "cyrene-gateway",
			Name:        displayName,
			DisplayName: displayName,
		})
	}
	// Group active connections by provider
	activeConnsByProvider := make(map[string][]model.ProviderConnection)
	for _, conn := range conns {
		if conn.IsActive {
			activeConnsByProvider[conn.Provider] = append(activeConnsByProvider[conn.Provider], conn)
		}
	}

	seenFullIDs := make(map[string]bool)

	// Helper to extract and append models for a provider
	appendProviderModels := func(providerID string, pConns []model.ProviderConnection) {
		var unifiedModels []provider.ModelRef
		seenModel := make(map[string]bool)

		// 1. Live cached models
		if cached, _ := s.cachedProviderModels(providerID); len(cached) > 0 {
			for _, m := range cached {
				seenModel[m.ID] = true
				name := m.DisplayName
				if name == "" {
					name = m.ID
				}
				unifiedModels = append(unifiedModels, provider.ModelRef{ID: m.ID, Name: name})
			}
		}

		// 2. Custom models from connections of this provider
		for _, conn := range pConns {
			for _, cm := range s.loadCustomModels(conn.ID) {
				if !seenModel[cm.ID] {
					seenModel[cm.ID] = true
					name := cm.Name
					if name == "" {
						name = cm.ID
					}
					unifiedModels = append(unifiedModels, provider.ModelRef{ID: cm.ID, Name: name})
				}
			}
		}

		// Sort models deterministically by ID within the provider
		sort.Slice(unifiedModels, func(i, j int) bool {
			return unifiedModels[i].ID < unifiedModels[j].ID
		})

		// Append to models list with metadata and disable-check
		for _, m := range unifiedModels {
			fullID := providerID + "/" + m.ID
			if seenFullIDs[fullID] || isModelDisabled.has(fullID, m.ID) {
				continue
			}
			seenFullIDs[fullID] = true
			meta := model.MergeMetadata(m.ID, nil, cacheIndex[fullID])
			displayName := meta.DisplayName
			if (displayName == "" || displayName == m.ID) && m.Name != "" && m.Name != m.ID {
				displayName = m.Name
			}
			if displayName == "" || displayName == m.ID {
				displayName = model.FormatFallbackDisplayName(m.ID)
			}
			if overrides != nil {
				ovRaw := overrides[fullID]
				if ovRaw == "" {
					ovRaw = overrides[m.ID]
				}
				if ovRaw != "" {
					var ov ModelMetaOverride
					if err := json.Unmarshal([]byte(ovRaw), &ov); err == nil {
						if ov.DisplayName != "" {
							displayName = ov.DisplayName
						}
						if ov.ContextLength > 0 {
							meta.ContextLength = ov.ContextLength
						}
						if ov.MaxOutput > 0 {
							meta.MaxOutput = ov.MaxOutput
						}
					}
				}
			}
			models = append(models, clientModelEntry{
				ID:            fullID,
				Object:        "model",
				OwnedBy:       providerID,
				Name:          displayName,
				DisplayName:   displayName,
				ContextLength: meta.ContextLength,
				MaxOutput:     meta.MaxOutput,
				Capabilities:  meta.Capabilities,
				Modalities:    meta.Modalities,
				Created:       meta.Created,
			})
		}
	}

	// Deterministic sorting of providers by registry priority, then by provider ID.
	// One comparator for both surfaces: routing resolves a bare name in this order
	// (provider.RegistryOrder), so the list must be built in it too or the two
	// disagree about which provider owns a shared id.
	sortedProviders := slices.SortedFunc(maps.Keys(activeConnsByProvider), provider.RegistryOrder)

	// Add models for all providers in stable priority order
	for _, providerID := range sortedProviders {
		pConns := activeConnsByProvider[providerID]
		appendProviderModels(providerID, pConns)
	}

	return models, nil
}

// filterModelsByKind applies the ?type=image contract: "image" keeps only
// dedicated image generators, anything else (the default chat surface) hides
// them. Non-image results are returned as a non-nil slice so the list endpoint
// encodes "data": [] rather than null.
func filterModelsByKind(models []clientModelEntry, targetKind string) []clientModelEntry {
	filtered := make([]clientModelEntry, 0, len(models))
	for _, m := range models {
		isImageGen := slices.Contains(m.Capabilities, "image-generation") ||
			strings.Contains(strings.ToLower(m.ID), "-image") ||
			strings.Contains(strings.ToLower(m.ID), "dall-e") ||
			strings.Contains(strings.ToLower(m.ID), "flux")
		if (targetKind == "image") == isImageGen {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// loadModelCacheIndex loads all cached provider model metadata into a lookup
// map keyed by "providerID/modelID".
func (s *Server) loadModelCacheIndex() map[string]*model.ModelMetadata {
	index := make(map[string]*model.ModelMetadata)
	caches, err := s.DB.KVList(model.KVScopeProviderModelCache)
	if err != nil {
		return index
	}
	for providerID, raw := range caches {
		var cached model.CachedModels
		if err := json.Unmarshal([]byte(raw), &cached); err != nil {
			continue
		}
		for i := range cached.Models {
			m := &cached.Models[i]
			index[providerID+"/"+m.ID] = m
		}
	}
	return index
}

func populateStaticModels(pInfo provider.ProviderInfo) []model.ModelMetadata {
	if len(pInfo.Models) == 0 {
		return nil
	}
	out := make([]model.ModelMetadata, 0, len(pInfo.Models))
	for _, m := range pInfo.Models {
		// Raw registry seed only; context/output/capabilities are filled by the
		// single cache-layer enrichment pass before the models are stored.
		out = append(out, model.ModelMetadata{ID: m.ID, DisplayName: m.Name})
	}
	return out
}

// handleRefreshModels triggers a live model fetch for a provider and caches the result.
func (s *Server) handleRefreshModels(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")

	var conn model.ProviderConnection
	var providerInfo provider.ProviderInfo
	var ok bool

	// 1. Check if targetID is an existing connection ID
	if existingConn, err := s.DB.GetConnection(targetID); err == nil && existingConn != nil {
		conn = *existingConn
		providerInfo, ok = provider.GetProvider(conn.Provider)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown provider for connection: " + conn.Provider})
			return
		}
	} else {
		// 2. targetID is a provider ID (e.g. "qoder", "openai", "custom-openai")
		providerID := targetID
		providerInfo, ok = provider.GetProvider(providerID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown provider"})
			return
		}
		// Find an active connection with credentials for this provider, or use NoAuth default for free providers
		conns, err := s.DB.ListConnectionsByProvider(providerID)
		if err == nil && len(conns) > 0 {
			// Pick first connection with credentials; fallback to first active connection
			found := false
			for _, c := range conns {
				if c.IsActive && (c.Data.APIKey != "" || c.Data.AccessToken != "") {
					conn = c
					found = true
					break
				}
			}
			if !found {
				conn = conns[0]
			}
		} else {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "no active connection for provider: " + providerID,
			})
			return
		}
	}

	providerID := providerInfo.ID
	client := s.getHTTPClient(30 * time.Second)
	// Operator-initiated: bypass provider-side catalog caches entirely.
	models, fromUpstream, fetchErr := s.fetchProviderModels(r.Context(), &conn, client, true)
	if fetchErr != nil {
		slog.Warn("Model refresh failed", slog.String("provider", providerID), "error", fetchErr)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fetchErr.Error()})
		return
	}
	if err := s.cacheProviderModels(providerID, models, client, fromUpstream); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to cache models"})
		return
	}

	slog.Info("Models refreshed", slog.String("provider", providerID), slog.Int("count", len(models)))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"provider": providerID,
		"count":    len(models),
		"models":   models,
	})
}

// fetchQoderCatalog resolves the connection credential (PAT → job token) and
// fetches Qoder's live model catalog via COSY signing. force bypasses the 1h
// in-memory catalog cache: an operator pressing refresh must get today's
// multipliers, while the background poll may reuse them.
func (s *Server) fetchQoderCatalog(conn *model.ProviderConnection, client *http.Client, force bool) []model.ModelMetadata {
	psd := conn.Data.ProviderSpecificData
	userID := ""
	machineID := ""
	organizationID := ""
	if psd != nil {
		userID, _ = psd["userId"].(string)
		machineID, _ = psd["machineId"].(string)
		organizationID, _ = psd["organizationId"].(string)
	}
	token := conn.Data.AccessToken
	if token == "" {
		token = conn.Data.APIKey
	}
	if token == "" {
		return nil
	}

	resolved, err := provider.ResolveQoderCredentialFor(conn.Provider, token, userID, client)
	if err != nil {
		slog.Warn("Qoder PAT exchange failed during model refresh", "error", err)
		return nil
	}
	if resolved.UserID == "" {
		resolved.UserID = userID
	}
	if resolved.UserID == "" {
		return nil
	}

	creds := provider.QoderCosyCreds{
		ProviderID:     conn.Provider,
		UserID:         resolved.UserID,
		AuthToken:      resolved.AccessToken,
		Name:           conn.Name,
		Email:          conn.Email,
		MachineID:      machineID,
		OrganizationID: organizationID,
	}
	models := provider.QoderCatalogModels(creds, client, force)
	if models == nil && !force {
		// Force one refresh — the cache may not be populated yet.
		models = provider.QoderCatalogModels(creds, client, true)
	}
	return models
}

// StartModelsDevPrefetch loads the models.dev catalog once at boot while memory
// is still empty. A fresh install has no copy in SQLite, and without a catalog the
// client surfaces publish models with no context window, output cap or release
// date until some provider's catalog sync happens to succeed. Deliberately async:
// startup must never wait on an outbound request.
//
// The condition is expiry, not emptiness: startup has already hydrated the stored
// copy, so "memory has entries" says nothing about their age. Skipping on emptiness
// alone left restarted installs serving a catalog they never refreshed again.
func (s *Server) StartModelsDevPrefetch() {
	if s.DB == nil {
		return
	}
	hydrateModelsDevCatalog(s.DB)
	if !model.ModelsDevCatalogExpired() {
		return
	}
	s.goBackground(func() {
		if cat := s.loadModelsDevCatalog(s.getHTTPClient(30 * time.Second)); len(cat) == 0 {
			slog.Warn("models.dev catalog prefetch at startup returned nothing")
		}
	})
}

// hydrateModelsDevCatalog restores the persisted catalog into the process cache,
// carrying its fetch stamp. Startup and the on-demand loader both prime from here
// so the stamp can never be rewritten to "now" on one path and not the other:
// doing that reset the 24h TTL on every boot, and a gateway that restarts daily
// then never fetched models.dev again.
func hydrateModelsDevCatalog(database *db.DB) {
	if database == nil || len(model.GetModelsDevCatalog()) > 0 {
		return
	}
	raw, err := database.KVGet("modelsDevCatalog", "cache")
	if err != nil || raw == "" {
		return
	}
	if entries, fetchedAt, decErr := model.DecodeModelsDevCache(raw); decErr == nil && len(entries) > 0 {
		model.SetModelsDevCatalogAt(entries, fetchedAt)
	}
}

func (s *Server) loadModelsDevCatalog(client *http.Client) map[string]model.ModelsDevEntry {
	// 1. Prime in-memory cache from DB if memory is currently empty.
	hydrateModelsDevCatalog(s.DB)

	// 2. Fetch fresh from network (LoadModelsDevCatalog respects 24h TTL internally
	// and downloads at most once even when several callers arrive together).
	catalog, fetchedAt, err := model.LoadModelsDevCatalog(client)
	if err == nil && len(catalog) > 0 {
		// Persist to DB for offline/cold-start resilience, pairing this exact
		// catalog with the stamp it was cached under.
		if data, err := model.EncodeModelsDevCache(catalog, fetchedAt); err == nil {
			s.DB.KVSet("modelsDevCatalog", "cache", string(data))
		}
		return catalog
	}

	// 3. Fallback to existing memory cache
	return model.GetModelsDevCatalog()
}

// StartBackgroundModelSync starts periodic synchronization of models for active connections.
func (s *Server) StartBackgroundModelSync(ctx context.Context, interval time.Duration) {
	go func() {
		// Run initial sync shortly after startup (respect context cancellation)
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
		s.syncAllActiveConnections()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.syncAllActiveConnections()
			}
		}
	}()
}

// syncAllActiveConnections refreshes the catalog of every provider that has an
// active connection. One provider is one catalog: the fetch walks its
// connections and stops at the first that answers, so an expired or
// credential-less account no longer freezes the cached multipliers for the
// whole provider.
func (s *Server) syncAllActiveConnections() {
	conns, err := s.DB.ListConnections()
	if err != nil {
		return
	}

	client := s.getHTTPClient(30 * time.Second)
	byProvider := make(map[string][]*model.ProviderConnection)
	var order []string
	for i := range conns {
		conn := &conns[i]
		if !conn.IsActive {
			continue
		}
		if _, ok := provider.GetProvider(conn.Provider); !ok {
			continue
		}
		if _, seen := byProvider[conn.Provider]; !seen {
			order = append(order, conn.Provider)
		}
		byProvider[conn.Provider] = append(byProvider[conn.Provider], conn)
	}

	for _, providerID := range order {
		for _, conn := range byProvider[providerID] {
			// Background poll: reuse short-lived provider catalog caches.
			models, fromUpstream, fetchErr := s.fetchProviderModels(context.Background(), conn, client, false)
			if fetchErr != nil {
				slog.Warn("Auto-sync found no usable catalog from this connection",
					slog.String("provider", providerID),
					slog.String("connection", conn.ID), "error", fetchErr)
				continue
			}
			if len(models) == 0 {
				continue
			}
			if err := s.cacheProviderModels(providerID, models, client, fromUpstream); err != nil {
				slog.Warn("Failed to cache auto-synced models", slog.String("provider", providerID), "error", err)
			}
			// Only a real upstream answer ends the attempt: a registry-seed fallback
			// written under the old stamp still counts as stale, so keep trying the
			// provider's other connections for an actual catalog.
			if fromUpstream {
				break
			}
		}
	}
}

// fetchProviderModels is the single upstream model-catalog read for a
// connection's provider. qoder and antigravity have no standard /models
// endpoint — qoder's catalog is COSY-signed per user, antigravity's is
// project-scoped — so they branch ahead of the registry-driven generic fetch.
// A provider whose live fetch fails keeps its static registry seed, when it has
// one; otherwise the error surfaces so callers can say why.
func (s *Server) fetchProviderModels(ctx context.Context, conn *model.ProviderConnection, client *http.Client, force bool) ([]model.ModelMetadata, bool, error) {
	pInfo, ok := provider.GetProvider(conn.Provider)
	if !ok {
		return nil, false, fmt.Errorf("unknown provider: %s", conn.Provider)
	}
	s.tryRefreshToken(conn)

	switch {
	case provider.IsQoder(conn.Provider):
		if models := s.fetchQoderCatalog(conn, client, force); models != nil {
			return models, true, nil
		}
		return nil, false, fmt.Errorf("qoder model catalog fetch failed")

	case conn.Provider == "antigravity":
		projectID, discErr := s.EnsureAntigravityProject(ctx, conn, client)
		models := s.fetchAntigravityCatalog(ctx, client, conn.Data.AccessToken, projectID)
		if models != nil {
			return models, true, nil
		}
		if discErr != nil {
			return nil, false, fmt.Errorf("antigravity model catalog fetch failed (project discovery failed: %v)", discErr)
		}
		return nil, false, fmt.Errorf("antigravity model catalog fetch failed")

	default:
		// Phase 36 T6: registry ModelsURL + format-derived auth scheme.
		cfg := provider.ModelsFetchFor(pInfo)
		baseURL := pInfo.BaseURL
		if conn.Data.BaseURL != "" {
			cfg.URL = "" // user base URL override → derive from it
			baseURL = conn.Data.BaseURL
		}
		fetched, err := model.FetchModels(client, conn.Provider, baseURL, conn.Data.APIKey, conn.Data.AccessToken, cfg)
		if err == nil && len(fetched) > 0 {
			return fetched, true, nil
		}
		if static := populateStaticModels(pInfo); len(static) > 0 {
			slog.Info("Live model fetch failed or empty, falling back to static registry models", slog.String("provider", conn.Provider), "error", err)
			// Reported as NOT from upstream: seeding a cache row and calling that a
			// successful sync made the 24h TTL renew itself every 6h, so a provider
			// with no working /models endpoint (perplexity, vertex) served registry
			// seeds forever and never looked stale enough to retry.
			return static, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		return nil, false, fmt.Errorf("provider returned no models")
	}
}

// cacheProviderModels applies the single models.dev enrichment pass and stores
// the catalog under the provider's cache key. The cache is provider-scoped, not
// per-connection: entitlement and credit multipliers follow the provider's
// published catalog, so any healthy account is authoritative for all of them.
//
// fromUpstream says whether the rows came from the provider's own endpoint. A
// registry-seed fallback keeps the previous fetch stamp, so staleness stays
// visible and the next sync attempt is not postponed by a write that fetched
// nothing.
func (s *Server) cacheProviderModels(providerID string, models []model.ModelMetadata, client *http.Client, fromUpstream bool) error {
	s.loadModelsDevCatalog(client)
	model.EnrichModelsFromCatalog(models)
	fetchedAt := time.Now().UTC()
	if !fromUpstream {
		fetchedAt = s.previousModelCacheStamp(providerID)
	}
	data, err := json.Marshal(model.CachedModels{
		FetchedAt: fetchedAt,
		Models:    models,
	})
	if err != nil {
		return err
	}
	if err := s.DB.KVSet(model.KVScopeProviderModelCache, providerID, string(data)); err != nil {
		return err
	}
	if fromUpstream {
		slog.Info("Live models synced for provider", slog.String("provider", providerID), slog.Int("count", len(models)))
	} else {
		slog.Info("Static registry models cached for provider", slog.String("provider", providerID), slog.Int("count", len(models)))
	}
	return nil
}

// previousModelCacheStamp returns the stored fetch time so a seed write cannot
// extend the TTL; an absent or unreadable row falls back to the epoch, which
// reads as expired and forces the next sync to try the network again.
func (s *Server) previousModelCacheStamp(providerID string) time.Time {
	if s.DB == nil {
		return time.Unix(0, 0).UTC()
	}
	raw, err := s.DB.KVGet(model.KVScopeProviderModelCache, providerID)
	if err != nil || raw == "" {
		return time.Unix(0, 0).UTC()
	}
	var cached model.CachedModels
	if err := json.Unmarshal([]byte(raw), &cached); err != nil {
		return time.Unix(0, 0).UTC()
	}
	return cached.FetchedAt
}

// disabledModelSet snapshots the disabledModels scope once per request. List
// endpoints gate hundreds of rows, so calling provider.IsModelDisabled per row
// would turn one KV read into one per model.
type disabledModelSet map[string]string

func (s *Server) loadDisabledModels() disabledModelSet {
	if s.DB == nil {
		return nil
	}
	m, _ := s.DB.KVList(model.KVScopeDisabledModels)
	return m
}

// has reports a model disabled by either spelling: the qualified
// "provider/model" id and the bare id, which is how the operator toggle writes
// it and how a client may type it.
func (d disabledModelSet) has(fullID, bareID string) bool {
	if d == nil {
		return false
	}
	return d[fullID] != "" || d[bareID] != ""
}

// cachedProviderModels reads a stored catalog back. Stale entries are returned
// as-is — billing lookups and the model panel must not blank out because an
// upstream is slow — and the bool reports staleness so a caller that holds a
// connection can kick off a refresh instead of serving it indefinitely.
func (s *Server) cachedProviderModels(providerID string) ([]model.ModelMetadata, bool) {
	if s.DB == nil {
		return nil, false
	}
	raw, err := s.DB.KVGet(model.KVScopeProviderModelCache, providerID)
	if err != nil || raw == "" {
		return nil, false
	}
	var cached model.CachedModels
	if err := json.Unmarshal([]byte(raw), &cached); err != nil || len(cached.Models) == 0 {
		return nil, false
	}
	return cached.Models, cached.IsExpired()
}

// cachedModelEntry finds one model in a provider's cached catalog, matching any
// of the supplied ids (upstream ids and the client-facing short name differ for
// several providers). Returns nil when the catalog has no such entry.
func (s *Server) cachedModelEntry(providerID string, ids ...string) *model.ModelMetadata {
	cached, _ := s.cachedProviderModels(providerID)
	for i := range cached {
		for _, id := range ids {
			if id != "" && cached[i].ID == id {
				return &cached[i]
			}
		}
	}
	return nil
}

// claimModelSync takes the right to kick a background catalog refresh for one
// connection, and releases it when the work returns. Without it a dashboard
// polling a stale panel hit the refresh branch on every request — 30-second
// upstream calls stacking up per connection while the upstream is down.
func (s *Server) claimModelSync(connID string, work func()) {
	if _, loaded := s.modelSyncInFlight.LoadOrStore(connID, struct{}{}); loaded {
		return
	}
	s.goBackground(func() {
		defer s.modelSyncInFlight.Delete(connID)
		work()
	})
}

func (s *Server) syncConnectionModels(conn *model.ProviderConnection) {
	if conn == nil {
		return
	}
	client := s.getHTTPClient(30 * time.Second)
	// Event-driven (login / credential edit / stale cache read): fetch authoritatively.
	models, fromUpstream, err := s.fetchProviderModels(context.Background(), conn, client, true)
	if err != nil {
		slog.Warn("Model sync failed", slog.String("provider", conn.Provider), "error", err)
		return
	}
	if len(models) == 0 {
		return
	}
	if err := s.cacheProviderModels(conn.Provider, models, client, fromUpstream); err != nil {
		slog.Warn("Failed to cache synced models", slog.String("provider", conn.Provider), "error", err)
	}
}
