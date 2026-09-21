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

			// Only protect /v1/* and /v1beta/* routes
			if !strings.HasPrefix(path, "/v1/") && !strings.HasPrefix(path, "/v1beta/") {
				next.ServeHTTP(w, r)
				return
			}

			// Extract API key
			keyStr := auth.ExtractAPIKey(r)

			// Check if requireApiKey is enabled
			settings, err := database.GetSettings()
			requireKey := err == nil && settings != nil && settings.RequireAPIKey

			if keyStr == "" {
				if requireKey {
					// Allow authenticated dashboard sessions from WebUI / Playground
					cookie, err := r.Cookie("auth_token")
					hasValidCookie := err == nil && cookie.Value != "" && auth.VerifySessionToken(cookie.Value)
					hasValidHeader := r.Header.Get("X-Cyrene-Auth-Token") != "" && auth.VerifySessionToken(r.Header.Get("X-Cyrene-Auth-Token"))
					if hasValidCookie || hasValidHeader {
						next.ServeHTTP(w, r)
						return
					}
					writeAuthError(w, http.StatusUnauthorized, "API key required")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			// Allow authenticated dashboard sessions passing session token in Authorization / Header
			if auth.VerifySessionToken(keyStr) {
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

// isAllowedManagementOrigin reports whether an Origin header is permitted to access /api/* management endpoints.
// Permitted if:
// 1. originHeader is empty (same-origin simple request or non-browser client)
// 2. originHeader is loopback (localhost, 127.0.0.1) for local development or dev tools
// 3. origin matches the current request's Host or X-Forwarded-Host (legitimate same-origin/reverse-proxy request)
func isAllowedManagementOrigin(originHeader string, r *http.Request) bool {
	if originHeader == "" {
		return true
	}
	u, err := url.Parse(originHeader)
	if err != nil {
		return false
	}
	if isLoopbackHost(u.Host) {
		return true
	}
	if r != nil {
		reqHost := r.Header.Get("X-Forwarded-Host")
		if reqHost == "" {
			reqHost = r.Host
		}
		if reqHost != "" {
			origH, _, err := net.SplitHostPort(u.Host)
			if err != nil {
				origH = u.Host
			}
			reqH, _, err := net.SplitHostPort(reqHost)
			if err != nil {
				reqH = reqHost
			}
			origH = strings.TrimPrefix(strings.TrimSuffix(origH, "]"), "[")
			reqH = strings.TrimPrefix(strings.TrimSuffix(reqH, "]"), "[")
			if strings.EqualFold(origH, reqH) {
				return true
			}
		}
	}
	return false
}

// DashboardAuth protects /api/* management routes with session auth.
// Public WAN callers ALWAYS require authentication for management APIs to prevent unauthenticated remote takeover.
// Localhost and LAN/private network callers follow the settings.RequireLogin configuration.
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

			isWAN := auth.IsPublicWANRequest(r)
			mustAuthenticate := settings.RequireLogin || isWAN

			if mustAuthenticate {
				// Allow initial admin password setup when no password has been configured yet (prevents lockout)
				if path == "/api/auth/password" && settings.PasswordHash == "" {
					next.ServeHTTP(w, r)
					return
				}
				token := ""
				if cookie, err := r.Cookie("auth_token"); err == nil && cookie.Value != "" {
					token = cookie.Value
				}
				if token == "" {
					authHeader := r.Header.Get("Authorization")
					if strings.HasPrefix(authHeader, "Bearer ") {
						candidate := strings.TrimPrefix(authHeader, "Bearer ")
						if strings.Contains(candidate, ".") {
							token = candidate
						}
					}
				}
				if token == "" {
					token = r.Header.Get("X-Cyrene-Auth-Token")
				}
				if token == "" {
					token = r.URL.Query().Get("token")
				}
				if token == "" || !auth.VerifySessionToken(token) {
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
