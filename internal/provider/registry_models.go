package provider

// registry_models.go holds the static model catalogs for the curated chat
// registry (Phase 36). Synced from 9router open-sse/providers/registry/ —
// qoder's list follows the realigned aliases from 9router@9c9dd7b1.
// Live catalog fetches (Phase 36 T6) override these; they remain the offline
// fallback.

// ModelRef is a single model entry (id + display name) from the upstream registry.
type ModelRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RegistryModels is empty by default; Cyrene Gateway discovers models purely dynamically from upstream providers.
var RegistryModels = map[string][]ModelRef{}

// GetRegistryModels returns the known registry models for a provider id (may be empty).
func GetRegistryModels(id string) []ModelRef {
	return RegistryModels[id]
}
