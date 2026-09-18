package handler

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/config"
	"github.com/arisvia/cyrene-gateway/internal/db"
)

// EndpointHandler serves endpoint discovery API.
type EndpointHandler struct {
	cfg *config.Config
	db  *db.DB
}

func NewEndpointHandler(cfg *config.Config, database *db.DB) *EndpointHandler {
	return &EndpointHandler{cfg: cfg, db: database}
}

func (h *EndpointHandler) HandleEndpoints(w http.ResponseWriter, r *http.Request) {
	type Endpoint struct {
		Label string `json:"label"`
		URL   string `json:"url"`
		Type  string `json:"type"`
	}

	port := h.cfg.Port
	var endpoints []Endpoint
	seenURLs := make(map[string]bool)

	// 1. Current access host/domain if request context provided
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" || strings.Contains(r.Header.Get("CF-Visitor"), `"scheme":"https"`) {
		scheme = "https"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	if host != "" {
		currentURL := fmt.Sprintf("%s://%s", scheme, host)
		endpoints = append(endpoints, Endpoint{
			Label: "Current (" + host + ")",
			URL:   currentURL,
			Type:  "current",
		})
		seenURLs[currentURL] = true
	}

	// 2. Localhost
	localURL := fmt.Sprintf("http://localhost:%d", port)
	if !seenURLs[localURL] {
		endpoints = append(endpoints, Endpoint{
			Label: "Localhost",
			URL:   localURL,
			Type:  "local",
		})
		seenURLs[localURL] = true
	}

	// 3. LAN addresses
	for _, ip := range localIPs() {
		lanURL := fmt.Sprintf("http://%s:%d", ip, port)
		if !seenURLs[lanURL] {
			endpoints = append(endpoints, Endpoint{
				Label: "LAN (" + ip + ")",
				URL:   lanURL,
				Type:  "lan",
			})
			seenURLs[lanURL] = true
		}
	}
	// Auth status
	requireAuth := false
	if settings, err := h.db.GetSettings(); err == nil && settings != nil {
		requireAuth = settings.RequireAPIKey
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"endpoints":   endpoints,
		"requireAuth": requireAuth,
		"port":        port,
	})
}

func localIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ip4 := ipNet.IP.To4(); ip4 != nil {
				ips = append(ips, ip4.String())
			}
		}
	}
	return ips
}
