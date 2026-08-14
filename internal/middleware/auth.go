package middleware

import (
	"net/http"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/db"
)

// APIKeyAuth validates Bearer tokens on /v1/* routes when requireApiKey is enabled.
func APIKeyAuth(database *db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Only enforce on /v1/* API routes
			if !strings.HasPrefix(path, "/v1/") && !strings.HasPrefix(path, "/v1beta/") {
				next.ServeHTTP(w, r)
				return
			}

			// Check if requireApiKey is enabled
			settings, err := database.GetSettings()
			if err != nil || !settings.RequireAPIKey {
				next.ServeHTTP(w, r)
				return
			}

			// Extract and validate API key
			apiKey := auth.ExtractAPIKey(
				r.Header.Get("Authorization"),
				r.Header.Get("x-api-key"),
			)
			if apiKey == "" {
				writeAuthError(w, http.StatusUnauthorized, "API key required")
				return
			}

			valid, err := database.ValidateAPIKey(apiKey)
			if err != nil || !valid {
				writeAuthError(w, http.StatusUnauthorized, "invalid API key")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// DashboardAuth is the single authorization point for every /api/* management
// route, including the privileged ones (tunnel, MITM, CLI config writes,
// settings). Session auth is enforced when login is required, and always when
// the server is bound to a non-loopback address (37A).
func DashboardAuth(database *db.DB, forceAuth bool) func(http.Handler) http.Handler {
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

			// First-run bootstrap: password setup stays reachable until a
			// password exists, even on remote binds. Once set, this endpoint
			// requires an authenticated session like every other /api route.
			if path == "/api/auth/password" && r.Method == http.MethodPost {
				if s, err := database.GetSettings(); err == nil && s.PasswordHash == "" {
					next.ServeHTTP(w, r)
					return
				}
			}

			required := forceAuth
			if !required {
				settings, err := database.GetSettings()
				if err == nil && settings.RequireLogin {
					required = true
				}
			}
			if !required {
				next.ServeHTTP(w, r)
				return
			}

			// Verify session cookie
			cookie, err := r.Cookie("auth_token")
			if err != nil || !auth.VerifySessionToken(cookie.Value) {
				writeAuthError(w, http.StatusUnauthorized, "unauthorized")
				return
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
	for _, p := range publicPaths {
		if path == p {
			return true
		}
	}
	return false
}

func writeAuthError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(`{"error":"` + msg + `"}`))
}
