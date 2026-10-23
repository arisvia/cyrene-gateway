package handler

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/cache"
	"github.com/arisvia/cyrene-gateway/internal/config"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/media"
	"github.com/arisvia/cyrene-gateway/internal/metrics"
	"github.com/arisvia/cyrene-gateway/internal/middleware"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
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
	Endpoints   *EndpointHandler
	Events      *EventBroadcaster
	Metrics     *metrics.M
	Config      *config.Config
	Cache       *cache.Cache
	startTime   time.Time
	onRestart   func()
	RestartFunc func() error
	restarting  atomic.Bool

	// usageStatsCache collapses the burst of /api/usage/stats calls that several
	// open dashboards produce on every SSE event into one DB read per TTL.
	usageStatsMu    sync.Mutex
	usageStatsCache map[string]usageStatsEntry

	// background tracks fire-and-forget work (catalog sync after a provider
	// write) so tests can wait for it instead of racing a live goroutine.
	background sync.WaitGroup
}

// goBackground runs fn asynchronously while keeping it observable to tests.
func (s *Server) goBackground(fn func()) {
	s.background.Add(1)
	go func() {
		defer s.background.Done()
		fn()
	}()
}

// waitBackground blocks until every async task started via goBackground returns.
func (s *Server) waitBackground() {
	s.background.Wait()
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

	s := &Server{
		DB:          database,
		Router:      mux,
		Combos:      provider.NewComboManager(),
		Proxies:     proxyMgr,
		MediaClient: media.NewClient(),
		Dashboard:   NewDashboardHandler(cfg),
		Auth:        NewAuthHandler(database),
		Endpoints:   NewEndpointHandler(cfg, database),
		Events:      NewEventBroadcaster(),
		Metrics:     metrics.New(Version()),
		Config:      cfg,
		Cache:       cache.New(2000, time.Hour),
		startTime:   time.Now(),
	}
	if s.MediaClient != nil {
		allowPrivate := false
		if s.Config != nil && s.Config.AllowPrivateNetworks {
			allowPrivate = true
		}
		s.MediaClient.HTTPClient = provider.SafeHTTPClient(2*time.Minute, allowPrivate)
	}
	// Hydrate models.dev catalog from SQLite on startup
	if raw, err := database.KVGet("modelsDevCatalog", "cache"); err == nil && raw != "" {
		var dbCat map[string]model.ModelsDevEntry
		if err := json.Unmarshal([]byte(raw), &dbCat); err == nil && len(dbCat) > 0 {
			model.SetModelsDevCatalog(dbCat)
		}
	}
	s.registerRoutes()
	s.registerMediaRoutes()
	// Prometheus scrape endpoint (public; no session required)
	s.Router.Handle("GET /metrics", s.Metrics.Handler())

	s.Handler = middleware.Chain(mux,
		middleware.Recovery,
		middleware.SecurityHeaders,
		middleware.SanitizeInternalHeaders,
		middleware.Logging,
		middleware.RequestSizeLimiter(),
		middleware.CORS,
		middleware.APIKeyAuth(database),
		middleware.APIKeyRateLimit(func(_ string) int {
			if st, err := database.GetSettings(); err == nil && st != nil {
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
	s.Router.HandleFunc("POST /v1/responses", s.handleResponses)
	s.Router.HandleFunc("POST /v1/embeddings", s.handleEmbeddings)
	s.Router.HandleFunc("POST /v1/messages", s.handleMessages)
	s.Router.HandleFunc("POST /v1/messages/count_tokens", s.handleCountTokens)

	// Google Gemini compatible API surface
	s.Router.HandleFunc("POST /v1beta/models/{modelAction...}", s.handleGeminiGenerateContent)
	s.Router.HandleFunc("POST /v1/models/{modelAction...}", s.handleGeminiGenerateContent)

	// Dashboard management API
	s.Router.HandleFunc("GET /api/settings", s.handleGetSettings)
	s.Router.HandleFunc("PUT /api/settings", s.handlePutSettings)
	s.Router.HandleFunc("PATCH /api/settings", s.handlePatchSettings)
	s.Router.HandleFunc("GET /api/cache/stats", s.handleGetCacheStats)
	s.Router.HandleFunc("POST /api/cache/clear", s.handleClearCache)
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
	s.Router.HandleFunc("PUT /api/keys/{id}", s.handleUpdateKey)
	s.Router.HandleFunc("DELETE /api/keys/{id}", s.handleDeleteKey)
	s.Router.HandleFunc("GET /api/models/alias", s.handleListAliases)
	s.Router.HandleFunc("POST /api/models/alias", s.handleSetAlias)
	s.Router.HandleFunc("DELETE /api/models/alias", s.handleDeleteAlias)
	s.Router.HandleFunc("GET /api/models/disabled", s.handleListDisabledModels)
	s.Router.HandleFunc("POST /api/models/disabled", s.handleSetDisabledModels)
	s.Router.HandleFunc("POST /api/models/test", s.handleTestModel)
	s.Router.HandleFunc("GET /api/proxy-pools", s.handleListProxyPools)
	s.Router.HandleFunc("POST /api/proxy-pools", s.handleCreateProxyPool)
	s.Router.HandleFunc("GET /api/proxy-pools/{id}", s.handleGetProxyPool)
	s.Router.HandleFunc("PUT /api/proxy-pools/{id}", s.handleUpdateProxyPool)
	s.Router.HandleFunc("DELETE /api/proxy-pools/{id}", s.handleDeleteProxyPool)
	// System Backup & Restore
	s.Router.HandleFunc("GET /api/system/backup", s.handleExportBackup)
	s.Router.HandleFunc("POST /api/system/restore", s.handleRestoreBackup)

	// System Ops & Maintenance
	s.Router.HandleFunc("GET /api/system/stats", s.handleSystemStats)
	s.Router.HandleFunc("POST /api/system/maintenance", s.handleSystemMaintenance)
	s.Router.HandleFunc("GET /api/system/update/check", s.handleSystemUpdateCheck)
	s.Router.HandleFunc("POST /api/system/update", s.handleSystemUpdate)
	s.Router.HandleFunc("POST /api/system/restart", s.handleSystemRestart)
	s.Router.HandleFunc("POST /api/system/rollback", s.handleSystemRollback)

	// Provider connection testing
	s.Router.HandleFunc("POST /api/providers/{id}/test", s.handleTestProvider)
	s.Router.HandleFunc("POST /api/providers/test-credentials", s.handleTestCredentials)
	s.Router.HandleFunc("POST /api/providers/test-batch", s.handleTestBatch)
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

	s.Router.HandleFunc("GET /api/endpoints", s.Endpoints.HandleEndpoints)
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
		"version": Version(),
		"service": "cyrene-gateway",
	})
}

func (s *Server) handleRegistry(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	type EnrichedRegistryProvider struct {
		provider.ProviderInfo
		Capabilities []string `json:"capabilities"`
	}

	type EnrichedCategory struct {
		Category  string                     `json:"category"`
		Providers []EnrichedRegistryProvider `json:"providers"`
		Count     int                        `json:"count"`
	}

	// Helper to determine capabilities for a provider ID
	getCapabilities := func(id string, isChat bool) []string {
		set := make(map[string]struct{})
		if isChat {
			set["llm"] = struct{}{}
		}
		if mp, ok := media.Registry[id]; ok {
			for _, k := range mp.Kinds {
				set[string(k)] = struct{}{}
			}
		}
		// Desired display order
		order := []string{"llm", "image", "tts", "stt", "video", "embedding", "web-search", "web-fetch"}
		var caps []string
		for _, k := range order {
			if _, exists := set[k]; exists {
				caps = append(caps, k)
				delete(set, k)
			}
		}
		// Kinds missing from the display order would otherwise come out in map order.
		for _, k := range slices.Sorted(maps.Keys(set)) {
			caps = append(caps, k)
		}
		return caps
	}

	// Helper to synthesize pure media providers into EnrichedRegistryProvider
	buildMediaProviders := func() []EnrichedRegistryProvider {
		var list []EnrichedRegistryProvider
		for id, mp := range media.Registry {
			if _, inChat := provider.GetProvider(id); inChat {
				continue
			}
			kinds := make([]string, len(mp.Kinds))
			for i, k := range mp.Kinds {
				kinds[i] = string(k)
			}
			var firstCfg media.ProviderConfig
			for _, cfg := range mp.Configs {
				firstCfg = cfg
				break
			}
			caps := getCapabilities(id, false)
			list = append(list, EnrichedRegistryProvider{
				ID:           id,
				Name:         mp.Name,
				BaseURL:      firstCfg.BaseURL,
				APIType:      "media",
				AuthType:     firstCfg.AuthType,
				Category:     "media",
				AuthHint:     fmt.Sprintf("支持能力: %s", strings.Join(kinds, ", ")),
				AuthModes:    []string{"api-key"},
				APIKeyURL:    media.APIKeyURLs[id],
				Capabilities: caps,
			})
		}
		sort.Slice(list, func(i, j int) bool {
			return list[i].Name < list[j].Name
		})
		return list
	}

	if category != "" {
		if category == "media" {
			mediaProviders := buildMediaProviders()
			writeJSON(w, http.StatusOK, map[string]any{"providers": mediaProviders, "count": len(mediaProviders)})
			return
		}
		// Filter by category
		providers := make([]EnrichedRegistryProvider, 0)
		for _, p := range provider.Registry {
			if p.Category == category {
				providers = append(providers, EnrichedRegistryProvider{
					ProviderInfo: p,
					Capabilities: getCapabilities(p.ID, true),
				})
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"providers": providers, "count": len(providers)})
		return
	}

	// Return grouped by category with counts
	categories := provider.GetRegistryByCategory()
	enrichedCategories := make([]EnrichedCategory, len(categories))
	// Enrich capabilities for chat categories
	for ci, cat := range categories {
		enrichedProviders := make([]EnrichedRegistryProvider, len(cat.Providers))
		for pi, p := range cat.Providers {
			enrichedProviders[pi] = EnrichedRegistryProvider{
				ProviderInfo: p,
				Capabilities: getCapabilities(p.ID, true),
			}
		}
		enrichedCategories[ci] = EnrichedCategory{
			Category:  cat.Category,
			Providers: enrichedProviders,
			Count:     cat.Count,
		}
	}

	mediaProviders := buildMediaProviders()
	if len(mediaProviders) > 0 {
		enrichedCategories = append(enrichedCategories, EnrichedCategory{
			Category:  "media",
			Providers: mediaProviders,
			Count:     len(mediaProviders),
		})
	}
	total := len(provider.Registry) + len(mediaProviders)
	writeJSON(w, http.StatusOK, map[string]any{"categories": enrichedCategories, "total": total})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
