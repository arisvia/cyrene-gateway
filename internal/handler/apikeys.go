package handler

import (
	"encoding/json"
	"net/http"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

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
		Name          string   `json:"name"`
		AllowedModels []string `json:"allowedModels,omitempty"`
		RPM           int      `json:"rpm,omitempty"`
		SystemPrompt  string   `json:"systemPrompt,omitempty"`
		ExpiresAt     string   `json:"expiresAt,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	key := &model.APIKey{
		ID:            generateID(),
		Key:           auth.GenerateAPIKey(),
		Name:          req.Name,
		IsActive:      true,
		AllowedModels: req.AllowedModels,
		RPM:           req.RPM,
		SystemPrompt:  req.SystemPrompt,
		ExpiresAt:     req.ExpiresAt,
	}

	if err := s.DB.CreateAPIKey(key); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create key"})
		return
	}
	writeJSON(w, http.StatusCreated, key)
}

func (s *Server) handleUpdateKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.DB.GetAPIKey(id)
	if err != nil || existing == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "API key not found"})
		return
	}

	var req struct {
		Name          *string   `json:"name"`
		IsActive      *bool     `json:"isActive"`
		AllowedModels *[]string `json:"allowedModels"`
		RPM           *int      `json:"rpm"`
		SystemPrompt  *string   `json:"systemPrompt"`
		ExpiresAt     *string   `json:"expiresAt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}
	if req.AllowedModels != nil {
		existing.AllowedModels = *req.AllowedModels
	}
	if req.RPM != nil {
		existing.RPM = *req.RPM
	}
	if req.SystemPrompt != nil {
		existing.SystemPrompt = *req.SystemPrompt
	}
	if req.ExpiresAt != nil {
		existing.ExpiresAt = *req.ExpiresAt
	}

	if err := s.DB.UpdateAPIKey(existing); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update API key"})
		return
	}
	writeJSON(w, http.StatusOK, existing)
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

func (s *Server) handleSetDisabledModels(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model    string   `json:"model"`
		Models   []string `json:"models"`
		Disabled bool     `json:"disabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	targets := req.Models
	if len(targets) == 0 && req.Model != "" {
		targets = []string{req.Model}
	}
	if len(targets) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": 0})
		return
	}

	var err error
	if req.Disabled {
		err = s.DB.KVBatchSet("disabledModels", targets, "true")
	} else {
		err = s.DB.KVBatchDelete("disabledModels", targets)
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update disabled models"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": len(targets)})
}
