package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/cli"
	"github.com/arisvia/cyrene-gateway/internal/config"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/media"
	"github.com/arisvia/cyrene-gateway/internal/metrics"
	"github.com/arisvia/cyrene-gateway/internal/middleware"
	"github.com/arisvia/cyrene-gateway/internal/mitm"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
	"github.com/arisvia/cyrene-gateway/internal/tunnel"
)

type Server struct {
	DB          *db.DB
	Router      *http.ServeMux
	Handler     http.Handler // Router wrapped with middleware
	Combos      *provider.ComboManager
	Proxies     *provider.ProxyManager
	MediaClient *media.Client
	Dashboard   *DashboardHandler
	Auth        *AuthHandler
	Tunnel      *TunnelHandler
	CLI         *CLIHandler
	Endpoints   *EndpointHandler
	Events      *EventBroadcaster
	MITM        *MITMHandler
	Metrics     *metrics.M
	Config      *config.Config
	startTime   time.Time
}

func NewServer(database *db.DB, cfg *config.Config) *Server {
	mux := http.NewServeMux()

	// Initialize proxy manager from active pools
	var proxyMgr *provider.ProxyManager
	if pools, err := database.ListProxyPools(); err == nil {
		proxyMgr = provider.NewProxyManager(pools)
	} else {
		proxyMgr = provider.NewProxyManager(nil)
	}

	tunnelMgr := tunnel.NewManager(cfg.DataDir, cfg.Port)

	// MITM proxy: only enabled with explicit -mitm flag AND localhost bind (safety)
	mitmEnabled := cfg.MITM && (cfg.Host == "127.0.0.1" || cfg.Host == "localhost")
	if cfg.MITM && !mitmEnabled {
		slog.Warn("MITM requested but host is not localhost — refusing to enable (server mode safety)",
			slog.String("host", cfg.Host))
	}
	mitmSrv := mitm.NewServer(cfg.MITMPort, cfg.Port, cfg.DataDir)

	s := &Server{
		DB:          database,
		Router:      mux,
		Combos:      provider.NewComboManager(),
		Proxies:     proxyMgr,
		MediaClient: media.NewClient(),
		Dashboard:   NewDashboardHandler(cfg),
		Auth:        NewAuthHandler(database),
		Tunnel:      NewTunnelHandler(tunnelMgr),
		CLI:         NewCLIHandler(cli.NewManager()),
		Endpoints:   NewEndpointHandler(cfg, database, tunnelMgr),
		MITM:        NewMITMHandler(mitmSrv, mitmEnabled),
		Metrics:     metrics.New(Version()),
		Config:      cfg,
		startTime:   time.Now(),
	}
	s.registerRoutes()
	s.registerMediaRoutes()
	// Prometheus scrape endpoint (public; no session required)
	s.Router.Handle("GET /metrics", s.Metrics.Handler())

	s.Handler = middleware.Chain(mux,
		middleware.Recovery,
		middleware.Logging,
		middleware.RequestSizeLimiter(),
		middleware.CORS,
		middleware.APIKeyAuth(database),
		middleware.APIKeyRateLimit(func() int {
			if st, err := database.GetSettings(); err == nil {
				return st.APIKeyRPM
			}
			return 0
		}),
		middleware.DashboardAuth(database),
	)
	return s
}

