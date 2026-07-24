package handler

import (
	"fmt"
	"net/http"

	"github.com/arisvia/cyrene-gateway/internal/tunnel"
)

type TunnelHandler struct {
	tunnel *tunnel.Manager
}

func NewTunnelHandler(tm *tunnel.Manager) *TunnelHandler {
	return &TunnelHandler{tunnel: tm}
}

func (h *TunnelHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	status := h.tunnel.GetStatus()
	writeJSON(w, http.StatusOK, status)
}

func (h *TunnelHandler) HandleInstall(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	err := h.tunnel.Install(r.Context(), func(msg string) {
		fmt.Fprintf(w, "event: progress\ndata: {\"message\":%q}\n\n", msg)
		flusher.Flush()
	})

	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: {\"error\":%q}\n\n", err.Error())
		flusher.Flush()
		return
	}

	fmt.Fprintf(w, "event: done\ndata: {\"success\":true}\n\n")
	flusher.Flush()
}

func (h *TunnelHandler) HandleEnable(w http.ResponseWriter, r *http.Request) {
	result, err := h.tunnel.Enable()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *TunnelHandler) HandleDisable(w http.ResponseWriter, r *http.Request) {
	if err := h.tunnel.Disable(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
