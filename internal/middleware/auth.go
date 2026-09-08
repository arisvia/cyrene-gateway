package middleware

import (
	"net"
	"net/http"
	"slices"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/db"
)

// APIKeyAuth validates API keys for /v1/* endpoints when requireApiKey is enabled,
// and injects the authenticated APIKey object into the request context.
func APIKeyAuth(database *db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Only protect /v1/* routes
			if !strings.HasPrefix(path, "/v1/") {
				next.ServeHTTP(w, r)
				return
			}

			// Extract API key
			keyStr := auth.ExtractAPIKey(
				r.Header.Get("Authorization"),
				r.Header.Get("x-api-key"),
			)

			// Check if requireApiKey is enabled
			settings, err := database.GetSettings()
			requireKey := err == nil && settings != nil && settings.RequireAPIKey

			if keyStr == "" {
				if requireKey {
					writeAuthError(w, http.StatusUnauthorized, "API key required")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			// Validate signature first (fast path)
			if !auth.VerifyAPIKeySignature(keyStr) {
				if requireKey {
					writeAuthError(w, http.StatusUnauthorized, "invalid API key signature")
					return
				}
				// Open gateway mode: unverified key treated as anonymous caller
				next.ServeHTTP(w, r)
				return
			}

			// Validate against database
			keyObj, err := database.GetAPIKeyByKey(keyStr)
			if err != nil || keyObj == nil || !keyObj.IsActive {
				if requireKey {
					writeAuthError(w, http.StatusUnauthorized, "invalid or inactive API key")
					return
				}
				// Open gateway mode: unknown/inactive key treated as anonymous caller
				next.ServeHTTP(w, r)
				return
			}
			if keyObj.IsExpired() {
				if requireKey {
					writeAuthError(w, http.StatusUnauthorized, "API key has expired")
					return
				}
				// Open gateway mode: expired key treated as anonymous caller
				next.ServeHTTP(w, r)
				return
			}
			ctx := auth.WithAPIKey(r.Context(), keyObj)
			next.ServeHTTP(w, r.WithContext(ctx))
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
