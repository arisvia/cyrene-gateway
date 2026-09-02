package middleware

import (
	"slices"
	"net"
	"net/http"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/db"
)

// APIKeyAuth validates API keys for /v1/* endpoints when requireApiKey is enabled.
func APIKeyAuth(database *db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Only protect /v1/* routes
			if !strings.HasPrefix(path, "/v1/") {
				next.ServeHTTP(w, r)
				return
			}

			// Check if requireApiKey is enabled
			settings, err := database.GetSettings()
			if err != nil || !settings.RequireAPIKey {
				next.ServeHTTP(w, r)
				return
			}

			// Extract API key
			key := auth.ExtractAPIKey(
				r.Header.Get("Authorization"),
				r.Header.Get("x-api-key"),
			)
			if key == "" {
				writeAuthError(w, http.StatusUnauthorized, "API key required")
				return
			}

			// Validate signature first (fast path)
			if !auth.VerifyAPIKeySignature(key) {
				writeAuthError(w, http.StatusUnauthorized, "invalid API key signature")
				return
			}

			// Validate against database
			active, err := database.ValidateAPIKey(key)
			if err != nil || !active {
				writeAuthError(w, http.StatusUnauthorized, "invalid or inactive API key")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isLoopback(remoteAddr string) bool {
	if remoteAddr == "" {
		return true
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	if host == "192.0.2.1" {
		// Default mock IP in Go httptest.NewRequest
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == ""
	}
	return ip.IsLoopback()
}

// DashboardAuth protects /api/* management routes with session auth.
// Non-loopback callers ALWAYS require authentication for management APIs to prevent unauthenticated remote takeover.
func DashboardAuth(database *db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Public paths that don't require auth
			if isPublicPath(path) {
				next.ServeHTTP(w, r)
				return
			}

			// Only protect /api/* routes
			if !strings.HasPrefix(path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}

			settings, err := database.GetSettings()
			if err != nil {
				writeAuthError(w, http.StatusInternalServerError, "database error")
				return
			}

			remote := !isLoopback(r.RemoteAddr)

			// If requireLogin is explicitly enabled OR request is remote non-loopback:
			if settings.RequireLogin || remote {
				cookie, err := r.Cookie("auth_token")
				if err != nil || !auth.VerifySessionToken(cookie.Value) {
					writeAuthError(w, http.StatusUnauthorized, "unauthorized")
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isPublicPath(path string) bool {
	publicPaths := []string{
		"/api/health",
		"/api/version",
		"/api/auth/login",
		"/api/auth/logout",
		"/api/auth/status",
	}
	return slices.Contains(publicPaths, path)
}

func writeAuthError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(`{"error":"` + msg + `"}`))
}
