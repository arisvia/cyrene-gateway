package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"time"
)

// Chain wraps a handler with the given middleware in order.
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for _, mw := range slices.Backward(mws) {
		h = mw(h)
	}
	return h
}

// Logging logs each request with method, path, status, and duration.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(sw, r)

		slog.Info("request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", sw.status),
			slog.Duration("duration", time.Since(start)),
			slog.String("remote", r.RemoteAddr),
		)
	})
}

// Recovery catches panics and returns 500 instead of crashing.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered",
					slog.Any("error", rec),
					slog.String("path", r.URL.Path),
					slog.String("stack", string(debug.Stack())),
				)
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders applies browser hardening without forcing HTTPS on local deployments.
// The CSP is only attached to the dashboard HTML document, where the console runs:
// applying it globally would break the user-configurable remote wallpaper feature
// (cross-origin fetch + <img src> in the appearance panel) and the OAuth callback page.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if isDashboardDocument(r.URL.Path) {
			// img-src https: data: blob: and connect-src https: are required by the
			// user-configurable remote wallpaper (appearance panel fetches an
			// arbitrary image URL and renders it). Everything else stays locked
			// to 'self'; script-src stays strict because the theme-init bootstrap
			// is an external classic script (webui/public/theme-init.js).
			h.Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; frame-ancestors 'none'; object-src 'none'; form-action 'self'; img-src 'self' data: blob: https:; font-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self' https:")
		}
		next.ServeHTTP(w, r)
	})
}

// isDashboardDocument reports whether the path resolves to the SPA entry document,
// the only response that renders console markup and therefore needs the CSP.
func isDashboardDocument(path string) bool {
	return path == "/" || path == "/index.html"
}

// SanitizeInternalHeaders strips gateway-internal control headers from incoming external requests.
func SanitizeInternalHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("X-Cyrene-Original-Endpoint")
		r.Header.Del("X-Cyrene-Request-ID")
		next.ServeHTTP(w, r)
	})
}

// CORS adds CORS headers. Permissive for /v1/* and public paths, loopback-only for protected /api/*.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		isProtectedAPI := strings.HasPrefix(r.URL.Path, "/api/") && !isPublicPath(r.URL.Path)

		if isProtectedAPI {
			// For protected management APIs, only allow loopback or legitimate same-origin requests
			if origin != "" && isAllowedManagementOrigin(origin, r) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
		} else {
			// For public LLM API routes (/v1/*) and public health/version endpoints, allow any origin
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, X-API-Key, X-Cyrene-Auth-Token, X-Cyrene-Connection-ID, X-Goog-Api-Key, Anthropic-Version, Anthropic-Beta, Anthropic-Dangerous-Direct-Browser-Access")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
