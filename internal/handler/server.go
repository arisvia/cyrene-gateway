package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/config"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/middleware"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

type Server struct {
	DB        *db.DB
	Router    *http.ServeMux
	Handler   http.Handler // Router wrapped with middleware
	Combos    *provider.ComboManager
	Dashboard *DashboardHandler
}

func NewServer(database *db.DB, cfg *config.Config) *Server {
	mux := http.NewServeMux()
	s := &Server{
		DB:        database,
		Router:    mux,
		Combos:    provider.NewComboManager(),
		Dashboard: NewDashboardHandler(cfg),
	}
	s.registerRoutes()

	// Wrap the mux with the middleware chain
	s.Handler = middleware.Chain(mux,
		middleware.Recovery,
		middleware.Logging,
		middleware.CORS,
	)
	return s
}

func (s *Server) registerRoutes() {
	// Dashboard panel (root)
	s.Router.Handle("GET /{$}", s.Dashboard)

	// Health & meta
	s.Router.HandleFunc("GET /api/health", s.handleHealth)
	s.Router.HandleFunc("GET /api/version", s.handleVersion)

	// OpenAI-compatible API surface
	s.Router.HandleFunc("GET /v1/models", s.handleModels)
	s.Router.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
	s.Router.HandleFunc("POST /v1/embeddings", s.handleEmbeddings)
	s.Router.HandleFunc("POST /v1/messages", s.handleMessages)

	// Dashboard management API
	s.Router.HandleFunc("GET /api/settings", s.handleGetSettings)
	s.Router.HandleFunc("PUT /api/settings", s.handlePutSettings)
	s.Router.HandleFunc("GET /api/providers", s.handleListProviders)
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
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "cyrene-gateway",
		"status":  "active",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":    Version(),
		"service":    "cyrene-gateway",
		"refactored": "9router (Next.js) → Go",
	})
}

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
	}

	var models []ModelEntry

	// Add aliases as models
	aliases, _ := s.DB.KVList("aliases")
	for alias := range aliases {
		models = append(models, ModelEntry{
			ID:      alias,
			Object:  "model",
			OwnedBy: "cyrene-gateway",
		})
	}

	// Add combo names as models
	combos, _ := s.DB.ListCombos()
	for _, c := range combos {
		models = append(models, ModelEntry{
			ID:      c.Name,
			Object:  "model",
			OwnedBy: "cyrene-gateway",
		})
	}

	// Add provider wildcard entries from active connections
	seen := make(map[string]bool)
	for _, conn := range conns {
		if !conn.IsActive || seen[conn.Provider] {
			continue
		}
		seen[conn.Provider] = true
		if _, ok := provider.GetProvider(conn.Provider); ok {
			models = append(models, ModelEntry{
				ID:      conn.Provider + "/*",
				Object:  "model",
				OwnedBy: conn.Provider,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   models,
	})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.DB.GetSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get settings"})
		return
	}
	writeJSON(w, http.StatusOK, settings)
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

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	conns, err := s.DB.ListConnections()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, conns)
}

func (s *Server) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string         `json:"id"`
		Provider string         `json:"provider"`
		AuthType string         `json:"authType"`
		Name     string         `json:"name"`
		Email    string         `json:"email"`
		Priority int            `json:"priority"`
		Data     map[string]any `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
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
	json.Unmarshal(dataBytes, &connData)

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
	writeJSON(w, http.StatusCreated, pc)
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
		var connData model.ConnectionData
		json.Unmarshal(dataBytes, &connData)
		existing.Data = connData
	}

	if err := s.DB.UpdateConnection(existing); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update connection"})
		return
	}
	writeJSON(w, http.StatusOK, existing)
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
	writeJSON(w, http.StatusOK, conn)
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
		Key:      generateAPIKey(),
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
