package media

import "slices"

// Kind represents a media service type.
type Kind string

const (
	KindEmbedding Kind = "embedding"
	KindImage     Kind = "image"
	KindTTS       Kind = "tts"
	KindSTT       Kind = "stt"
	KindVideo     Kind = "video"
	KindWebFetch  Kind = "web-fetch"
	KindWebSearch Kind = "web-search"
)

// ProviderConfig defines a media provider's endpoint configuration for a specific kind.
type ProviderConfig struct {
	Provider   string `json:"provider"`
	Kind       Kind   `json:"kind"`
	BaseURL    string `json:"baseUrl"`
	AuthType   string `json:"authType"`   // "apikey", "oauth", "none"
	AuthHeader string `json:"authHeader"` // "bearer", "x-api-key", "key", "token"
	Format     string `json:"format"`     // provider-specific format identifier
}

// ModelEntry defines a model available for a media kind.
type ModelEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind Kind   `json:"kind"`
}

// MediaProviderInfo is the full media provider definition returned by the API.
type MediaProviderInfo struct {
	Provider string                  `json:"provider"`
	Name     string                  `json:"name"`
	Kinds    []Kind                  `json:"kinds"`
	Models   []ModelEntry            `json:"models,omitempty"`
	Configs  map[Kind]ProviderConfig `json:"configs,omitempty"`
}

// Registry holds all media provider definitions keyed by provider ID.
var Registry map[string]*MediaProviderInfo

func init() {
	Registry = make(map[string]*MediaProviderInfo)
	registerEmbeddingProviders()
	registerImageProviders()
	registerTTSProviders()
	registerSTTProviders()
	registerVideoProviders()
	registerWebProviders()
}

// GetProvidersByKind returns all providers supporting a given kind.
func GetProvidersByKind(kind Kind) []*MediaProviderInfo {
	var result []*MediaProviderInfo
	for _, p := range Registry {
		if slices.Contains(p.Kinds, kind) {
			result = append(result, p)
		}
	}
	return result
}

// GetConfig returns the config for a provider+kind, or nil.
func GetConfig(provider string, kind Kind) *ProviderConfig {
	p, ok := Registry[provider]
	if !ok {
		return nil
	}
	cfg, ok := p.Configs[kind]
	if !ok {
		return nil
	}
	return &cfg
}

// SupportsKind checks if a provider supports a given kind.
func SupportsKind(provider string, kind Kind) bool {
	p, ok := Registry[provider]
	if !ok {
		return false
	}
	return slices.Contains(p.Kinds, kind)
}

func addProvider(id, name string, kinds []Kind, models []ModelEntry, configs map[Kind]ProviderConfig) {
	Registry[id] = &MediaProviderInfo{
		Provider: id,
		Name:     name,
		Kinds:    kinds,
		Models:   models,
		Configs:  configs,
	}
}

// mergeProvider adds or merges a kind into an existing provider entry.
func mergeProvider(id, name string, kind Kind, models []ModelEntry, cfg ProviderConfig) {
	p, ok := Registry[id]
	if !ok {
		p = &MediaProviderInfo{
			Provider: id,
			Name:     name,
			Kinds:    []Kind{},
			Models:   []ModelEntry{},
			Configs:  map[Kind]ProviderConfig{},
		}
		Registry[id] = p
	}
	// Add kind if not present
	found := slices.Contains(p.Kinds, kind)
	if !found {
		p.Kinds = append(p.Kinds, kind)
	}
	// Add models
	p.Models = append(p.Models, models...)
	// Set config
	p.Configs[kind] = cfg
}