func (s *Server) registerRoutes() {
	// Dashboard panel (root + SPA static assets/fallback; specific routes take precedence)
	s.Router.Handle("GET /{$}", s.Dashboard)
	s.Router.Handle("GET /{path...}", s.Dashboard)

	// Health & meta
	s.Router.HandleFunc("GET /api/health", s.handleHealth)
	s.Router.HandleFunc("GET /api/version", s.handleVersion)
	s.Router.HandleFunc("GET /api/registry", s.handleRegistry)

	// Auth endpoints
	s.Router.HandleFunc("POST /api/auth/login", s.Auth.HandleLogin)
	s.Router.HandleFunc("POST /api/auth/logout", s.Auth.HandleLogout)
	s.Router.HandleFunc("GET /api/auth/status", s.Auth.HandleStatus)
	s.Router.HandleFunc("POST /api/auth/password", s.Auth.HandleSetPassword)

	// OpenAI-compatible API surface
	s.Router.HandleFunc("GET /v1/models", s.handleModels)
	s.Router.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
	s.Router.HandleFunc("POST /v1/embeddings", s.handleEmbeddings)
	s.Router.HandleFunc("POST /v1/messages", s.handleMessages)

	// Dashboard management API
	s.Router.HandleFunc("GET /api/settings", s.handleGetSettings)
	s.Router.HandleFunc("PUT /api/settings", s.handlePutSettings)
	s.Router.HandleFunc("PATCH /api/settings", s.handlePatchSettings)
	s.Router.HandleFunc("GET /api/providers", s.handleListProviders)
	s.Router.HandleFunc("GET /api/providers/{id}", s.handleGetProvider)
	s.Router.HandleFunc("POST /api/providers", s.handleCreateProvider)
	s.Router.HandleFunc("PUT /api/providers/{id}", s.handleUpdateProvider)
	s.Router.HandleFunc("DELETE /api/providers/{id}", s.handleDeleteProvider)
	s.Router.HandleFunc("POST /api/providers/{id}/reset", s.handleResetProviderStatus)
	s.Router.HandleFunc("GET /api/provider-nodes", s.handleListNodes)
	s.Router.HandleFunc("POST /api/provider-nodes", s.handleCreateNode)
	s.Router.HandleFunc("PUT /api/provider-nodes/{id}", s.handleUpdateNode)
	s.Router.HandleFunc("DELETE /api/provider-nodes/{id}", s.handleDeleteNode)
	s.Router.HandleFunc("GET /api/combos", s.handleListCombos)
	s.Router.HandleFunc("POST /api/combos", s.handleCreateCombo)
	s.Router.HandleFunc("PUT /api/combos/{id}", s.handleUpdateCombo)
	s.Router.HandleFunc("DELETE /api/combos/{id}", s.handleDeleteCombo)
	s.Router.HandleFunc("GET /api/keys", s.handleListKeys)
	s.Router.HandleFunc("POST /api/keys", s.handleCreateKey)
	s.Router.HandleFunc("DELETE /api/keys/{id}", s.handleDeleteKey)
	s.Router.HandleFunc("GET /api/models/alias", s.handleListAliases)
	s.Router.HandleFunc("POST /api/models/alias", s.handleSetAlias)
	s.Router.HandleFunc("DELETE /api/models/alias", s.handleDeleteAlias)
	s.Router.HandleFunc("GET /api/models/disabled", s.handleListDisabledModels)
	s.Router.HandleFunc("POST /api/models/disabled", s.handleDisableModel)
	s.Router.HandleFunc("DELETE /api/models/disabled", s.handleEnableModel)
	s.Router.HandleFunc("POST /api/models/test", s.handleTestModel)
	s.Router.HandleFunc("GET /api/proxy-pools", s.handleListProxyPools)
	s.Router.HandleFunc("POST /api/proxy-pools", s.handleCreateProxyPool)
	s.Router.HandleFunc("GET /api/proxy-pools/{id}", s.handleGetProxyPool)
	s.Router.HandleFunc("PUT /api/proxy-pools/{id}", s.handleUpdateProxyPool)
	s.Router.HandleFunc("DELETE /api/proxy-pools/{id}", s.handleDeleteProxyPool)

	// Provider connection testing
	s.Router.HandleFunc("POST /api/providers/{id}/test", s.handleTestProvider)
	s.Router.HandleFunc("POST /api/providers/test-credentials", s.handleTestCredentials)
	s.Router.HandleFunc("POST /api/providers/test-batch", s.handleTestBatch)
	s.Router.HandleFunc("POST /api/providers/enable-free", s.handleEnableFreeProviders)
	s.Router.HandleFunc("POST /api/providers/{id}/refresh-models", s.handleRefreshModels)

	// Provider detail: models (registry + custom)
	s.Router.HandleFunc("GET /api/providers/{id}/models", s.handleGetProviderModels)
	s.Router.HandleFunc("POST /api/providers/{id}/models", s.handleAddProviderModel)
	s.Router.HandleFunc("DELETE /api/providers/{id}/models", s.handleDeleteProviderModel)
	s.Router.HandleFunc("POST /api/providers/{id}/models/meta", s.handleSaveProviderModelMeta)
	s.Router.HandleFunc("DELETE /api/providers/{id}/models/meta", s.handleResetProviderModelMeta)
	// OAuth authorization flow
	s.Router.HandleFunc("GET /api/oauth/{provider}/authorize", s.handleOAuthAuthorize)
	s.Router.HandleFunc("GET /api/oauth/{provider}/callback", s.handleOAuthCallback)
	s.Router.HandleFunc("POST /api/oauth/{provider}/exchange", s.handleOAuthExchange)
	s.Router.HandleFunc("POST /api/oauth/{provider}/device-code", s.handleOAuthDeviceCode)
	s.Router.HandleFunc("POST /api/oauth/{provider}/device-code/poll", s.handleOAuthDeviceCodePoll)
	s.Router.HandleFunc("POST /api/oauth/{provider}/import", s.handleOAuthImport)
	s.Router.HandleFunc("GET /api/oauth/{provider}/status", s.handleOAuthStatus)
	s.Router.HandleFunc("POST /api/oauth/{provider}/refresh", s.handleOAuthRefresh)

	// Usage & observability API
	s.Router.HandleFunc("GET /api/usage/stats", s.handleUsageStats)
	s.Router.HandleFunc("GET /api/usage/history", s.handleUsageHistory)
	s.Router.HandleFunc("GET /api/usage/chart", s.handleUsageChart)
	s.Router.HandleFunc("GET /api/usage/request-details", s.handleUsageRequestDetails)
	s.Router.HandleFunc("GET /api/usage/request-details/{id}", s.handleUsageRequestDetailByID)
	s.Router.HandleFunc("GET /api/usage/stream", s.handleUsageStream)
	s.Router.HandleFunc("GET /api/usage/logs", s.handleUsageLogs)
	s.Router.HandleFunc("GET /api/system/logs", s.handleSystemLogs)
	s.Router.HandleFunc("GET /api/system/logs/stream", s.handleSystemLogsStream)
	s.Router.HandleFunc("GET /api/usage/providers", s.handleUsageProviders)

	// Quota tracker
	s.Router.HandleFunc("GET /api/quota", s.handleQuota)
	// Per-connection real usage/quota from provider APIs (Phase 31)
	s.Router.HandleFunc("GET /api/usage/connection/{id}", s.handleConnectionUsage)

	// Tunnel management
	s.Router.HandleFunc("GET /api/tunnel/status", s.Tunnel.HandleStatus)
	s.Router.HandleFunc("POST /api/tunnel/tailscale-install", s.Tunnel.HandleInstall)
	s.Router.HandleFunc("POST /api/tunnel/tailscale-enable", s.Tunnel.HandleEnable)
	s.Router.HandleFunc("POST /api/tunnel/tailscale-disable", s.Tunnel.HandleDisable)

	// CLI tools integration
	s.Router.HandleFunc("GET /api/cli-tools", s.CLI.HandleList)
	s.Router.HandleFunc("GET /api/cli-tools/all-statuses", s.CLI.HandleAllStatuses)
	s.Router.HandleFunc("GET /api/cli-tools/{id}", s.CLI.HandleGet)
	s.Router.HandleFunc("POST /api/cli-tools/{id}", s.CLI.HandleApply)
	s.Router.HandleFunc("DELETE /api/cli-tools/{id}", s.CLI.HandleReset)

	// Endpoints & Skills
	s.Router.HandleFunc("GET /api/endpoints", s.Endpoints.HandleEndpoints)
	s.Router.HandleFunc("GET /api/skills", s.Endpoints.HandleSkills)

	// MITM proxy management
	s.Router.HandleFunc("GET /api/mitm/status", s.MITM.HandleStatus)
	s.Router.HandleFunc("POST /api/mitm/start", s.MITM.HandleStart)
	s.Router.HandleFunc("POST /api/mitm/stop", s.MITM.HandleStop)
	s.Router.HandleFunc("GET /api/mitm/cert", s.MITM.HandleCert)
	s.Router.HandleFunc("POST /api/mitm/dns", s.MITM.HandleDNS)
	s.Router.HandleFunc("GET /api/mitm/traffic", s.MITM.HandleTraffic)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if err := s.DB.Ping(); err != nil {
		dbStatus = "error"
	}

	conns, _ := s.DB.ListConnections()
	activeCount := 0
	for _, c := range conns {
		if c.IsActive {
			activeCount++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                true,
		"service":           "cyrene-gateway",
		"status":            "active",
		"time":              time.Now().UTC().Format(time.RFC3339),
		"db":                dbStatus,
		"uptimeSeconds":     int(time.Since(s.startTime).Seconds()),
		"connections":       len(conns),
		"activeConnections": activeCount,
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":    Version(),
		"service":    "cyrene-gateway",
		"refactored": "9router (Next.js) → Go",
	})
}

