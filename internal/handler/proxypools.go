package handler

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
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
	parsed, err := provider.ValidateProxyURL(req.ProxyURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid proxyUrl: " + err.Error()})
		return
	}

	p := &model.ProxyPool{
		ID:       generateID(),
		IsActive: true,
		Data: model.ProxyPoolData{
			Name:        req.Name,
			ProxyURL:    req.ProxyURL,
			StrictProxy: req.StrictProxy,
			// Type is a display label derived from the URL scheme, never
			// client-supplied: the dialer keys off the scheme, so a hand-picked
			// label could only ever drift from reality.
			Type: proxySchemeLabel(parsed),
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
		StrictProxy *bool   `json:"strictProxy"`
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
		parsed, err := provider.ValidateProxyURL(*req.ProxyURL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid proxyUrl: " + err.Error()})
			return
		}
		existing.Data.ProxyURL = *req.ProxyURL
		existing.Data.Type = proxySchemeLabel(parsed)
	}
	if req.StrictProxy != nil {
		existing.Data.StrictProxy = *req.StrictProxy
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
	if _, err := s.DB.GetProxyPool(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "proxy pool not found"})
		return
	}
	if err := s.DB.DeleteProxyPool(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete proxy pool"})
		return
	}
	s.refreshProxies()
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// proxySchemeLabel turns a validated proxy URL's scheme into the display type.
// The dialer routes purely off the scheme, so this keeps the badge truthful
// (socks5://… shows socks5) instead of a client-picked label that can drift.
func proxySchemeLabel(u *url.URL) string {
	switch u.Scheme {
	case "socks5", "socks5h":
		return "socks5"
	case "https":
		return "https"
	default:
		return "http"
	}
}

// refreshProxies re-syncs the in-memory ProxyManager with current database proxy pools.
func (s *Server) refreshProxies() {
	if s.Proxies == nil || s.DB == nil {
		return
	}
	if pools, err := s.DB.ListProxyPools(); err == nil {
		s.Proxies.UpdatePools(pools)
	}
}
