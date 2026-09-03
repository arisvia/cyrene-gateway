package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Status is the detection result for a single CLI tool.
type Status struct {
	Installed  bool           `json:"installed"`
	HasGateway bool           `json:"hasGateway"`
	Has9Router bool           `json:"has9Router"` // legacy alias for backward compatibility
	ConfigPath string         `json:"configPath,omitempty"`
	Settings   map[string]any `json:"settings,omitempty"`
	Message    string         `json:"message,omitempty"`
}

// ApplyRequest carries the desired gateway configuration for a tool.
type ApplyRequest struct {
	BaseURL string   `json:"baseUrl"`
	APIKey  string   `json:"apiKey"`
	Model   string   `json:"model"`
	Models  []string `json:"models"`
}

// homeDir returns the user's home directory (best-effort).
func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// commandInstalled reports whether the named binary is on PATH.
func commandInstalled(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// installed reports whether a tool is present: either its binary is on PATH or
// one of its known config files exists.
func installed(bin string, configPaths ...string) bool {
	if bin != "" && commandInstalled(bin) {
		return true
	}
	for _, p := range configPaths {
		if p != "" && fileExists(p) {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// readJSON reads and parses a JSON file, tolerating JSONC trailing commas.
// Returns nil when the file is missing or unparseable.
func readJSON(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	stripped := trailingCommaRe.ReplaceAll(data, []byte("$1"))
	var out map[string]any
	if json.Unmarshal(stripped, &out) != nil {
		return nil
	}
	return out
}

// readJSONArray reads and parses a JSON array file, tolerating JSONC.
func readJSONArray(path string) []any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	stripped := trailingCommaRe.ReplaceAll(data, []byte("$1"))
	var out []any
	if json.Unmarshal(stripped, &out) != nil {
		return nil
	}
	return out
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func readText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func writeText(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// trailingCommaRe strips JSONC trailing commas before } or ].
var trailingCommaRe = regexp.MustCompile(`,(\s*[}\]])`)

// ensureV1 appends a /v1 suffix to a base URL if not already present.
func ensureV1(baseURL string) string {
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL
	}
	return strings.TrimSuffix(baseURL, "/") + "/v1"
}

// stripV1 removes a trailing /v1 suffix from a base URL.
func stripV1(baseURL string) string {
	return strings.TrimSuffix(baseURL, "/v1")
}

// looksLikeGateway reports whether a base URL points at a local gateway.
func looksLikeGateway(baseURL string) bool {
	return strings.Contains(baseURL, "localhost") ||
		strings.Contains(baseURL, "127.0.0.1") ||
		strings.Contains(baseURL, "0.0.0.0") ||
		strings.Contains(baseURL, "9router") ||
		strings.Contains(baseURL, "cyrene")
}

// getNested returns the value at a dotted key path within a nested map.
func getNested(m map[string]any, dotted string) any {
	keys := strings.Split(dotted, ".")
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

// setNested sets a value at a dotted key path, creating intermediate maps.
func setNested(m map[string]any, dotted string, value any) {
	keys := strings.Split(dotted, ".")
	cur := m
	for _, k := range keys[:len(keys)-1] {
		next, ok := cur[k].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[k] = next
		}
		cur = next
	}
	cur[keys[len(keys)-1]] = value
}

// deleteNested removes a value at a dotted key path.
func deleteNested(m map[string]any, dotted string) {
	keys := strings.Split(dotted, ".")
	cur := m
	for _, k := range keys[:len(keys)-1] {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return
		}
		cur = next
	}
	delete(cur, keys[len(keys)-1])
}