func (s *Server) handleRegistry(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	if category != "" {
		// Filter by category
		providers := make([]provider.ProviderInfo, 0)
		for _, p := range provider.Registry {
			if p.Category == category {
				providers = append(providers, p)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"providers": providers, "count": len(providers)})
		return
	}
	// Return grouped by category with counts
	categories := provider.GetRegistryByCategory()
	total := len(provider.Registry)
	writeJSON(w, http.StatusOK, map[string]any{"categories": categories, "total": total})
}

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
		models = append(models, ModelEntry{
			ID:      c.Name,
			Object:  "model",
			OwnedBy: "cyrene-gateway",
		})
	}

	// Group active connections by provider
	activeConnsByProvider := make(map[string][]model.ProviderConnection)
	for _, conn := range conns {
		if conn.IsActive {
			activeConnsByProvider[conn.Provider] = append(activeConnsByProvider[conn.Provider], conn)
		}
	}

	seenProviders := make(map[string]bool)
	seenFullIDs := make(map[string]bool)

	// Helper to extract and append models for a provider
	appendProviderModels := func(providerID string, isUnauthOpenCode bool, pConns []model.ProviderConnection) {
		var unifiedModels []provider.ModelRef
		seenModel := make(map[string]bool)

		// 1. Live cached models
		if raw, err := s.DB.KVGet("providerModelCache", providerID); err == nil && raw != "" {
			var cached model.CachedModels
			if err := json.Unmarshal([]byte(raw), &cached); err == nil && len(cached.Models) > 0 {
				for _, m := range cached.Models {
					if isUnauthOpenCode && !provider.IsOpenCodeFreeModel(m.ID) {
						continue
					}
					seenModel[m.ID] = true
					name := m.DisplayName
					if name == "" {
						name = m.ID
					}
					unifiedModels = append(unifiedModels, provider.ModelRef{ID: m.ID, Name: name})
				}
			}
		}

		// 2. Fallback registry models
		if regModels, ok := provider.RegistryModels[providerID]; ok {
			for _, m := range regModels {
				if isUnauthOpenCode && !provider.IsOpenCodeFreeModel(m.ID) {
					continue
				}
				if !seenModel[m.ID] {
					seenModel[m.ID] = true
					unifiedModels = append(unifiedModels, m)
				}
			}
		}

		// 3. Fallback for unauthenticated OpenCode if no models found
		if isUnauthOpenCode && len(unifiedModels) == 0 {
			for _, fm := range provider.GetOpenCodeFreeModels() {
				if !seenModel[fm.ID] {
					seenModel[fm.ID] = true
					unifiedModels = append(unifiedModels, fm)
				}
			}
		}

		// 4. Custom models from connections of this provider
		for _, conn := range pConns {
			for _, cm := range s.loadCustomModels(conn.ID) {
				if isUnauthOpenCode && !provider.IsOpenCodeFreeModel(cm.ID) {
					continue
				}
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

		// Append to models list with metadata and disable-check
		for _, m := range unifiedModels {
			fullID := providerID + "/" + m.ID
			if seenFullIDs[fullID] || isModelDisabled(fullID, m.ID) {
				continue
			}
			seenFullIDs[fullID] = true

			meta := model.MergeMetadata(m.ID, nil, cacheIndex[fullID])
			displayName := meta.DisplayName
			if displayName == m.ID && m.Name != "" {
				displayName = m.Name
			}
			if overrides != nil && overrides[fullID] != "" {
				var ov ModelMetaOverride
				if err := json.Unmarshal([]byte(overrides[fullID]), &ov); err == nil {
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

	// Add models for all providers with active connections
	for providerID, pConns := range activeConnsByProvider {
		seenProviders[providerID] = true
		hasApiKey := false
		for _, c := range pConns {
			if c.Data.APIKey != "" {
				hasApiKey = true
				break
			}
		}
		isUnauthOpenCode := providerID == "opencode" && !hasApiKey
		appendProviderModels(providerID, isUnauthOpenCode, pConns)
	}

	// Add NoAuth (free) providers even without connections
	for id, p := range provider.Registry {
		if p.NoAuth && !p.Hidden && !seenProviders[id] {
			isUnauthOpenCode := id == "opencode"
			appendProviderModels(id, isUnauthOpenCode, nil)
		}
	}

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

// handleRefreshModels triggers a live model fetch for a provider and caches the result.
func (s *Server) handleRefreshModels(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("id")

	providerInfo, ok := provider.GetProvider(providerID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown provider"})
		return
	}
	// Find an active connection for this provider, or use NoAuth default for free providers
	conns, err := s.DB.ListConnectionsByProvider(providerID)
	baseURL := providerInfo.BaseURL
	var conn model.ProviderConnection
	if err == nil && len(conns) > 0 {
		conn = conns[0]
		if conn.Data.BaseURL != "" {
			baseURL = conn.Data.BaseURL
		}
	} else if !providerInfo.NoAuth && providerInfo.Category != "free" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "no active connection for provider: " + providerID,
		})
		return
	}
	client := &http.Client{Timeout: 30 * time.Second}

	// Qoder: COSY-signed catalog (resolve PATs first) — its inference
	// protocol has no standard /models endpoint.
	var models []model.ModelMetadata
	if providerID == "qoder" {
		models = s.fetchQoderCatalog(&conn, client)
		if models == nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "qoder model catalog fetch failed"})
			return
		}
	} else {
		// Phase 36 T6: registry ModelsURL + format-derived auth scheme.
		cfg := provider.ModelsFetchFor(providerInfo)
		if conn.Data.BaseURL != "" {
			cfg.URL = "" // user base URL override → derive from it
		}
		fetched, fetchErr := model.FetchModels(client, providerID, baseURL, conn.Data.APIKey, conn.Data.AccessToken, cfg)
		if fetchErr != nil {
			slog.Warn("Model refresh failed", slog.String("provider", providerID), "error", fetchErr)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": fetchErr.Error()})
			return
		}
		models = fetched

		// models.dev backfill for context/output metadata when the provider
		// API omits it (best-effort, never overwrites live values).
		if catalog, catErr := model.LoadModelsDevCatalog(client); catErr == nil {
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

	resolved, err := provider.ResolveQoderCredential(token, userID, client)
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
		UserID:    resolved.UserID,
		AuthToken: resolved.AccessToken,
		Name:      conn.Name,
		Email:     conn.Email,
		MachineID: machineID,
	}
	models := provider.QoderCatalogModels(creds, client, false)
	if models == nil {
		// Force one refresh — the cache may not be populated yet.
		models = provider.QoderCatalogModels(creds, client, true)
	}
	return models
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

	client := &http.Client{Timeout: 30 * time.Second}
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

	// Also sync public free providers (like opencode) to keep real-time available models updated
	for id, p := range provider.Registry {
		if (p.NoAuth || p.Category == "free") && !seen[id] && !p.Hidden {
			seen[id] = true
			targets = append(targets, syncTarget{
				providerID: id,
				baseURL:    p.BaseURL,
			})
		}
	}

	for _, target := range targets {
		pInfo, _ := provider.GetProvider(target.providerID)
		var models []model.ModelMetadata
		if target.providerID == "qoder" && target.conn != nil {
			models = s.fetchQoderCatalog(target.conn, client)
		} else {
			cfg := provider.ModelsFetchFor(pInfo)
			fetched, err := model.FetchModels(client, target.providerID, target.baseURL, target.apiKey, target.accessToken, cfg)
			if err == nil && len(fetched) > 0 {
				models = fetched
			}
		}
		if len(models) == 0 {
			continue
		}

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
	client := &http.Client{Timeout: 30 * time.Second}
	var models []model.ModelMetadata
	if conn.Provider == "qoder" {
		models = s.fetchQoderCatalog(conn, client)
	} else {
		cfg := provider.ModelsFetchFor(pInfo)
		fetched, err := model.FetchModels(client, conn.Provider, baseURL, conn.Data.APIKey, conn.Data.AccessToken, cfg)
		if err == nil && len(fetched) > 0 {
			models = fetched
		}
	}
	if len(models) > 0 {
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

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.DB.GetSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get settings"})
		return
	}
	// Redact PasswordHash from settings response
	sanitized := *settings
	sanitized.PasswordHash = ""
	writeJSON(w, http.StatusOK, sanitized)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var settings db.Settings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := s.DB.SaveSettings(&settings); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save settings"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// handlePatchSettings merges a partial JSON object into the existing settings.
// This allows the frontend to update individual fields (e.g. providerStrategies)
// without needing to send the full settings blob.
func (s *Server) handlePatchSettings(w http.ResponseWriter, r *http.Request) {
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	current, err := s.DB.GetSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get settings"})
		return
	}

	// Marshal current settings, unmarshal patch on top, re-save
	currentBytes, _ := json.Marshal(current)
	var merged map[string]json.RawMessage
	json.Unmarshal(currentBytes, &merged)
	maps.Copy(merged, patch)
	mergedBytes, _ := json.Marshal(merged)

	var updated db.Settings
	if err := json.Unmarshal(mergedBytes, &updated); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid patch data"})
		return
	}
	if err := s.DB.SaveSettings(&updated); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save settings"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	conns, err := s.DB.ListConnections()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	dtos := make([]model.ConnectionDTO, len(conns))
	for i, c := range conns {
		dtos[i] = c.ToDTO()
	}
	writeJSON(w, http.StatusOK, dtos)
}

func (s *Server) handleGetProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conn, err := s.DB.GetConnection(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}
	writeJSON(w, http.StatusOK, conn.ToDTO())
}

