package cli

// Adapter implements detection and configuration for a single CLI tool.
type Adapter interface {
	// Status detects whether the tool is installed and gateway-configured.
	Status() Status
	// Apply writes the gateway configuration to the tool's config files.
	Apply(req ApplyRequest) (Status, error)
	// Reset removes the gateway configuration from the tool's config files.
	Reset() (Status, error)
}

// Manager resolves adapters by tool id.
type Manager struct{}

func NewManager() *Manager { return &Manager{} }

// Adapter returns the adapter for a tool id, or nil if the id is unknown.
func (m *Manager) Adapter(id string) Adapter {
	switch id {
	case "claude":
		return &claudeAdapter{}
	case "codex":
		return &codexAdapter{}
	case "opencode":
		return &opencodeAdapter{}
	case "aider":
		return &aiderAdapter{}
	case "cline":
		return &clineAdapter{}
	case "continue":
		return &continueAdapter{}
	case "copilot":
		return &copilotAdapter{}
	case "deepseek-tui":
		return &deepseekTuiAdapter{}
	case "grok-cli", "grok-build":
		return &grokCliAdapter{}
	default:
		return nil
	}
}

// AllStatuses gathers the status of every configurable tool.
func (m *Manager) AllStatuses() map[string]Status {
	out := make(map[string]Status)
	for _, tool := range Registry {
		a := m.Adapter(tool.ID)
		if a == nil {
			out[tool.ID] = Status{Installed: false, Message: "manual setup only"}
			continue
		}
		out[tool.ID] = a.Status()
	}
	return out
}
