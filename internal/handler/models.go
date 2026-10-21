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
		ID            string   `json:"id"`
		Object        string   `json:"object"`
		OwnedBy       string   `json:"owned_by"`
		DisplayName   string   `json:"display_name,omitempty"`
		Capabilities  []string `json:"capabilities,omitempty"`
		Modalities    []string `json:"modalities,omitempty"`
		ContextLength int      `json:"context_length,omitempty"`
		MaxOutput     int      `json:"max_output_tokens,omitempty"`
	}

	var models []ModelEntry

	// Load cached model metadata for all providers
	cacheIndex := s.loadModelCacheIndex()
	overrides, _ := s.DB.KVList("modelMetaOverrides")
	// Load disabled models mapping to gate out models marked not public / disabled
	disabledMap, _ := s.DB.KVList("disabledModels")
	isModelDisabled := func(fullID, bareID string) bool {
		if disabledMap == nil {
			return false
		}
		return disabledMap[fullID] != "" || disabledMap[bareID] != ""
	}

	// Add aliases as models (filtering out disabled ones)
	aliases, _ := s.DB.KVList("aliases")
	for alias := range aliases {
		if isModelDisabled(alias, alias) {
			continue
		}
		models = append(models, ModelEntry{
			ID:      alias,
			Object:  "model",
			OwnedBy: "cyrene-gateway",
		})
	}

	// Add combo names as models (filtering out disabled ones)
	combos, _ := s.DB.ListCombos()
	for _, c := range combos {
		if isModelDisabled(c.Name, c.Name) {
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
		if raw, err := s.DB.KVGet("providerModelCache", providerID); err == nil && raw != "" {
			var cached model.CachedModels
			if err := json.Unmarshal([]byte(raw), &cached); err == nil && len(cached.Models) > 0 {
				for _, m := range cached.Models {
					seenModel[m.ID] = true
					name := m.DisplayName
					if name == "" {
						name = m.ID
					}
					unifiedModels = append(unifiedModels, provider.ModelRef{ID: m.ID, Name: name})
				}
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
			if seenFullIDs[fullID] || isModelDisabled(fullID, m.ID) {
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
				OwnedBy:       providerID,
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
	caches, err := s.DB.KVList("providerModelCache")
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
		meta := model.ModelMetadata{ID: m.ID, DisplayName: m.Name}
		if cat := model.LookupCatalog(m.ID); cat != nil {
			if meta.DisplayName == "" || meta.DisplayName == m.ID {
				meta.DisplayName = cat.DisplayName
			}
			meta.ContextLength = cat.ContextLength
			meta.MaxOutput = cat.MaxOutput
			meta.Capabilities = cat.Capabilities
			meta.Modalities = cat.Modalities
			meta.Family = cat.Family
		}
		out = append(out, meta)
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
	baseURL := providerInfo.BaseURL
	if conn.Data.BaseURL != "" {
		baseURL = conn.Data.BaseURL
	}
	client := s.getHTTPClient(30 * time.Second)
	s.tryRefreshToken(&conn)
	// Qoder: COSY-signed catalog (resolve PATs first) — its inference
	// protocol has no standard /models endpoint.
	var models []model.ModelMetadata
	if strings.HasPrefix(providerID, "qoder") {
		models = s.fetchQoderCatalog(&conn, client)
		if models == nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "qoder model catalog fetch failed"})
			return
		}
	} else if providerID == "antigravity" {
		token := conn.Data.AccessToken
		projectID, discErr := s.EnsureAntigravityProject(r.Context(), &conn, client)
		models = s.fetchAntigravityCatalog(r.Context(), client, token, projectID)
		if models == nil {
			errMsg := "antigravity model catalog fetch failed"
			if discErr != nil {
				errMsg += fmt.Sprintf(" (project discovery failed: %v)", discErr)
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": errMsg})
			return
		}

	} else {
		// Phase 36 T6: registry ModelsURL + format-derived auth scheme.
		cfg := provider.ModelsFetchFor(providerInfo)
		if conn.Data.BaseURL != "" {
			cfg.URL = "" // user base URL override → derive from it
		}
		fetched, fetchErr := model.FetchModels(client, providerID, baseURL, conn.Data.APIKey, conn.Data.AccessToken, cfg)
		if fetchErr != nil || len(fetched) == 0 {
			if static := populateStaticModels(providerInfo); len(static) > 0 {
				slog.Info("Live model fetch failed or empty, falling back to static registry models", slog.String("provider", providerID), "error", fetchErr)
				models = static
			} else {
				errMsg := "provider returned no models"
				if fetchErr != nil {
					errMsg = fetchErr.Error()
				}
				slog.Warn("Model refresh failed", slog.String("provider", providerID), "error", errMsg)
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": errMsg})
				return
			}
		} else {
			models = fetched
		}

		// models.dev backfill for context/output metadata when the provider
		// API omits it (best-effort, never overwrites live values).
		if catalog := s.loadModelsDevCatalog(client); len(catalog) > 0 {
			model.BackfillFromModelsDev(models, catalog)
		}
	}

	// Store in KV cache
	cached := model.CachedModels{
		FetchedAt: time.Now().UTC(),
		Models:    models,
	}
	data, _ := json.Marshal(cached)
	if err := s.DB.KVSet("providerModelCache", providerID, string(data)); err != nil {
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
// fetches Qoder's live model catalog via COSY signing.
func (s *Server) fetchQoderCatalog(conn *model.ProviderConnection, client *http.Client) []model.ModelMetadata {
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
	models := provider.QoderCatalogModels(creds, client, false)
	if models == nil {
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

func (s *Server) syncAllActiveConnections() {
	conns, err := s.DB.ListConnections()
	if err != nil {
		return
	}

	client := s.getHTTPClient(30 * time.Second)
	seen := make(map[string]bool)
	type syncTarget struct {
		providerID  string
		baseURL     string
		apiKey      string
		accessToken string
		conn        *model.ProviderConnection
	}
	var targets []syncTarget

	for i := range conns {
		conn := &conns[i]
		if !conn.IsActive || seen[conn.Provider] {
			continue
		}
		seen[conn.Provider] = true
		pInfo, ok := provider.GetProvider(conn.Provider)
		if !ok {
			continue
		}
		baseURL := pInfo.BaseURL
		if conn.Data.BaseURL != "" {
			baseURL = conn.Data.BaseURL
		}
		targets = append(targets, syncTarget{
			providerID:  conn.Provider,
			baseURL:     baseURL,
			apiKey:      conn.Data.APIKey,
			accessToken: conn.Data.AccessToken,
			conn:        conn,
		})
	}

	for _, target := range targets {
		if target.conn != nil {
			s.tryRefreshToken(target.conn)
			target.accessToken = target.conn.Data.AccessToken
		}
		pInfo, _ := provider.GetProvider(target.providerID)
		var models []model.ModelMetadata
		if strings.HasPrefix(target.providerID, "qoder") && target.conn != nil {
			models = s.fetchQoderCatalog(target.conn, client)
		} else if target.providerID == "antigravity" && target.conn != nil {

			projectID, _ := s.EnsureAntigravityProject(context.Background(), target.conn, client)
			models = s.fetchAntigravityCatalog(context.Background(), client, target.conn.Data.AccessToken, projectID)
		} else {
			cfg := provider.ModelsFetchFor(pInfo)
			if target.conn != nil && target.conn.Data.BaseURL != "" {
				cfg.URL = ""
			}
			fetched, err := model.FetchModels(client, target.providerID, target.baseURL, target.apiKey, target.accessToken, cfg)
			if err == nil && len(fetched) > 0 {
				models = fetched
			} else if static := populateStaticModels(pInfo); len(static) > 0 {
				models = static
			}
		}
		if len(models) == 0 {
			continue
		}

		if catalog := s.loadModelsDevCatalog(client); len(catalog) > 0 {
			model.BackfillFromModelsDev(models, catalog)
		}
		model.EnrichModelsFromCatalog(models)

		cached := model.CachedModels{
			FetchedAt: time.Now().UTC(),
			Models:    models,
		}
		if data, err := json.Marshal(cached); err == nil {
			s.DB.KVSet("providerModelCache", target.providerID, string(data))
			slog.Info("Auto-synced live models for provider", slog.String("provider", target.providerID), slog.Int("count", len(models)))
		}
	}
}

func (s *Server) syncConnectionModels(conn *model.ProviderConnection) {
	if conn == nil {
		return
	}
	pInfo, ok := provider.GetProvider(conn.Provider)
	if !ok {
		return
	}
	baseURL := pInfo.BaseURL
	if conn.Data.BaseURL != "" {
		baseURL = conn.Data.BaseURL
	}
	client := s.getHTTPClient(30 * time.Second)
	s.tryRefreshToken(conn)
	var models []model.ModelMetadata
	if strings.HasPrefix(conn.Provider, "qoder") {
		models = s.fetchQoderCatalog(conn, client)
	} else if conn.Provider == "antigravity" {

		projectID, _ := s.EnsureAntigravityProject(context.Background(), conn, client)
		models = s.fetchAntigravityCatalog(context.Background(), client, conn.Data.AccessToken, projectID)
	} else {
		cfg := provider.ModelsFetchFor(pInfo)
		if conn.Data.BaseURL != "" {
			cfg.URL = ""
		}
		fetched, err := model.FetchModels(client, conn.Provider, baseURL, conn.Data.APIKey, conn.Data.AccessToken, cfg)
		if err == nil && len(fetched) > 0 {
			models = fetched
		} else if static := populateStaticModels(pInfo); len(static) > 0 {
			models = static
		}
	}
	if len(models) > 0 {
		if catalog := s.loadModelsDevCatalog(client); len(catalog) > 0 {
			model.BackfillFromModelsDev(models, catalog)
		}
		model.EnrichModelsFromCatalog(models)
		cached := model.CachedModels{
			FetchedAt: time.Now().UTC(),
			Models:    models,
		}
		if data, err := json.Marshal(cached); err == nil {
			s.DB.KVSet("providerModelCache", conn.Provider, string(data))
			slog.Info("Live models synced for connection", slog.String("provider", conn.Provider), slog.Int("count", len(models)))
		}
	}
}