func (s *Server) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data     map[string]any `json:"data"`
		ID       string         `json:"id"`
		Provider string         `json:"provider"`
		AuthType string         `json:"authType"`
		Name     string         `json:"name"`
		Email    string         `json:"email"`
		Priority int            `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider is required"})
		return
	}
	if req.ID == "" {
		req.ID = generateID()
	}
	if req.AuthType == "" {
		req.AuthType = "api-key"
	}

	// Convert generic data map to ConnectionData
	dataBytes, _ := json.Marshal(req.Data)
	var connData model.ConnectionData
	if err := json.Unmarshal(dataBytes, &connData); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid data payload"})
		return
	}

	if req.AuthType == "api-key" && connData.APIKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "apiKey is required for api-key auth"})
		return
	}

	allowPrivate := s.Config != nil && s.Config.AllowPrivateNetworks
	if connData.BaseURL != "" {
		if _, err := provider.ValidateUpstreamURL(connData.BaseURL, allowPrivate); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid baseUrl: " + err.Error()})
			return
		}
	}

	// Duplicate guard: one active connection per provider+authType
	existing, err := s.DB.ListConnectionsByProvider(req.Provider)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check existing connections"})
		return
	}
	for _, c := range existing {
		if c.AuthType == req.AuthType {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "connection for this provider already exists"})
			return
		}
	}
	pc := &model.ProviderConnection{
		ID:       req.ID,
		Provider: req.Provider,
		AuthType: req.AuthType,
		Name:     req.Name,
		Email:    req.Email,
		Priority: req.Priority,
		IsActive: true,
		Data:     connData,
	}

	if err := s.DB.CreateConnection(pc); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create connection"})
		return
	}

	// Trigger async dynamic model catalog discovery for the newly added connection
	go s.syncConnectionModels(pc)

	writeJSON(w, http.StatusCreated, pc.ToDTO())
}

func (s *Server) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.DB.GetConnection(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	var req struct {
		Provider *string        `json:"provider"`
		AuthType *string        `json:"authType"`
		Name     *string        `json:"name"`
		Email    *string        `json:"email"`
		Priority *int           `json:"priority"`
		IsActive *bool          `json:"isActive"`
		Data     map[string]any `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Provider != nil {
		existing.Provider = *req.Provider
	}
	if req.AuthType != nil {
		existing.AuthType = *req.AuthType
	}
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Email != nil {
		existing.Email = *req.Email
	}
	if req.Priority != nil {
		existing.Priority = *req.Priority
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}
	if req.Data != nil {
		dataBytes, _ := json.Marshal(req.Data)
		var incomingData model.ConnectionData
		json.Unmarshal(dataBytes, &incomingData)

		// Preserve existing secrets if empty in patch
		if incomingData.APIKey == "" {
			incomingData.APIKey = existing.Data.APIKey
		}
		if incomingData.AccessToken == "" {
			incomingData.AccessToken = existing.Data.AccessToken
		}
		if incomingData.RefreshToken == "" {
			incomingData.RefreshToken = existing.Data.RefreshToken
		}
		if incomingData.ProviderSpecificData == nil && existing.Data.ProviderSpecificData != nil {
			incomingData.ProviderSpecificData = existing.Data.ProviderSpecificData
		}
		if incomingData.BaseURL != "" {
			allowPrivate := s.Config != nil && s.Config.AllowPrivateNetworks
			if _, err := provider.ValidateUpstreamURL(incomingData.BaseURL, allowPrivate); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid baseUrl: " + err.Error()})
				return
			}
		}
		existing.Data = incomingData
	}
	if err := s.DB.UpdateConnection(existing); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update connection"})
		return
	}
	writeJSON(w, http.StatusOK, existing.ToDTO())
}

