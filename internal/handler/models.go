package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	conns, err := s.DB.ListConnections()
	if err != nil {
		slog.Error("Failed to list connections", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	type ModelEntry struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
		// Name is the spelling OpenAI-compatible discoverers read; DisplayName is
		// what Anthropic's own /v1/models uses. One value, both spellings, so
		// neither client family falls back to the raw "provider/model" id.
		Name          string   `json:"name,omitempty"`
		DisplayName   string   `json:"display_name,omitempty"`
		Capabilities  []string `json:"capabilities,omitempty"`
		Modalities    []string `json:"modalities,omitempty"`
		ContextLength int      `json:"context_length,omitempty"`
		MaxOutput     int      `json:"max_output_tokens,omitempty"`
	}

	var models []ModelEntry

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
		models = append(models, ModelEntry{
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
			models = append(models, ModelEntry{
				ID:            fullID,
				Object:        "model",
				OwnedBy:       providerID,
				Name:          displayName,
				DisplayName:   displayName,
				ContextLength: meta.ContextLength,
				MaxOutput:     meta.MaxOutput,
				Capabilities:  meta.Capabilities,
				Modalities:    meta.Modalities,
			})
		}
	}

	// Filter query parameter: ?type=image or ?kind=image
	targetKind := strings.ToLower(r.URL.Query().Get("type"))
	if targetKind == "" {
		targetKind = strings.ToLower(r.URL.Query().Get("kind"))
	}

	// Deterministic sorting of providers by registry priority, then by provider ID
	sortedProviders := make([]string, 0, len(activeConnsByProvider))
	for pid := range activeConnsByProvider {
		sortedProviders = append(sortedProviders, pid)
	}
	sort.Slice(sortedProviders, func(i, j int) bool {
		p1, p2 := sortedProviders[i], sortedProviders[j]
		info1, ok1 := provider.GetProvider(p1)
		info2, ok2 := provider.GetProvider(p2)
		prio1 := 1000
		if ok1 && info1.Priority > 0 {
			prio1 = info1.Priority
		}
		prio2 := 1000
		if ok2 && info2.Priority > 0 {
			prio2 = info2.Priority
		}
		if prio1 != prio2 {
			return prio1 < prio2
		}
		return p1 < p2
	})

	// Add models for all providers in stable priority order
	for _, providerID := range sortedProviders {
		pConns := activeConnsByProvider[providerID]
		appendProviderModels(providerID, pConns)
	}

	// Capability filtering: if targetKind == "image", only return image-generation models.
	// Otherwise (default chat completions), filter out dedicated image generation models.
	var filtered []ModelEntry
	for _, m := range models {
		isImageGen := slices.Contains(m.Capabilities, "image-generation") ||
			strings.Contains(strings.ToLower(m.ID), "-image") ||
			strings.Contains(strings.ToLower(m.ID), "dall-e") ||
			strings.Contains(strings.ToLower(m.ID), "flux")
		if targetKind == "image" {
			if isImageGen {
				filtered = append(filtered, m)
			}
		} else {
			if !isImageGen {
				filtered = append(filtered, m)
			}
		}
	}
	models = filtered

	keyObj := auth.APIKeyFromContext(r.Context())
	if keyObj != nil && len(keyObj.AllowedModels) > 0 {
		var allowed []ModelEntry
		for _, m := range models {
			if keyObj.IsModelAllowed(m.ID) {
				allowed = append(allowed, m)
			}
		}
		models = allowed
	}

	// Only exposed models for providers that actually have configured active connections
	if models == nil {
		models = []ModelEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   models,
	})
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
	models, fetchErr := s.fetchProviderModels(r.Context(), &conn, client, true)
	if fetchErr != nil {
		slog.Warn("Model refresh failed", slog.String("provider", providerID), "error", fetchErr)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fetchErr.Error()})
		return
	}
	if err := s.cacheProviderModels(providerID, models, client); err != nil {
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
	if psd != nil {
		userID, _ = psd["userId"].(string)
		machineID, _ = psd["machineId"].(string)
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
		ProviderID: conn.Provider,
		UserID:     resolved.UserID,
		AuthToken:  resolved.AccessToken,
		Name:       conn.Name,
		Email:      conn.Email,
		MachineID:  machineID,
	}
	models := provider.QoderCatalogModels(creds, client, force)
	if models == nil && !force {
		// Force one refresh — the cache may not be populated yet.
		models = provider.QoderCatalogModels(creds, client, true)
	}
	return models
}

