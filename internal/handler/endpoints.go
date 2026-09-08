package handler

import (
	"fmt"
	"net"
	"net/http"

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

	// Local
	endpoints = append(endpoints, Endpoint{
		Label: "Localhost",
		URL:   fmt.Sprintf("http://localhost:%d", port),
		Type:  "local",
	})

	// LAN addresses
	for _, ip := range localIPs() {
		endpoints = append(endpoints, Endpoint{
			Label: "LAN (" + ip + ")",
			URL:   fmt.Sprintf("http://%s:%d", ip, port),
			Type:  "lan",
		})
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
