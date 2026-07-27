package handler

import (
	"encoding/json"
	"net/http"

	"github.com/arisvia/cyrene-gateway/internal/mitm"
)

// MITMHandler exposes MITM proxy management endpoints.
type MITMHandler struct {
	server  *mitm.Server
	enabled bool
}

func NewMITMHandler(server *mitm.Server, enabled bool) *MITMHandler {
	return &MITMHandler{server: server, enabled: enabled}
}

// HandleStatus returns MITM status. When disabled, reports a clear disabled state.
func (h *MITMHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if !h.enabled {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false,
			"running": false,
			"reason":  "MITM is disabled. Start the gateway with -mitm to enable (local deployments only).",
		})
		return
	}
	status := h.server.Status()
	status["enabled"] = true
	writeJSON(w, http.StatusOK, status)
}

// HandleStart starts the MITM proxy.
func (h *MITMHandler) HandleStart(w http.ResponseWriter, r *http.Request) {
	if !h.enabled {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "MITM is disabled. Restart the gateway with the -mitm flag to enable it.",
		})
		return
	}
	if err := h.server.Start(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, h.server.Status())
}

// HandleStop stops the MITM proxy.
func (h *MITMHandler) HandleStop(w http.ResponseWriter, r *http.Request) {
	if !h.enabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "MITM is disabled."})
		return
	}
	if err := h.server.Stop(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, h.server.Status())
}

// HandleCert downloads the Root CA certificate (PEM).
func (h *MITMHandler) HandleCert(w http.ResponseWriter, r *http.Request) {
	if !h.enabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "MITM is disabled."})
		return
	}
	pem, err := h.server.CertPEM()
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Root CA not generated yet. Start the MITM server first."})
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="cyrene-mitm-rootCA.crt"`)
	w.WriteHeader(http.StatusOK)
	w.Write(pem)
}

// HandleDNS toggles DNS interception for a tool.
func (h *MITMHandler) HandleDNS(w http.ResponseWriter, r *http.Request) {
	if !h.enabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "MITM is disabled."})
		return
	}

	var req struct {
		Tool    string `json:"tool"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if _, ok := mitm.ToolHosts[req.Tool]; !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown tool"})
		return
	}

	var err error
	if req.Enabled {
		err = h.server.EnableDNS(req.Tool)
	} else {
		err = h.server.DisableDNS(req.Tool)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "dns": h.server.Status()["dns"]})
}

// HandleTraffic returns recent intercepted traffic.
func (h *MITMHandler) HandleTraffic(w http.ResponseWriter, r *http.Request) {
	if !h.enabled {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "traffic": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "traffic": h.server.TrafficLog()})
}

// Shutdown stops the MITM proxy if running (called on gateway shutdown).
func (h *MITMHandler) Shutdown() {
	if h.enabled {
		h.server.Stop()
	}
}