func (s *Server) loadModelsDevCatalog(client *http.Client) map[string]model.ModelsDevEntry {
	// 1. Prime in-memory cache from DB if memory is currently empty
	if len(model.GetModelsDevCatalog()) == 0 {
		if raw, err := s.DB.KVGet("modelsDevCatalog", "cache"); err == nil && raw != "" {
			var dbCat map[string]model.ModelsDevEntry
			if err := json.Unmarshal([]byte(raw), &dbCat); err == nil && len(dbCat) > 0 {
				model.SetModelsDevCatalog(dbCat)
			}
		}
	}

	// 2. Fetch fresh from network (LoadModelsDevCatalog respects 24h TTL internally)
	catalog, err := model.LoadModelsDevCatalog(client)
	if err == nil && len(catalog) > 0 {
		// Persist to DB for offline/cold-start resilience
		if data, err := json.Marshal(catalog); err == nil {
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
			models, fetchErr := s.fetchProviderModels(context.Background(), conn, client, false)
			if fetchErr != nil {
				slog.Warn("Auto-sync found no usable catalog from this connection",
					slog.String("provider", providerID),
					slog.String("connection", conn.ID), "error", fetchErr)
				continue
			}
			if len(models) == 0 {
				continue
			}
			if err := s.cacheProviderModels(providerID, models, client); err != nil {
				slog.Warn("Failed to cache auto-synced models", slog.String("provider", providerID), "error", err)
			}
			break
		}
	}
}

// fetchProviderModels is the single upstream model-catalog read for a
// connection's provider. qoder and antigravity have no standard /models
// endpoint — qoder's catalog is COSY-signed per user, antigravity's is
// project-scoped — so they branch ahead of the registry-driven generic fetch.
// A provider whose live fetch fails keeps its static registry seed, when it has
// one; otherwise the error surfaces so callers can say why.
func (s *Server) fetchProviderModels(ctx context.Context, conn *model.ProviderConnection, client *http.Client, force bool) ([]model.ModelMetadata, error) {
	pInfo, ok := provider.GetProvider(conn.Provider)
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", conn.Provider)
	}
	s.tryRefreshToken(conn)

	switch {
	case provider.IsQoder(conn.Provider):
		if models := s.fetchQoderCatalog(conn, client, force); models != nil {
			return models, nil
		}
		return nil, fmt.Errorf("qoder model catalog fetch failed")

	case conn.Provider == "antigravity":
		projectID, discErr := s.EnsureAntigravityProject(ctx, conn, client)
		models := s.fetchAntigravityCatalog(ctx, client, conn.Data.AccessToken, projectID)
		if models != nil {
			return models, nil
		}
		if discErr != nil {
			return nil, fmt.Errorf("antigravity model catalog fetch failed (project discovery failed: %v)", discErr)
		}
		return nil, fmt.Errorf("antigravity model catalog fetch failed")

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
			return fetched, nil
		}
		if static := populateStaticModels(pInfo); len(static) > 0 {
			slog.Info("Live model fetch failed or empty, falling back to static registry models", slog.String("provider", conn.Provider), "error", err)
			return static, nil
		}
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("provider returned no models")
	}
}

// cacheProviderModels applies the single models.dev enrichment pass and stores
// the catalog under the provider's cache key. The cache is provider-scoped, not
// per-connection: entitlement and credit multipliers follow the provider's
// published catalog, so any healthy account is authoritative for all of them.
func (s *Server) cacheProviderModels(providerID string, models []model.ModelMetadata, client *http.Client) error {
	s.loadModelsDevCatalog(client)
	model.EnrichModelsFromCatalog(models)
	data, err := json.Marshal(model.CachedModels{
		FetchedAt: time.Now().UTC(),
		Models:    models,
	})
	if err != nil {
		return err
	}
	if err := s.DB.KVSet(model.KVScopeProviderModelCache, providerID, string(data)); err != nil {
		return err
	}
	slog.Info("Live models synced for provider", slog.String("provider", providerID), slog.Int("count", len(models)))
	return nil
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

func (s *Server) syncConnectionModels(conn *model.ProviderConnection) {
	if conn == nil {
		return
	}
	client := s.getHTTPClient(30 * time.Second)
	// Event-driven (login / credential edit / stale cache read): fetch authoritatively.
	models, err := s.fetchProviderModels(context.Background(), conn, client, true)
	if err != nil {
		slog.Warn("Model sync failed", slog.String("provider", conn.Provider), "error", err)
		return
	}
	if len(models) == 0 {
		return
	}
	if err := s.cacheProviderModels(conn.Provider, models, client); err != nil {
		slog.Warn("Failed to cache synced models", slog.String("provider", conn.Provider), "error", err)
	}
}