func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.DB.DeleteConnection(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete connection"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (s *Server) handleResetProviderStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conn, err := s.DB.GetConnection(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	provider.ResetAccountState(conn)
	provider.ClearModelLocks(conn)

	if err := s.DB.UpdateConnection(conn); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reset connection"})
		return
	}
	writeJSON(w, http.StatusOK, conn.ToDTO())
}

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.DB.ListNodes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleCreateNode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type string `json:"type"`
		Name string `json:"name"`
		Data struct {
			Prefix  string `json:"prefix"`
			APIType string `json:"apiType"`
			BaseURL string `json:"baseUrl"`
			APIKey  string `json:"apiKey"`
		} `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	n := &model.ProviderNode{
		ID:   generateID(),
		Type: req.Type,
		Name: req.Name,
		Data: model.NodeData{
			Prefix:  req.Data.Prefix,
			APIType: req.Data.APIType,
			BaseURL: req.Data.BaseURL,
			APIKey:  req.Data.APIKey,
		},
	}

	if err := s.DB.CreateNode(n); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create node"})
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) handleUpdateNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.DB.GetNode(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "node not found"})
		return
	}

	var req struct {
		Type *string `json:"type"`
		Name *string `json:"name"`
		Data *struct {
			Prefix  string `json:"prefix"`
			APIType string `json:"apiType"`
			BaseURL string `json:"baseUrl"`
			APIKey  string `json:"apiKey"`
		} `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Type != nil {
		existing.Type = *req.Type
	}
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Data != nil {
		existing.Data = model.NodeData{
			Prefix:  req.Data.Prefix,
			APIType: req.Data.APIType,
			BaseURL: req.Data.BaseURL,
			APIKey:  req.Data.APIKey,
		}
	}

	if err := s.DB.UpdateNode(existing); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update node"})
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.DB.DeleteNode(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete node"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (s *Server) handleListCombos(w http.ResponseWriter, r *http.Request) {
	combos, err := s.DB.ListCombos()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, combos)
}

