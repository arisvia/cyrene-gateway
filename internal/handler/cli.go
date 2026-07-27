package handler

import (
	"encoding/json"
	"net/http"

	"github.com/arisvia/cyrene-gateway/internal/cli"
)

type CLIHandler struct {
	cli *cli.Manager
}

func NewCLIHandler(m *cli.Manager) *CLIHandler {
	return &CLIHandler{cli: m}
}

// HandleList returns the static tool registry.
func (h *CLIHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tools": cli.Registry})
}

// HandleAllStatuses returns the detection status for every tool.
func (h *CLIHandler) HandleAllStatuses(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.cli.AllStatuses())
}

// HandleGet returns the definition + live status for a single tool.
func (h *CLIHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tool := cli.GetTool(id)
	if tool == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown tool"})
		return
	}
	var status cli.Status
	if a := h.cli.Adapter(id); a != nil {
		status = a.Status()
	} else {
		status = cli.Status{Installed: false, Message: "manual setup only"}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tool": tool, "status": status})
}

// HandleApply writes the gateway configuration to a tool's config files.
func (h *CLIHandler) HandleApply(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a := h.cli.Adapter(id)
	if a == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tool is not configurable via this endpoint"})
		return
	}
	var req cli.ApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.BaseURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "baseUrl is required"})
		return
	}
	status, err := a.Apply(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "status": status})
}

// HandleReset removes the gateway configuration from a tool's config files.
func (h *CLIHandler) HandleReset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a := h.cli.Adapter(id)
	if a == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tool is not configurable via this endpoint"})
		return
	}
	status, err := a.Reset()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "status": status})
}
