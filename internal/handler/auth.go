package handler

import (
	"encoding/json"
	"net/http"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/db"
)

// AuthHandler handles dashboard authentication endpoints. There is no default
// password (P0-3): a fresh instance requires an explicit first-time password
// setup before login works. forceAuth reports whether the server forces login
// regardless of the requireLogin setting (non-loopback bind, 37A).
type AuthHandler struct {
	db        *db.DB
	limiter   *auth.LoginLimiter
	forceAuth bool
}

func NewAuthHandler(database *db.DB, limiter *auth.LoginLimiter, forceAuth bool) *AuthHandler {
	return &AuthHandler{db: database, limiter: limiter, forceAuth: forceAuth}
}

func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	ip := auth.ClientIP(r.RemoteAddr)

	// Check rate limit
	if locked, retryAfter := h.limiter.CheckLock(ip); locked {
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

	settings, err := h.db.GetSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// No password configured: first-time setup must happen before login.
	if settings.PasswordHash == "" {
		h.limiter.RecordFail(ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no password configured, complete first-time setup"})
		return
	}

	if req.Password == "" || !auth.VerifyPassword(req.Password, settings.PasswordHash) {
		h.limiter.RecordFail(ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid password"})
		return
	}

	h.limiter.RecordSuccess(ip)

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

// HandleSetPassword sets the dashboard password. When a password already
// exists this endpoint requires an authenticated session (enforced by
// DashboardAuth). First-time setup is only reachable while no password is
// stored.
func (h *AuthHandler) HandleSetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if len(req.Password) < auth.MinPasswordLength {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
		return
	}

	settings, err := h.db.GetSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		return
	}

	settings.PasswordHash = hash
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

	hasPassword := settings.PasswordHash != ""
	loginRequired := settings.RequireLogin || h.forceAuth
	authenticated := false
	if !loginRequired {
		authenticated = true
	} else {
		cookie, err := r.Cookie("auth_token")
		if err == nil {
			authenticated = auth.VerifySessionToken(cookie.Value)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"requireLogin":  loginRequired,
		"hasPassword":   hasPassword,
		"authenticated": authenticated,
	})
}