func (s *Server) handleCreateCombo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string   `json:"name"`
		Kind   string   `json:"kind"`
		Models []string `json:"models"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	c := &model.Combo{
		ID:     generateID(),
		Name:   req.Name,
		Kind:   req.Kind,
		Models: req.Models,
	}

	if err := s.DB.CreateCombo(c); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create combo"})
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleUpdateCombo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	combos, err := s.DB.ListCombos()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	var existing *model.Combo
	for i := range combos {
		if combos[i].ID == id {
			existing = &combos[i]
			break
		}
	}
	if existing == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "combo not found"})
		return
	}

	var req struct {
		Name   *string   `json:"name"`
		Kind   *string   `json:"kind"`
		Models *[]string `json:"models"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Kind != nil {
		existing.Kind = *req.Kind
	}
	if req.Models != nil {
		existing.Models = *req.Models
	}

	if err := s.DB.UpdateCombo(existing); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update combo"})
		return
	}
	s.Combos.ResetRotation(existing.Name)
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) handleDeleteCombo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.DB.DeleteCombo(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete combo"})
		return
	}
	s.Combos.ResetRotation("")
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.DB.ListAPIKeys()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (s *Server) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	key := &model.APIKey{
		ID:       generateID(),
		Key:      auth.GenerateAPIKey(),
		Name:     req.Name,
		IsActive: true,
	}

	if err := s.DB.CreateAPIKey(key); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create key"})
		return
	}
	writeJSON(w, http.StatusCreated, key)
}

