package handler

import (
	"encoding/json"
	"net/http"

	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// customModelsScope is the KV scope storing per-connection custom model lists.
const customModelsScope = "providerModels"

// customModel is a user-defined model attached to a specific connection.
type customModel struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// loadCustomModels reads the custom model list for a connection from the KV store.
func (s *Server) loadCustomModels(connID string) []customModel {
	raw, err := s.DB.KVGet(customModelsScope, connID)
	if err != nil || raw == "" {
		return nil
	}
	var models []customModel
	if err := json.Unmarshal([]byte(raw), &models); err != nil {
		return nil
	}
	return models
}

// saveCustomModels persists the custom model list for a connection.
func (s *Server) saveCustomModels(connID string, models []customModel) error {
	data, err := json.Marshal(models)
	if err != nil {
		return err
	}
	return s.DB.KVSet(customModelsScope, connID, string(data))
}

// handleGetProviderModels returns registry models + custom models for a connection.
func (s *Server) handleGetProviderModels(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conn, err := s.DB.GetConnection(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	registryModels := provider.GetRegistryModels(conn.Provider)
	if registryModels == nil {
		registryModels = []provider.ModelRef{}
	}
	customModels := s.loadCustomModels(conn.ID)
	if customModels == nil {
		customModels = []customModel{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"provider":       conn.Provider,
		"registryModels": registryModels,
		"customModels":   customModels,
	})
}

// handleAddProviderModel adds a custom model to a connection.
func (s *Server) handleAddProviderModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conn, err := s.DB.GetConnection(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	var req customModel
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model id is required"})
		return
	}
	if req.Name == "" {
		req.Name = req.ID
	}

	models := s.loadCustomModels(conn.ID)
	for _, m := range models {
		if m.ID == req.ID {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "model already exists"})
			return
		}
	}
	models = append(models, req)
	if err := s.saveCustomModels(conn.ID, models); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save model"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"customModels": models})
}

// handleDeleteProviderModel removes a custom model from a connection.
func (s *Server) handleDeleteProviderModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conn, err := s.DB.GetConnection(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	var req struct {
		ModelID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.ModelID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model id is required"})
		return
	}

	models := s.loadCustomModels(conn.ID)
	filtered := models[:0]
	for _, m := range models {
		if m.ID != req.ModelID {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) == len(models) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "model not found"})
		return
	}
	if err := s.saveCustomModels(conn.ID, filtered); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete model"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"customModels": filtered})
}
