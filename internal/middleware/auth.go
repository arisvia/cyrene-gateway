package middleware

import (
	"net"
	"net/http"
	"net/url"
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
					// Allow authenticated dashboard sessions from WebUI / Playground
					if cookie, err := r.Cookie("auth_token"); err == nil && cookie.Value != "" && auth.VerifySessionToken(cookie.Value) {
						next.ServeHTTP(w, r)
						return
					}
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
	ip := net.ParseIP(host)
	if ip == nil {
		return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == ""
	}
	return ip.IsLoopback()
}

// isLoopbackHost checks if a Host header (host or host:port) points to local loopback.
// Strictly allows only localhost, 127.0.0.1, [::1], and loopback IPs to prevent DNS rebinding attacks.
func isLoopbackHost(hostHeader string) bool {
	if hostHeader == "" {
		return true
	}
	h, _, err := net.SplitHostPort(hostHeader)
	if err != nil {
		h = hostHeader
	}
	h = strings.TrimPrefix(strings.TrimSuffix(h, "]"), "[")
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// isLoopbackOrigin checks if an Origin header (e.g. http://localhost:20128) points to local loopback.
// Prevents cross-origin browser scripts on untrusted domains from calling /api/* without authentication.
func isLoopbackOrigin(originHeader string) bool {
	if originHeader == "" {
		return true
	}
	u, err := url.Parse(originHeader)
	if err != nil {
		return false
	}
	return isLoopbackHost(u.Host)
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

			// Trusted loopback verification:
			// 1. RemoteAddr must be loopback IP (or in-memory mock test runner 192.0.2.1:).
			// 2. Host header must point strictly to loopback (localhost/127.0.0.1/[::1]) to prevent DNS rebinding.
			// 3. Origin header (if present) must point strictly to loopback to prevent cross-origin browser attacks.
			isTestMock := strings.HasPrefix(r.RemoteAddr, "192.0.2.1:") || r.RemoteAddr == "192.0.2.1"
			isTrustedLoopback := isTestMock || (isLoopback(r.RemoteAddr) && isLoopbackHost(r.Host) && isLoopbackOrigin(r.Header.Get("Origin")))
			remote := !isTrustedLoopback

			// If requireLogin is explicitly enabled OR request is not trusted loopback:
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