func (s *Server) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.DB.DeleteAPIKey(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete key"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (s *Server) handleListAliases(w http.ResponseWriter, r *http.Request) {
	aliases, err := s.DB.KVList("aliases")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, aliases)
}

func (s *Server) handleSetAlias(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Alias  string `json:"alias"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Alias == "" || req.Target == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "alias and target required"})
		return
	}
	if err := s.DB.KVSet("aliases", req.Alias, req.Target); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to set alias"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (s *Server) handleDeleteAlias(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Alias string `json:"alias"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := s.DB.KVDelete("aliases", req.Alias); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete alias"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (s *Server) handleListDisabledModels(w http.ResponseWriter, r *http.Request) {
	disabled, err := s.DB.KVList("disabledModels")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, disabled)
}

func (s *Server) handleDisableModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model required"})
		return
	}
	if err := s.DB.KVSet("disabledModels", req.Model, "true"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to disable model"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (s *Server) handleEnableModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := s.DB.KVDelete("disabledModels", req.Model); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enable model"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (s *Server) handleListProxyPools(w http.ResponseWriter, r *http.Request) {
	pools, err := s.DB.ListProxyPools()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proxyPools": pools})
}

func (s *Server) handleGetProxyPool(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	pool, err := s.DB.GetProxyPool(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "proxy pool not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proxyPool": pool})
}

