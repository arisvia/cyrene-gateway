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
// Display fields mirror provider.ProviderInfo so a media-only provider renders
// the same card as a chat provider (brand colour, console link) instead of
// falling back to initials.
type MediaProviderInfo struct {
	Provider  string                  `json:"provider"`
	Name      string                  `json:"name"`
	Kinds     []Kind                  `json:"kinds"`
	Color     string                  `json:"color,omitempty"`
	Website   string                  `json:"website,omitempty"`
	APIKeyURL string                  `json:"apiKeyUrl,omitempty"`
	Models    []ModelEntry            `json:"models,omitempty"`
	Configs   map[Kind]ProviderConfig `json:"configs,omitempty"`
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

	for id, m := range displayMeta {
		p, ok := Registry[id]
		if !ok {
			continue
		}
		p.Color, p.Website, p.APIKeyURL = m.color, m.website, m.apiKeyURL
	}
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

// displayMeta carries the brand metadata the per-kind registration files would
// otherwise each have to repeat. Providers absent from this table have no
// brand assets (they render initials), or are also chat providers whose
// ProviderInfo already supplies the colour.
var displayMeta = map[string]struct{ color, website, apiKeyURL string }{
	"stability-ai": {"#8B5CF6", "https://platform.stability.ai", "https://platform.stability.ai/account/keys"},
	"elevenlabs":   {"#141414", "https://elevenlabs.io", "https://elevenlabs.io/app/settings/api-keys"},
	"deepgram":     {"#13EF93", "https://deepgram.com", "https://console.deepgram.com/"},
	"brave-search": {"#FB542B", "https://brave.com/search/api/", "https://brave.com/search/api/"},
	"tavily":       {"#6855F5", "https://tavily.com", "https://app.tavily.com/home"},
	"exa":          {"#1F40ED", "https://exa.ai", "https://dashboard.exa.ai/api-keys"},
	"firecrawl":    {"#F97316", "https://firecrawl.dev", "https://www.firecrawl.dev/app/api-keys"},
	"fal":          {"#2563EB", "https://fal.ai", "https://fal.ai/dashboard/keys"},
	"cartesia":     {"#FF4F8B", "https://cartesia.ai", "https://play.cartesia.ai/keys"},
	"assemblyai":   {"#0062FF", "https://assemblyai.com", "https://www.assemblyai.com/app/api-keys"},
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
