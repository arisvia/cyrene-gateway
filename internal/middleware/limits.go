package middleware

import (
	"net/http"
)

const (
	// MaxBodySizeJSON limits standard JSON API requests (10 MB)
	MaxBodySizeJSON = 10 << 20
	// MaxBodySizeMultipart limits multipart uploads (50 MB)
	MaxBodySizeMultipart = 50 << 20
)

// RequestSizeLimiter enforces maximum body read size per endpoint.
func RequestSizeLimiter() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			maxBytes := int64(MaxBodySizeJSON)
			if r.Header.Get("Content-Type") != "" && len(r.Header.Get("Content-Type")) >= 19 && r.Header.Get("Content-Type")[:19] == "multipart/form-data" {
				maxBytes = MaxBodySizeMultipart
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
