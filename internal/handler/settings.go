package handler

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/db"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.DB.GetSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get settings"})
		return
	}
	// Redact PasswordHash from settings response and populate HasPassword flag
	sanitized := *settings
	sanitized.HasPassword = settings.PasswordHash != ""
	sanitized.PasswordHash = ""
	writeJSON(w, http.StatusOK, sanitized)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var settings db.Settings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	current, err := s.DB.GetSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get settings"})
		return
	}

	// Preserve password hash if omitted/empty (passwords are set via /api/auth/password)
	if settings.PasswordHash == "" && current != nil {
		settings.PasswordHash = current.PasswordHash
	}

	// Guard: cannot require login without an initialized password (prevents lockout)
	if settings.RequireLogin && settings.PasswordHash == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "admin password must be configured before enabling login requirement"})
		return
	}
	// Guard: cannot disable login requirement when accessing from public WAN or tunnel domain
	if current != nil && current.RequireLogin && !settings.RequireLogin && auth.IsPublicWANRequest(r) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot disable login requirement when accessing from public WAN"})
		return
	}
	if err := s.DB.SaveSettings(&settings); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save settings"})
		return
	}
	if s.Cache != nil && current != nil && cacheAffectingSettingsChanged(current, &settings) {
		s.Cache.Clear()
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
	// Do not allow wiping or mutating password hash via generic patch; password is set via /api/auth/password
	delete(patch, "passwordHash")

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

	// Guard: cannot require login without an initialized password (prevents lockout)
	if updated.RequireLogin && updated.PasswordHash == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "admin password must be configured before enabling login requirement"})
		return
	}
	// Guard: cannot disable login requirement when accessing from public WAN or tunnel domain
	if current.RequireLogin && !updated.RequireLogin && auth.IsPublicWANRequest(r) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot disable login requirement when accessing from public WAN"})
		return
	}
	if err := s.DB.SaveSettings(&updated); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save settings"})
		return
	}
	if s.Cache != nil && cacheAffectingSettingsChanged(current, &updated) {
		s.Cache.Clear()
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (s *Server) handleGetCacheStats(w http.ResponseWriter, r *http.Request) {
	if s.Cache == nil {
		writeJSON(w, http.StatusOK, map[string]any{"entries": 0, "hits": 0, "misses": 0})
		return
	}
	writeJSON(w, http.StatusOK, s.Cache.Stats())
}

func cacheAffectingSettingsChanged(a, b *db.Settings) bool {
	if a == nil || b == nil {
		return true
	}
	if a.ResponseCacheEnabled != b.ResponseCacheEnabled ||
		a.ResponseCacheTTL != b.ResponseCacheTTL ||
		a.ResponseCacheAll != b.ResponseCacheAll ||
		a.RTKEnabled != b.RTKEnabled ||
		a.CavemanEnabled != b.CavemanEnabled ||
		a.CavemanLevel != b.CavemanLevel ||
		a.PonytailEnabled != b.PonytailEnabled ||
		a.PonytailLevel != b.PonytailLevel ||
		a.LoopGuardEnabled != b.LoopGuardEnabled ||
		a.AutoPromptCache != b.AutoPromptCache ||
		!slices.Equal(a.TokenSaverExclude, b.TokenSaverExclude) {
		return true
	}
	return false
}

func (s *Server) handleClearCache(w http.ResponseWriter, r *http.Request) {
	if s.Cache != nil {
		s.Cache.Clear()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