var validProxyTypes = map[string]bool{"http": true, "vercel": true, "cloudflare": true, "deno": true}

func (s *Server) handleCreateProxyPool(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		ProxyURL    string `json:"proxyUrl"`
		NoProxy     string `json:"noProxy"`
		Type        string `json:"type"`
		StrictProxy bool   `json:"strictProxy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if req.ProxyURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "proxyUrl is required"})
		return
	}
	if !validProxyTypes[req.Type] {
		req.Type = "http"
	}

	p := &model.ProxyPool{
		ID:       generateID(),
		IsActive: true,
		Data: model.ProxyPoolData{
			Name:        req.Name,
			ProxyURL:    req.ProxyURL,
			NoProxy:     req.NoProxy,
			StrictProxy: req.StrictProxy,
			Type:        req.Type,
		},
	}

	if err := s.DB.CreateProxyPool(p); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create proxy pool"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"proxyPool": p})
}

func (s *Server) handleUpdateProxyPool(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.DB.GetProxyPool(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "proxy pool not found"})
		return
	}

	var req struct {
		Name        *string `json:"name"`
		ProxyURL    *string `json:"proxyUrl"`
		NoProxy     *string `json:"noProxy"`
		StrictProxy *bool   `json:"strictProxy"`
		Type        *string `json:"type"`
		IsActive    *bool   `json:"isActive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Name != nil {
		if *req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		existing.Data.Name = *req.Name
	}
	if req.ProxyURL != nil {
		if *req.ProxyURL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "proxyUrl is required"})
			return
		}
		existing.Data.ProxyURL = *req.ProxyURL
	}
	if req.NoProxy != nil {
		existing.Data.NoProxy = *req.NoProxy
	}
	if req.StrictProxy != nil {
		existing.Data.StrictProxy = *req.StrictProxy
	}
	if req.Type != nil {
		if !validProxyTypes[*req.Type] {
			existing.Data.Type = "http"
		} else {
			existing.Data.Type = *req.Type
		}
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	if err := s.DB.UpdateProxyPool(existing); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update proxy pool"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proxyPool": existing})
}

func (s *Server) handleDeleteProxyPool(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, err := s.DB.GetProxyPool(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "proxy pool not found"})
		return
	}

	// Check if any connection is bound to this pool
	conns, _ := s.DB.ListConnections()
	boundCount := 0
	for _, c := range conns {
		if c.Data.ProviderSpecificData != nil {
			if poolID, ok := c.Data.ProviderSpecificData["proxyPoolId"]; ok {
				if poolID == id {
					boundCount++
				}
			}
		}
	}
	if boundCount > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":                "proxy pool is currently in use",
			"boundConnectionCount": boundCount,
		})
		return
	}

	if err := s.DB.DeleteProxyPool(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete proxy pool"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
