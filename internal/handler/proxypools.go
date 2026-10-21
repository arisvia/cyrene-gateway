package handler

import (
	"encoding/json"
	"net/http"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

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
	s.refreshProxies()
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
	s.refreshProxies()
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
	s.refreshProxies()
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

var validProxyTypes = map[string]bool{"http": true, "vercel": true, "cloudflare": true, "deno": true}

// refreshProxies re-syncs the in-memory ProxyManager with current database proxy pools.
func (s *Server) refreshProxies() {
	if s.Proxies == nil || s.DB == nil {
		return
	}
	if pools, err := s.DB.ListProxyPools(); err == nil {
		s.Proxies.UpdatePools(pools)
	}
}
