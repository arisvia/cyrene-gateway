package handler

import (
	"encoding/json"
	"net/http"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/db"
)

// AuthHandler handles dashboard authentication endpoints.
type AuthHandler struct {
	db *db.DB
}

func NewAuthHandler(database *db.DB) *AuthHandler {
	return &AuthHandler{db: database}
}

func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	ip := auth.ClientIP(r.RemoteAddr)

	// Check rate limit
	if locked, retryAfter := auth.CheckLock(ip); locked {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":      "too many failed attempts",
			"retryAfter": retryAfter,
		})
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Password == "" {
		auth.RecordFail(ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "password required"})
		return
	}

	// Verify password against stored hash
	settings, err := h.db.GetSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	valid := false
	if settings.PasswordHash != "" {
		valid = auth.VerifyPassword(req.Password, settings.PasswordHash)
	} else {
		// Default password when none is set
		valid = req.Password == "123456"
	}

	if !valid {
		auth.RecordFail(ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid password"})
		return
	}

	auth.RecordSuccess(ip)

	token, err := auth.CreateSessionToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create session"})
		return
	}

	// Set cookie
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})

	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *AuthHandler) HandleSetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if len(req.Password) < 6 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 6 characters"})
		return
	}

	settings, err := h.db.GetSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	settings.PasswordHash = auth.HashPassword(req.Password)
	if err := h.db.SaveSettings(settings); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save password"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *AuthHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	settings, err := h.db.GetSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	authenticated := false
	if !settings.RequireLogin {
		authenticated = true
	} else {
		cookie, err := r.Cookie("auth_token")
		if err == nil {
			authenticated = auth.VerifySessionToken(cookie.Value)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"requireLogin":  settings.RequireLogin,
		"authenticated": authenticated,
	})
}
