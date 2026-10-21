package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

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

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || strings.Contains(req.Name, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "combo name must be non-empty and must not contain '/'"})
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
	if s.Cache != nil {
		s.Cache.Clear()
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
	existing.Name = strings.TrimSpace(existing.Name)
	if existing.Name == "" || strings.Contains(existing.Name, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "combo name must be non-empty and must not contain '/'"})
		return
	}

	if err := s.DB.UpdateCombo(existing); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update combo"})
		return
	}
	if s.Cache != nil {
		s.Cache.Clear()
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
	if s.Cache != nil {
		s.Cache.Clear()
	}
	s.Combos.ResetRotation("")
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}
