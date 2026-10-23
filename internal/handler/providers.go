package handler

import (
	"encoding/json"
	"net/http"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

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
	// Routing and listing key off the canonical id; storing a shorthand would
	// strand the connection out of every candidate set.
	req.Provider = provider.ResolveProviderAlias(req.Provider)
	if req.ID == "" {
		req.ID = generateID()
	}
	if req.AuthType == "" {
		req.AuthType = "api-key"
	}
	if !requireKnownAuthType(w, req.AuthType) {
		return
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

	// Duplicate guard: prevent duplicate nameless accounts or duplicate identical credentials
	existing, err := s.DB.ListConnectionsByProvider(req.Provider)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check existing connections"})
		return
	}
	for _, c := range existing {
		if c.AuthType == req.AuthType {
			if req.Name == "" || req.Name == c.Name || (connData.APIKey != "" && c.Data.APIKey == connData.APIKey) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "connection for this provider already exists"})
				return
			}
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

	dto := pc.ToDTO()
	pcCopy := *pc
	// Trigger async dynamic model catalog discovery for the newly added connection
	s.goBackground(func() { s.syncConnectionModels(&pcCopy) })

	writeJSON(w, http.StatusCreated, dto)
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
		existing.Provider = provider.ResolveProviderAlias(*req.Provider)
	}
	if req.AuthType != nil {
		if !requireKnownAuthType(w, *req.AuthType) {
			return
		}
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

	dto := existing.ToDTO()
	existingCopy := *existing
	// Trigger async dynamic model catalog discovery for updated connection
	s.goBackground(func() { s.syncConnectionModels(&existingCopy) })

	writeJSON(w, http.StatusOK, dto)
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
	lockPatch := provider.ModelLockRemovalPatch(conn.Data.ProviderSpecificData)
	provider.ClearModelLocks(conn)

	if err := s.DB.UpdateConnectionHealth(conn.ID, conn.Data); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reset connection"})
		return
	}
	if len(lockPatch) > 0 {
		if err := s.DB.UpdateConnectionProviderData(conn.ID, lockPatch); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reset model locks"})
			return
		}
	}
	writeJSON(w, http.StatusOK, conn.ToDTO())
}

// requireKnownAuthType guards the two connection write boundaries. An arbitrary
// authType stores a row no credential resolver can serve, and nothing repairs it
// later, so the typo has to fail here rather than surface as a dead connection.
func requireKnownAuthType(w http.ResponseWriter, authType string) bool {
	switch authType {
	case "api-key", "oauth", "access_token":
		return true
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": `authType must be "api-key", "oauth" or "access_token"`})
	return false
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
